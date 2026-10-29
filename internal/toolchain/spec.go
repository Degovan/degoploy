package toolchain

import (
	"errors"
	"strings"
)

// Spec is what a project or a version file asks for: a range, or an alias
// resolved against the release list.
type Spec struct {
	Range    Range
	Alias    string // "lts" or "latest"
	Codename string // with Alias "lts": a line such as "iron"
}

// ParseSpec reads a version spec as nvm and Composer do: "22", "v20.11.1",
// "^8.2", ">=18 <21", "lts", "lts/*", "lts/iron", "node", "latest".
func ParseSpec(s string) (Spec, error) {
	s = strings.TrimSpace(s)
	low := strings.ToLower(s)
	switch {
	case s == "":
		return Spec{}, errors.New("no version given")
	case low == "lts" || low == "lts/*":
		return Spec{Alias: "lts"}, nil
	case strings.HasPrefix(low, "lts/"):
		return Spec{Alias: "lts", Codename: low[len("lts/"):]}, nil
	case low == "node" || low == "latest" || low == "current" || low == "stable":
		return Spec{Alias: "latest"}, nil
	}
	r, err := ParseRange(s)
	if err != nil {
		return Spec{}, errors.New("use a version such as 22, 8.3, 20.11.1 or lts")
	}
	return Spec{Range: r}, nil
}

// alias reports whether r is the release an alias spec names.
func (s Spec) alias(r Release) bool {
	if s.Alias == "latest" {
		return true
	}
	return r.LTS != "" && (s.Codename == "" || strings.EqualFold(r.LTS, s.Codename))
}

// ValidSetting reports whether s is a project setting: Auto, Off or a spec.
func ValidSetting(s string) error {
	if s == Auto || s == Off {
		return nil
	}
	if len(s) > 50 {
		return errors.New("must be at most 50 characters")
	}
	_, err := ParseSpec(s)
	return err
}

// firstLine is the first line that is not empty or a comment.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}
