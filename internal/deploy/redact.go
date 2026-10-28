package deploy

import (
	"bytes"
	"io"
	"slices"
	"strings"
)

// redactMask replaces secret values in deployment output.
const redactMask = "***"

// minRedactLen is the shortest value that is masked; masking one- or two-letter
// values would garble the log without protecting anything.
const minRedactLen = 3

// redactor masks secret values in output before passing it on. A write can end
// in the middle of a secret, so trailing bytes that could start one are held
// back until the next write (or Flush) shows whether they do.
//
// It is not safe for concurrent use; exec.Cmd serializes writes when Stdout
// and Stderr are the same writer.
type redactor struct {
	w       io.Writer
	secrets [][]byte // longest first, so a secret containing another is masked whole
	held    []byte
}

// newRedactor masks every value and, for multi-line values, each line of it.
func newRedactor(w io.Writer, values []string) *redactor {
	seen := map[string]bool{}
	r := &redactor{w: w}
	add := func(s string) {
		if len(s) >= minRedactLen && !seen[s] {
			seen[s] = true
			r.secrets = append(r.secrets, []byte(s))
		}
	}
	for _, v := range values {
		add(v)
		if strings.Contains(v, "\n") {
			for line := range strings.Lines(v) {
				add(strings.TrimSpace(line))
			}
		}
	}
	slices.SortFunc(r.secrets, func(a, b []byte) int { return len(b) - len(a) })
	return r
}

func (r *redactor) Write(p []byte) (int, error) {
	if len(r.secrets) == 0 {
		return r.w.Write(p)
	}
	buf := append(r.held, p...)
	for _, s := range r.secrets {
		if bytes.Contains(buf, s) {
			buf = bytes.ReplaceAll(buf, s, []byte(redactMask))
		}
	}
	keep := r.partialSuffix(buf)
	r.held = append([]byte(nil), buf[len(buf)-keep:]...)
	if _, err := r.w.Write(buf[:len(buf)-keep]); err != nil {
		return 0, err
	}
	return len(p), nil
}

// partialSuffix is the length of the longest suffix of buf that is the start
// of a secret (but not a whole one, which Write has already masked).
func (r *redactor) partialSuffix(buf []byte) int {
	from := max(0, len(buf)-len(r.secrets[0])+1)
	for i := from; i < len(buf); i++ {
		tail := buf[i:]
		for _, s := range r.secrets {
			if len(tail) < len(s) && bytes.HasPrefix(s, tail) {
				return len(tail)
			}
		}
	}
	return 0
}

// Flush writes out the bytes held back by the last Write.
func (r *redactor) Flush() error {
	if len(r.held) == 0 {
		return nil
	}
	_, err := r.w.Write(r.held)
	r.held = nil
	return err
}
