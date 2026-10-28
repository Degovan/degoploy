package trigger

import (
	"fmt"
	"strings"

	"github.com/degovan/degoploy/internal/store"
)

const (
	KindPush = "push" // branch push
	KindTag  = "tag"  // tag push
)

// Push is the part of a webhook that rules look at.
type Push struct {
	Kind    string // KindPush or KindTag
	RefName string // branch or tag name, without refs/heads/ or refs/tags/
	Message string // head commit message
	Files   []string
	// FilesKnown is false when the payload does not list every changed file
	// (new branch, force push, truncated commit list). Path filters then pass.
	FilesKnown bool
}

// Result explains how one rule evaluated.
type Result struct {
	RuleID  int64  `json:"rule_id"`
	Name    string `json:"name"`
	Matched bool   `json:"matched"`
	Reason  string `json:"reason"`
}

// Evaluate checks rules in order and returns the first match (or nil) plus a
// result for every rule.
func Evaluate(rules []*store.TriggerRule, ev Push) (*store.TriggerRule, []Result) {
	var winner *store.TriggerRule
	results := make([]Result, 0, len(rules))
	for _, r := range rules {
		ok, reason := evaluate(r, ev)
		if ok && winner == nil {
			winner = r
		} else if ok {
			reason = "matches, but an earlier rule already matched"
			ok = false
		}
		results = append(results, Result{RuleID: r.ID, Name: r.Name, Matched: ok, Reason: reason})
	}
	return winner, results
}

func evaluate(r *store.TriggerRule, ev Push) (bool, string) {
	if !r.Enabled {
		return false, "rule is disabled"
	}
	if r.Event != ev.Kind {
		return false, fmt.Sprintf("rule listens for %s events, got %s", eventLabel(r.Event), eventLabel(ev.Kind))
	}

	refs, err := compileAll(SplitPatterns(r.RefPattern), false)
	if err != nil {
		return false, "invalid ref pattern: " + err.Error()
	}
	if !anyMatch(refs, ev.RefName) {
		return false, fmt.Sprintf("%q does not match %s", ev.RefName, r.RefPattern)
	}

	if kw := strings.TrimSpace(r.SkipKeyword); kw != "" && strings.Contains(ev.Message, kw) {
		return false, fmt.Sprintf("commit message contains %q", kw)
	}

	if len(r.PathsInclude) == 0 && len(r.PathsExclude) == 0 {
		return true, fmt.Sprintf("%s %q matches %s", refNoun(ev.Kind), ev.RefName, r.RefPattern)
	}
	if !ev.FilesKnown || len(ev.Files) == 0 {
		return true, "matched (changed files unknown, path filters skipped)"
	}
	include, err := compileAll(r.PathsInclude, true)
	if err != nil {
		return false, "invalid include pattern: " + err.Error()
	}
	exclude, err := compileAll(r.PathsExclude, true)
	if err != nil {
		return false, "invalid exclude pattern: " + err.Error()
	}
	for _, f := range ev.Files {
		if anyMatch(exclude, f) {
			continue
		}
		if len(include) == 0 || anyMatch(include, f) {
			return true, fmt.Sprintf("matched (%s changed)", f)
		}
	}
	return false, "no changed file passes the path filters"
}

func refNoun(kind string) string {
	if kind == KindTag {
		return "tag"
	}
	return "branch"
}

func eventLabel(kind string) string {
	if kind == KindTag {
		return "tag"
	}
	return "branch push"
}

// ValidateRule checks user-supplied rule fields. It returns (field, message) on error.
func ValidateRule(r *store.TriggerRule) (string, string) {
	r.Name = strings.TrimSpace(r.Name)
	r.SkipKeyword = strings.TrimSpace(r.SkipKeyword)
	if r.Name == "" || len(r.Name) > 100 {
		return "name", "must be 1–100 characters"
	}
	if r.Event != KindPush && r.Event != KindTag {
		return "event", "must be push or tag"
	}
	refs := SplitPatterns(r.RefPattern)
	if len(refs) == 0 {
		return "ref_pattern", "is required, e.g. main or release/*"
	}
	if _, err := compileAll(refs, false); err != nil {
		return "ref_pattern", err.Error()
	}
	r.RefPattern = strings.Join(refs, ", ")

	clean := func(list []string) []string {
		out := []string{}
		for _, p := range list {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	r.PathsInclude, r.PathsExclude = clean(r.PathsInclude), clean(r.PathsExclude)
	if _, err := compileAll(r.PathsInclude, true); err != nil {
		return "paths_include", err.Error()
	}
	if _, err := compileAll(r.PathsExclude, true); err != nil {
		return "paths_exclude", err.Error()
	}
	if len(r.SkipKeyword) > 100 {
		return "skip_keyword", "must be at most 100 characters"
	}
	for k, v := range r.ExtraEnv {
		if !validEnvName(k) {
			return "extra_env", fmt.Sprintf("%q is not a valid variable name", k)
		}
		up := strings.ToUpper(k)
		if strings.HasPrefix(up, "DEPLOY_") || strings.HasPrefix(up, "DEGOPLOY") || up == "GIT_SSH_COMMAND" {
			return "extra_env", fmt.Sprintf("%s is set by degoploy and cannot be overridden", k)
		}
		if len(v) > 4096 {
			return "extra_env", fmt.Sprintf("value of %s is too long", k)
		}
	}
	return "", ""
}

func validEnvName(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for i, c := range s {
		switch {
		case c == '_', c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}
