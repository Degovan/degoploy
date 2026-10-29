package node

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Project settings that are not a version.
const (
	// Auto reads the version from the repository (.nvmrc, .node-version,
	// package.json), falling back to the default version.
	Auto = "auto"
	// None leaves PATH alone: scripts get whatever node the server has.
	None = "none"
)

// Spec is what a project or .nvmrc asks for: a range, or an alias resolved
// against the release list.
type Spec struct {
	Range    Range
	Alias    string // "lts" or "latest"
	Codename string // with Alias "lts": a line such as "iron"
}

// ParseSpec reads a version spec as nvm does: "22", "v20.11.1", "^18.17",
// ">=18 <21", "lts", "lts/*", "lts/iron", "node", "latest".
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
		return Spec{}, errors.New("use a version such as 22, 20.11.1 or lts")
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

// Detect finds the version a repository asks for, the way nvm and similar
// tools read it. source names where it came from, for the deployment log.
func Detect(dir string) (spec, source string) {
	for _, name := range []string{".nvmrc", ".node-version"} {
		if b, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			if s := firstLine(string(b)); s != "" {
				return s, name
			}
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return "", ""
	}
	var pkg struct {
		Volta struct {
			Node string `json:"node"`
		} `json:"volta"`
		Engines struct {
			Node string `json:"node"`
		} `json:"engines"`
	}
	if json.Unmarshal(b, &pkg) != nil {
		return "", ""
	}
	if s := strings.TrimSpace(pkg.Volta.Node); s != "" {
		return s, "package.json volta.node"
	}
	if s := strings.TrimSpace(pkg.Engines.Node); s != "" {
		return s, "package.json engines.node"
	}
	return "", ""
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
