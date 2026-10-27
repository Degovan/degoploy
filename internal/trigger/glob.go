// Package trigger decides whether a push should deploy, based on a project's rules.
package trigger

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Glob is a compiled pattern. "*" matches any run of characters except '/',
// "**" matches any run including '/', and "?" matches one character except '/'.
//
// For path patterns without a '/', the pattern also matches the file's base name
// anywhere in the tree (so "*.md" matches "docs/readme.md"), like .gitignore.
type Glob struct {
	src      string
	re       *regexp.Regexp
	basename bool
}

func CompileGlob(pattern string, isPath bool) (*Glob, error) {
	p := strings.TrimSpace(pattern)
	if p == "" {
		return nil, fmt.Errorf("empty pattern")
	}
	if strings.ContainsAny(p, "[]{}\\") {
		return nil, fmt.Errorf("pattern %q: only *, ** and ? wildcards are supported", p)
	}
	p = strings.TrimPrefix(p, "/")

	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(p); i++ {
		switch c := p[i]; {
		case c == '*' && i+1 < len(p) && p[i+1] == '*':
			i++
			// "**/" also matches zero directories.
			if i+1 < len(p) && p[i+1] == '/' {
				i++
				b.WriteString("(?:.*/)?")
			} else {
				b.WriteString(".*")
			}
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, fmt.Errorf("pattern %q: %w", pattern, err)
	}
	return &Glob{src: pattern, re: re, basename: isPath && !strings.Contains(p, "/")}, nil
}

func (g *Glob) Match(s string) bool {
	if g.re.MatchString(s) {
		return true
	}
	return g.basename && g.re.MatchString(path.Base(s))
}

func (g *Glob) String() string { return g.src }

// SplitPatterns splits a comma- or newline-separated pattern list.
func SplitPatterns(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func compileAll(patterns []string, isPath bool) ([]*Glob, error) {
	out := make([]*Glob, 0, len(patterns))
	for _, p := range patterns {
		g, err := CompileGlob(p, isPath)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

func anyMatch(globs []*Glob, s string) bool {
	for _, g := range globs {
		if g.Match(s) {
			return true
		}
	}
	return false
}
