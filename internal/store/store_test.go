package store

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
)

func TestBackup(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(ctx, filepath.Join(dir, "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateUser(ctx, &User{Username: "ana", AuthProvider: ProviderLocal, Role: RoleAdmin, PasswordHash: "x"}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "backup.db")
	if err := s.Backup(ctx, out); err != nil {
		t.Fatal(err)
	}
	if err := s.Backup(ctx, out); err == nil {
		t.Fatal("backup over an existing file must fail")
	}
	b, err := Open(ctx, out)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if u, err := b.UserByUsername(ctx, "ana"); err != nil || u.Role != RoleAdmin {
		t.Fatalf("restored user: %+v %v", u, err)
	}
}

func TestDeploymentHealth(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p := &Project{PublicID: "pub", Slug: "app", Name: "App", Provider: "github", RepoURL: "git@x:a/b.git",
		WorkDir: "/srv/app", ScriptSource: ScriptManaged, WebhookSecret: "s", AutoPull: true, TimeoutSec: 60, Enabled: true}
	if err := s.CreateProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	// Oldest first; skipped/canceled runs do not count, only the newest 3 do.
	for _, st := range []string{DeployFailed, DeploySuccess, DeploySkipped, DeployTimeout, DeploySuccess, DeployCanceled} {
		d := &Deployment{ProjectID: p.ID, Trigger: TriggerManual, Ref: "refs/heads/main"}
		if err := s.CreateDeployment(ctx, d); err != nil {
			t.Fatal(err)
		}
		if _, err := s.StartDeployment(ctx, d.ID, "", true); err != nil {
			t.Fatal(err)
		}
		if err := s.FinishDeployment(ctx, d.ID, st, nil, ""); err != nil {
			t.Fatal(err)
		}
	}
	h, err := s.DeploymentHealth(ctx, p.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := h[p.ID]; got != (Health{Passed: 2, Total: 3}) {
		t.Fatalf("health = %+v, want 2 of 3", got)
	}
	if all, _ := s.DeploymentHealth(ctx, 0, 10); all[p.ID] != (Health{Passed: 2, Total: 4}) {
		t.Fatalf("health (all) = %+v, want 2 of 4", all[p.ID])
	}

	queued := &Deployment{ProjectID: p.ID, Trigger: TriggerManual, Ref: "refs/heads/main"}
	if err := s.CreateDeployment(ctx, queued); err != nil {
		t.Fatal(err)
	}
	active, err := s.RecentDeployments(ctx, 10, true)
	if err != nil || len(active) != 1 || active[0].ID != queued.ID || active[0].AutoPull != nil {
		t.Fatalf("active = %+v, err %v", active, err)
	}
	if recent, _ := s.RecentDeployments(ctx, 10, false); len(recent) != 7 || recent[1].AutoPull == nil || !*recent[1].AutoPull {
		t.Fatalf("recent = %d deployments, auto_pull %v", len(recent), recent[1].AutoPull)
	}
}

func TestDeploymentNeighbors(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	newProject := func(slug string) *Project {
		p := &Project{PublicID: slug, Slug: slug, Name: slug, Provider: "github", RepoURL: "git@x:a/b.git",
			WorkDir: "/srv/" + slug, ScriptSource: ScriptManaged, WebhookSecret: "s", AutoPull: true, TimeoutSec: 60, Enabled: true}
		if err := s.CreateProject(ctx, p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a, b := newProject("a"), newProject("b")
	// Interleaved: a's deployments are 1, 3, 4; b's is 2.
	var ids []int64
	for _, p := range []*Project{a, b, a, a} {
		d := &Deployment{ProjectID: p.ID, Trigger: TriggerManual}
		if err := s.CreateDeployment(ctx, d); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, d.ID)
	}
	str := func(p *int64) string {
		if p == nil {
			return "nil"
		}
		return strconv.FormatInt(*p, 10)
	}
	for _, c := range []struct {
		project, id int64
		prev, next  string
	}{
		{a.ID, ids[0], "nil", str(&ids[2])},
		{a.ID, ids[2], str(&ids[0]), str(&ids[3])},
		{a.ID, ids[3], str(&ids[2]), "nil"},
		{b.ID, ids[1], "nil", "nil"},
	} {
		prev, next, err := s.DeploymentNeighbors(ctx, c.project, c.id)
		if err != nil || str(prev) != c.prev || str(next) != c.next {
			t.Errorf("neighbors of %d = %s, %s (%v), want %s, %s", c.id, str(prev), str(next), err, c.prev, c.next)
		}
	}
}

func TestLastRuleMatches(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p := &Project{PublicID: "a", Slug: "a", Name: "a", Provider: "github", RepoURL: "git@x:a/b.git",
		WorkDir: "/srv/a", ScriptSource: ScriptManaged, WebhookSecret: "s", AutoPull: true, TimeoutSec: 60, Enabled: true}
	if err := s.CreateProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	var rules []*TriggerRule
	for _, name := range []string{"main", "tags", "never"} {
		r := &TriggerRule{ProjectID: p.ID, Name: name, Enabled: true, Event: "push", RefPattern: "main"}
		if err := s.CreateRule(ctx, r); err != nil {
			t.Fatal(err)
		}
		rules = append(rules, r)
	}
	deploy := func(rule *TriggerRule, trigger string) int64 {
		d := &Deployment{ProjectID: p.ID, RuleID: &rule.ID, Trigger: trigger}
		if err := s.CreateDeployment(ctx, d); err != nil {
			t.Fatal(err)
		}
		return d.ID
	}
	deploy(rules[0], TriggerWebhook)
	latest := deploy(rules[0], TriggerWebhook)
	tag := deploy(rules[1], TriggerWebhook)
	deploy(rules[0], TriggerManual) // a manual deploy is not a match

	got, err := s.LastRuleMatches(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[rules[0].ID].DeploymentID != latest || got[rules[1].ID].DeploymentID != tag ||
		got[rules[0].ID].At.IsZero() {
		t.Fatalf("LastRuleMatches = %+v, want rule %d → %d, rule %d → %d", got, rules[0].ID, latest, rules[1].ID, tag)
	}
}

func TestVariables(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p := &Project{PublicID: "a", Slug: "a", Name: "a", Provider: "github", RepoURL: "git@x:a/b.git",
		WorkDir: "/srv/a", ScriptSource: ScriptManaged, WebhookSecret: "s", AutoPull: true, TimeoutSec: 60, Enabled: true}
	if err := s.CreateProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	db := &Variable{ProjectID: p.ID, Name: "DB_PASSWORD", Value: "v1:sealed", Secret: true}
	url := &Variable{ProjectID: p.ID, Name: "APP_URL", Value: "https://a.test"}
	for _, v := range []*Variable{db, url} {
		if err := s.CreateVariable(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CreateVariable(ctx, &Variable{ProjectID: p.ID, Name: "APP_URL", Value: "x"}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate name: err = %v, want ErrDuplicate", err)
	}

	list, err := s.ListVariables(ctx, p.ID)
	if err != nil || len(list) != 2 || list[0].Name != "APP_URL" || !list[1].Secret || list[1].Value != "v1:sealed" {
		t.Fatalf("ListVariables = %+v, %v", list, err)
	}

	url.Name, url.Value = "BASE_URL", "https://b.test"
	if err := s.UpdateVariable(ctx, url); err != nil {
		t.Fatal(err)
	}
	if got, err := s.VariableByID(ctx, p.ID, url.ID); err != nil || got.Name != "BASE_URL" || got.Value != "https://b.test" {
		t.Fatalf("after update: %+v, %v", got, err)
	}
	url.Name = "DB_PASSWORD"
	if err := s.UpdateVariable(ctx, url); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("rename onto an existing name: err = %v, want ErrDuplicate", err)
	}

	if err := s.DeleteVariable(ctx, p.ID, db.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteVariable(ctx, p.ID, db.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: err = %v, want ErrNotFound", err)
	}
	// Deleting the project removes its variables.
	if err := s.DeleteProject(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListVariables(ctx, p.ID); len(list) != 0 {
		t.Fatalf("variables left after project delete: %+v", list)
	}
}
