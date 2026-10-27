package trigger

import (
	"testing"

	"github.com/degovan/degoploy/internal/store"
)

func TestGlob(t *testing.T) {
	cases := []struct {
		pattern string
		path    bool
		in      string
		want    bool
	}{
		{"main", false, "main", true},
		{"main", false, "main2", false},
		{"release/*", false, "release/1.0", true},
		{"release/*", false, "release/1.0/hotfix", false},
		{"release/**", false, "release/1.0/hotfix", true},
		{"v*", false, "v1.2.3", true},
		{"v?.*", false, "v1.2", true},
		{"src/**", true, "src/a/b.go", true},
		{"src/**", true, "lib/a.go", false},
		{"**/*.go", true, "main.go", true},
		{"**/*.go", true, "cmd/x/main.go", true},
		{"*.md", true, "docs/guide/readme.md", true},
		{"docs/*.md", true, "docs/guide/readme.md", false},
		{"/go.mod", true, "go.mod", true},
		{"a.b", true, "aXb", false},
	}
	for _, c := range cases {
		g, err := CompileGlob(c.pattern, c.path)
		if err != nil {
			t.Fatalf("%s: %v", c.pattern, err)
		}
		if got := g.Match(c.in); got != c.want {
			t.Errorf("%q.Match(%q) = %v, want %v", c.pattern, c.in, got, c.want)
		}
	}
	if _, err := CompileGlob("feat/[ab]", false); err == nil {
		t.Error("character classes should be rejected")
	}
}

func rule(id int64, mut func(*store.TriggerRule)) *store.TriggerRule {
	r := &store.TriggerRule{ID: id, Name: "r", Enabled: true, Event: KindPush, RefPattern: "main", SkipKeyword: "[skip deploy]"}
	if mut != nil {
		mut(r)
	}
	return r
}

func TestEvaluate(t *testing.T) {
	push := func(mut func(*Push)) Push {
		p := Push{Kind: KindPush, RefName: "main", Message: "fix bug", Files: []string{"src/app.go"}, FilesKnown: true}
		if mut != nil {
			mut(&p)
		}
		return p
	}
	cases := []struct {
		name string
		rule *store.TriggerRule
		ev   Push
		want bool
	}{
		{"plain match", rule(1, nil), push(nil), true},
		{"other branch", rule(1, nil), push(func(p *Push) { p.RefName = "dev" }), false},
		{"multi pattern", rule(1, func(r *store.TriggerRule) { r.RefPattern = "dev, main" }), push(nil), true},
		{"disabled", rule(1, func(r *store.TriggerRule) { r.Enabled = false }), push(nil), false},
		{"tag vs push", rule(1, func(r *store.TriggerRule) { r.Event = KindTag; r.RefPattern = "v*" }), push(nil), false},
		{"tag match", rule(1, func(r *store.TriggerRule) { r.Event = KindTag; r.RefPattern = "v*" }),
			push(func(p *Push) { p.Kind, p.RefName = KindTag, "v1.0.0" }), true},
		{"skip keyword", rule(1, nil), push(func(p *Push) { p.Message = "docs [skip deploy]" }), false},
		{"include hit", rule(1, func(r *store.TriggerRule) { r.PathsInclude = []string{"src/**"} }), push(nil), true},
		{"include miss", rule(1, func(r *store.TriggerRule) { r.PathsInclude = []string{"web/**"} }), push(nil), false},
		{"all excluded", rule(1, func(r *store.TriggerRule) { r.PathsExclude = []string{"*.md"} }),
			push(func(p *Push) { p.Files = []string{"README.md", "docs/a.md"} }), false},
		{"some not excluded", rule(1, func(r *store.TriggerRule) { r.PathsExclude = []string{"*.md"} }),
			push(func(p *Push) { p.Files = []string{"README.md", "main.go"} }), true},
		{"include but excluded", rule(1, func(r *store.TriggerRule) {
			r.PathsInclude, r.PathsExclude = []string{"src/**"}, []string{"src/**/*_test.go"}
		}), push(func(p *Push) { p.Files = []string{"src/a/x_test.go"} }), false},
		{"files unknown", rule(1, func(r *store.TriggerRule) { r.PathsInclude = []string{"web/**"} }),
			push(func(p *Push) { p.FilesKnown = false }), true},
	}
	for _, c := range cases {
		got, results := Evaluate([]*store.TriggerRule{c.rule}, c.ev)
		if (got != nil) != c.want {
			t.Errorf("%s: matched = %v, want %v (%s)", c.name, got != nil, c.want, results[0].Reason)
		}
	}
}

func TestEvaluateFirstRuleWins(t *testing.T) {
	rules := []*store.TriggerRule{
		rule(1, func(r *store.TriggerRule) { r.RefPattern = "dev" }),
		rule(2, nil),
		rule(3, func(r *store.TriggerRule) { r.RefPattern = "**" }),
	}
	got, results := Evaluate(rules, Push{Kind: KindPush, RefName: "main"})
	if got == nil || got.ID != 2 {
		t.Fatalf("winner = %+v", got)
	}
	if results[0].Matched || !results[1].Matched || results[2].Matched {
		t.Fatalf("results = %+v", results)
	}
}

func TestValidateRule(t *testing.T) {
	bad := map[string]func(*store.TriggerRule){
		"name":          func(r *store.TriggerRule) { r.Name = " " },
		"event":         func(r *store.TriggerRule) { r.Event = "release" },
		"ref_pattern":   func(r *store.TriggerRule) { r.RefPattern = " , " },
		"paths_include": func(r *store.TriggerRule) { r.PathsInclude = []string{"src/[a]"} },
		"extra_env":     func(r *store.TriggerRule) { r.ExtraEnv = map[string]string{"DEPLOY_ID": "x"} },
	}
	for field, mut := range bad {
		r := rule(0, mut)
		if got, _ := ValidateRule(r); got != field {
			t.Errorf("%s: field = %q", field, got)
		}
	}
	r := rule(0, func(r *store.TriggerRule) {
		r.RefPattern = "main,\n release/*"
		r.ExtraEnv = map[string]string{"APP_ENV": "prod"}
	})
	if f, msg := ValidateRule(r); f != "" {
		t.Fatalf("valid rule rejected: %s %s", f, msg)
	}
	if r.RefPattern != "main, release/*" {
		t.Fatalf("ref pattern not normalised: %q", r.RefPattern)
	}
}
