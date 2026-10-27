package store

import (
	"context"
	"path/filepath"
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
