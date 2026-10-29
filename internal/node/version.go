package node

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Version is a Node.js release number.
type Version [3]int

// ParseVersion reads "v22.11.0" or "22.11.0".
func ParseVersion(s string) (Version, error) {
	var v Version
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(s), "v"), ".")
	if len(parts) != 3 {
		return v, fmt.Errorf("%q is not a version like v22.11.0", s)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, fmt.Errorf("%q is not a version like v22.11.0", s)
		}
		v[i] = n
	}
	return v, nil
}

func (v Version) String() string { return fmt.Sprintf("v%d.%d.%d", v[0], v[1], v[2]) }

func (v Version) Major() int { return v[0] }

// Compare returns -1, 0 or 1.
func (v Version) Compare(o Version) int {
	for i := range 3 {
		switch {
		case v[i] < o[i]:
			return -1
		case v[i] > o[i]:
			return 1
		}
	}
	return 0
}

// Range is an npm-style version range, as used by .nvmrc files and the
// engines field of package.json: "22", "20.x", "^18.17", "~20.11.1",
// ">=18 <21", "18 - 20", "20 || 22".
type Range [][]comparator // OR of ANDs

type comparator struct {
	op string // ">=", ">", "<=", "<", "="
	v  Version
}

func (c comparator) ok(v Version) bool {
	d := v.Compare(c.v)
	switch c.op {
	case ">=":
		return d >= 0
	case ">":
		return d > 0
	case "<=":
		return d <= 0
	case "<":
		return d < 0
	}
	return d == 0
}

// Match reports whether v is in the range.
func (r Range) Match(v Version) bool {
	for _, set := range r {
		all := true
		for _, c := range set {
			if !c.ok(v) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

// Exact returns the version when the range names exactly one.
func (r Range) Exact() (Version, bool) {
	if len(r) == 1 && len(r[0]) == 1 && r[0][0].op == "=" {
		return r[0][0].v, true
	}
	return Version{}, false
}

var errRange = errors.New("not a version range")

// ParseRange reads an npm-style range. A bare "v" prefix is allowed anywhere.
func ParseRange(s string) (Range, error) {
	var r Range
	for _, alt := range strings.Split(s, "||") {
		set, err := parseSet(strings.TrimSpace(alt))
		if err != nil {
			return nil, fmt.Errorf("%q: %w", strings.TrimSpace(s), err)
		}
		r = append(r, set)
	}
	return r, nil
}

func parseSet(s string) ([]comparator, error) {
	if s == "" || s == "*" || s == "x" || s == "X" {
		return []comparator{{">=", Version{}}}, nil
	}
	// Hyphen range: "1.2 - 2.3.4".
	if a, b, ok := strings.Cut(s, " - "); ok {
		lo, _, err := partial(strings.TrimSpace(a))
		if err != nil {
			return nil, err
		}
		hi, n, err := partial(strings.TrimSpace(b))
		if err != nil {
			return nil, err
		}
		set := []comparator{{">=", lo}}
		if n == 3 {
			return append(set, comparator{"<=", hi}), nil
		}
		if n > 0 {
			set = append(set, comparator{"<", bump(hi, n)})
		}
		return set, nil
	}

	// Operators may be separated from their version by spaces (">= 18").
	fields := strings.Fields(strings.ReplaceAll(s, ",", " "))
	var tokens []string
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if strings.Trim(f, "<>=~^") == "" && i+1 < len(fields) {
			f += fields[i+1]
			i++
		}
		tokens = append(tokens, f)
	}
	var set []comparator
	for _, t := range tokens {
		cs, err := parseComparator(t)
		if err != nil {
			return nil, err
		}
		set = append(set, cs...)
	}
	return set, nil
}

func parseComparator(t string) ([]comparator, error) {
	op := ""
	for _, p := range []string{">=", "<=", ">", "<", "=", "~>", "~", "^"} {
		if strings.HasPrefix(t, p) {
			op, t = p, strings.TrimSpace(t[len(p):])
			break
		}
	}
	v, n, err := partial(t)
	if err != nil {
		return nil, err
	}
	switch op {
	case "", "=":
		switch n {
		case 0:
			return []comparator{{">=", Version{}}}, nil
		case 3:
			return []comparator{{"=", v}}, nil
		}
		return []comparator{{">=", v}, {"<", bump(v, n)}}, nil
	case "^":
		// Up to the next change of the first non-zero part.
		switch {
		case n == 0:
			return []comparator{{">=", Version{}}}, nil
		case v[0] > 0 || n == 1:
			return []comparator{{">=", v}, {"<", bump(v, 1)}}, nil
		case v[1] > 0 || n == 2:
			return []comparator{{">=", v}, {"<", bump(v, 2)}}, nil
		}
		return []comparator{{">=", v}, {"<", bump(v, 3)}}, nil
	case "~", "~>":
		if n == 0 {
			return []comparator{{">=", Version{}}}, nil
		}
		return []comparator{{">=", v}, {"<", bump(v, min(n, 2))}}, nil
	case ">":
		if n == 0 {
			return []comparator{{"<", Version{}}}, nil // nothing
		}
		if n < 3 {
			return []comparator{{">=", bump(v, n)}}, nil
		}
		return []comparator{{">", v}}, nil
	case ">=":
		return []comparator{{">=", v}}, nil
	case "<":
		return []comparator{{"<", v}}, nil
	case "<=":
		if n == 0 {
			return []comparator{{">=", Version{}}}, nil
		}
		if n < 3 {
			return []comparator{{"<", bump(v, n)}}, nil
		}
		return []comparator{{"<=", v}}, nil
	}
	return nil, errRange
}

// partial reads "22", "22.11", "22.11.0" or "22.x"; n is how many parts were
// given (wildcards end it).
func partial(s string) (Version, int, error) {
	var v Version
	s = strings.TrimPrefix(s, "v")
	if s == "" || s == "*" || s == "x" || s == "X" {
		return v, 0, nil
	}
	// Pre-release and build suffixes do not occur in Node releases.
	if i := strings.IndexAny(s, "-+"); i > 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		return v, 0, errRange
	}
	n := 0
	for i, p := range parts {
		if p == "x" || p == "X" || p == "*" {
			break
		}
		x, err := strconv.Atoi(p)
		if err != nil || x < 0 {
			return v, 0, errRange
		}
		v[i] = x
		n++
	}
	return v, n, nil
}

// bump returns the smallest version above every version that starts with the
// first n parts of v.
func bump(v Version, n int) Version {
	switch n {
	case 1:
		return Version{v[0] + 1, 0, 0}
	case 2:
		return Version{v[0], v[1] + 1, 0}
	}
	return Version{v[0], v[1], v[2] + 1}
}
