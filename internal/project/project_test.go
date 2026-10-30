package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/degovan/degoploy/internal/store"
)

func newManager(t *testing.T) (*Manager, *store.Store, string) {
	t.Helper()
	dataDir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dataDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return NewManager(st, dataDir, filepath.Join(t.TempDir(), "projects")), st, dataDir
}

func validInput(workDir string) Input {
	return Input{
		Name: "My App", Slug: "my-app", Provider: "github",
		RepoURL: "git@github.com:acme/my-app.git", WorkDir: workDir,
		ScriptSource: store.ScriptManaged, AutoPull: true, TimeoutSec: 600, Enabled: true,
	}
}

func fieldOf(err error) string {
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve.Field
	}
	return ""
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name  string
		mut   func(*Input)
		field string
	}{
		{"ok", func(*Input) {}, ""},
		{"ok https", func(in *Input) { in.RepoURL = "https://gitea.example.com/acme/app.git" }, ""},
		{"ok ssh url", func(in *Input) { in.RepoURL = "ssh://git@gitea.example.com:2222/acme/app.git" }, ""},
		{"bad slug", func(in *Input) { in.Slug = "-nope" }, "slug"},
		{"bad provider", func(in *Input) { in.Provider = "gitlab" }, "provider"},
		{"option injection", func(in *Input) { in.RepoURL = "--upload-pack=touch /tmp/x" }, "repo_url"},
		{"ext transport", func(in *Input) { in.RepoURL = "ext::sh -c touch% /tmp/x" }, "repo_url"},
		{"file transport", func(in *Input) { in.RepoURL = "file:///etc" }, "repo_url"},
		{"relative workdir", func(in *Input) { in.WorkDir = "srv/app" }, "work_dir"},
		{"root workdir", func(in *Input) { in.WorkDir = "/" }, "work_dir"},
		{"workdir inside data", func(in *Input) { in.WorkDir = "/data/projects/x" }, "work_dir"},
		{"workdir containing data", func(in *Input) { in.WorkDir = "/" + "data/.." + "/" }, "work_dir"},
		{"workdir inside projects", func(in *Input) { in.WorkDir = "/srv/degoploy/app" }, ""},
		{"workdir is projects", func(in *Input) { in.WorkDir = "/srv/degoploy" }, "work_dir"},
		{"workdir containing projects", func(in *Input) { in.WorkDir = "/srv" }, "work_dir"},
		{"workdir missing", func(in *Input) { in.WorkDir = "" }, "work_dir"},
		{"repo needs auto pull", func(in *Input) {
			in.ScriptSource, in.ScriptPath, in.AutoPull = store.ScriptRepo, "deploy.sh", false
		}, "auto_pull"},
		{"repo path escapes", func(in *Input) { in.ScriptSource, in.ScriptPath = store.ScriptRepo, "../x.sh" }, "script_path"},
		{"repo path absolute", func(in *Input) { in.ScriptSource, in.ScriptPath = store.ScriptRepo, "/bin/sh" }, "script_path"},
		{"timeout", func(in *Input) { in.TimeoutSec = 1 }, "timeout_sec"},
	}
	for _, c := range cases {
		in := validInput("/srv/app")
		c.mut(&in)
		in.normalize()
		if got := fieldOf(in.validate("/data", "/srv/degoploy", true)); got != c.field {
			t.Errorf("%s: field = %q, want %q", c.name, got, c.field)
		}
	}
}

func TestCreateUpdateDelete(t *testing.T) {
	ctx := context.Background()
	m, st, _ := newManager(t)
	root := t.TempDir()

	p, err := m.Create(ctx, validInput(filepath.Join(root, "my-app")), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.PublicID) < 20 || len(p.WebhookSecret) != 64 {
		t.Fatalf("weak identifiers: %q %q", p.PublicID, p.WebhookSecret)
	}

	st1, err := os.Stat(m.KeyPath(p))
	if err != nil || st1.Mode().Perm() != 0o600 {
		t.Fatalf("private key perms: %v %v", st1, err)
	}
	if d, _ := os.Stat(m.Dir(p)); d.Mode().Perm() != 0o700 {
		t.Fatalf("project dir perms = %v", d.Mode().Perm())
	}
	key, fp, err := m.PublicKey(p)
	if err != nil || !strings.HasPrefix(key, "ssh-ed25519 ") || !strings.HasSuffix(key, " degoploy-my-app") || !strings.HasPrefix(fp, "SHA256:") {
		t.Fatalf("public key = %q %q %v", key, fp, err)
	}

	s, err := m.ReadScript(p)
	if err != nil || !s.Exists || !strings.Contains(s.Content, "set -euo pipefail") {
		t.Fatalf("default script: %+v %v", s, err)
	}
	revs, _ := st.ListScriptRevisions(ctx, p.ID, 10)
	if len(revs) != 1 {
		t.Fatalf("revisions = %d, want 1", len(revs))
	}

	// Same slug or overlapping work_dir must be rejected.
	if _, err := m.Create(ctx, validInput(filepath.Join(root, "other")), 0); fieldOf(err) != "slug" {
		t.Fatalf("duplicate slug: %v", err)
	}
	other := validInput(filepath.Join(root, "my-app", "sub"))
	other.Slug = "other"
	if _, err := m.Create(ctx, other, 0); fieldOf(err) != "work_dir" {
		t.Fatalf("nested work_dir: %v", err)
	}

	// Key rotation changes the key.
	if err := m.RegenerateKey(p); err != nil {
		t.Fatal(err)
	}
	if key2, _, _ := m.PublicKey(p); key2 == key {
		t.Fatal("key did not change")
	}

	// CRLF is normalised and each save adds a revision.
	if _, err := m.SaveScript(ctx, p, "echo hi\r\necho bye", 0); err != nil {
		t.Fatal(err)
	}
	if s, _ := m.ReadScript(p); s.Content != "echo hi\necho bye\n" {
		t.Fatalf("saved content = %q", s.Content)
	}

	// Switching to a repo script makes the managed one read-only.
	in := InputFrom(p)
	in.ScriptSource, in.ScriptPath = store.ScriptRepo, ".degoploy/deploy.sh"
	if err := m.Update(ctx, p, in, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SaveScript(ctx, p, "x", 0); fieldOf(err) != "content" {
		t.Fatalf("save on repo script: %v", err)
	}
	if s, err := m.ReadScript(p); err != nil || s.Exists {
		t.Fatalf("missing repo script should read as not-exists: %+v %v", s, err)
	}

	dir := m.Dir(p)
	if err := m.Delete(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("project dir still exists: %v", err)
	}
}

func TestDefaultWorkDir(t *testing.T) {
	ctx := context.Background()
	m, _, _ := newManager(t)

	p, err := m.Create(ctx, validInput(""), 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(m.ProjectsDir(), "my-app"); p.WorkDir != want {
		t.Fatalf("work_dir = %q, want %q", p.WorkDir, want)
	}

	// Clearing it on update goes back to the default.
	in := InputFrom(p)
	in.WorkDir = t.TempDir()
	if err := m.Update(ctx, p, in, 0); err != nil {
		t.Fatal(err)
	}
	in.WorkDir = ""
	if err := m.Update(ctx, p, in, 0); err != nil || p.WorkDir != filepath.Join(m.ProjectsDir(), "my-app") {
		t.Fatalf("cleared work_dir = %q, %v", p.WorkDir, err)
	}
}

func TestWorkDirAccess(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root may write anywhere")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	closed := filepath.Join(root, "closed")
	notGit := filepath.Join(root, "not-git")
	file := filepath.Join(root, "file")
	git := filepath.Join(root, "git")
	empty := filepath.Join(root, "empty")
	os.Mkdir(empty, 0o755)
	os.Mkdir(locked, 0o555)
	os.Mkdir(closed, 0o700)
	os.Mkdir(filepath.Join(closed, "app"), 0o755)
	os.Chmod(closed, 0o600)
	t.Cleanup(func() { os.Chmod(closed, 0o700) })
	os.Mkdir(notGit, 0o755)
	os.WriteFile(filepath.Join(notGit, "index.php"), nil, 0o644)
	os.WriteFile(file, nil, 0o644)
	os.MkdirAll(filepath.Join(git, ".git"), 0o755)

	cases := []struct {
		dir  string
		want string // part of the message; empty means accepted
	}{
		{filepath.Join(root, "new", "app"), ""},
		{empty, ""},
		{git, ""},
		{filepath.Join(locked, "app"), "sudo install -d"},
		{filepath.Join(closed, "app"), "setfacl -m"},
		{locked, "setfacl -R"},
		{notGit, "not a git repository"},
		{file, "is a file"},
		{filepath.Join(file, "app"), "is a file"},
	}
	for _, c := range cases {
		err := CheckWorkDirAccess(c.dir)
		var ve *ValidationError
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: %v", c.dir, err)
		case c.want != "" && (!errors.As(err, &ve) || ve.Field != "work_dir" || !strings.Contains(ve.Message, c.want)):
			t.Errorf("%s: err = %v, want %q", c.dir, err, c.want)
		}
	}
}

func TestResolveRepoScriptRejectsSymlinkEscape(t *testing.T) {
	work := t.TempDir()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "evil.sh"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(work, "ok.sh"), []byte("x"), 0o644)
	os.Symlink(filepath.Join(outside, "evil.sh"), filepath.Join(work, "link.sh"))

	if _, err := ResolveRepoScript(work, "ok.sh"); err != nil {
		t.Fatalf("ok.sh: %v", err)
	}
	if _, err := ResolveRepoScript(work, "link.sh"); err == nil {
		t.Fatal("symlink escaping work_dir must be rejected")
	}
}
