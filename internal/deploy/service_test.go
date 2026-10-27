package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/degovan/degoploy/internal/project"
	"github.com/degovan/degoploy/internal/realtime"
	"github.com/degovan/degoploy/internal/store"
)

// fixture is a git repository served over git's "dumb" HTTP protocol plus a
// degoploy store, project manager and runner.
type fixture struct {
	t       *testing.T
	src     string // working clone used to make commits
	bare    string
	url     string
	store   *store.Store
	manager *project.Manager
	hub     *realtime.Hub
	svc     *Service
	root    string
}

func run(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	f := &fixture{t: t, root: root, src: filepath.Join(root, "src"), bare: filepath.Join(root, "served", "app.git")}

	os.MkdirAll(f.src, 0o755)
	run(t, f.src, "git", "init", "-q", "-b", "main")
	f.commit("README.md", "v1\n", "first")
	run(t, root, "git", "clone", "-q", "--bare", f.src, f.bare)
	run(t, f.src, "git", "remote", "add", "origin", f.bare)
	run(t, f.bare, "git", "update-server-info")

	srv := httptest.NewServer(http.FileServer(http.Dir(filepath.Join(root, "served"))))
	t.Cleanup(srv.Close)
	f.url = srv.URL + "/app.git"

	dataDir := filepath.Join(root, "data")
	os.MkdirAll(dataDir, 0o700)
	st, err := store.Open(context.Background(), filepath.Join(dataDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	f.store = st
	f.manager = project.NewManager(st, dataDir)
	f.hub = realtime.NewHub()
	f.svc = NewService(st, f.manager, f.hub, 2)
	if err := f.svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.svc.Shutdown(5 * time.Second) })
	return f
}

// commit writes a file in the source clone, commits and publishes it.
func (f *fixture) commit(name, content, msg string) string {
	os.WriteFile(filepath.Join(f.src, name), []byte(content), 0o644)
	run(f.t, f.src, "git", "add", name)
	run(f.t, f.src, "git", "commit", "-q", "-m", msg)
	if f.bare != "" {
		if _, err := os.Stat(f.bare); err == nil {
			run(f.t, f.src, "git", "push", "-q", "origin", "main")
			run(f.t, f.bare, "git", "update-server-info")
		}
	}
	return run(f.t, f.src, "git", "rev-parse", "HEAD")
}

func (f *fixture) project(slug, script string, timeout int) *store.Project {
	f.t.Helper()
	p, err := f.manager.Create(context.Background(), project.Input{
		Name: slug, Slug: slug, Provider: "gitea", RepoURL: f.url,
		WorkDir: filepath.Join(f.root, "www", slug), ScriptSource: store.ScriptManaged,
		AutoPull: true, TimeoutSec: 600, Enabled: true,
	}, 0)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.manager.SaveScript(context.Background(), p, script, 0); err != nil {
		f.t.Fatal(err)
	}
	if timeout != 0 {
		p.TimeoutSec = timeout // below the UI minimum, to keep the test fast
		if err := f.store.UpdateProject(context.Background(), p); err != nil {
			f.t.Fatal(err)
		}
	}
	return p
}

func (f *fixture) enqueue(p *store.Project, rule *store.TriggerRule) *store.Deployment {
	f.t.Helper()
	d, err := f.svc.Enqueue(context.Background(), Request{Project: p, Rule: rule, Trigger: store.TriggerManual, Ref: "refs/heads/main"})
	if err != nil {
		f.t.Fatal(err)
	}
	return d
}

func (f *fixture) wait(id int64, statuses ...string) *store.Deployment {
	f.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		d, err := f.store.DeploymentByID(context.Background(), id)
		if err != nil {
			f.t.Fatal(err)
		}
		for _, s := range statuses {
			if d.Status == s {
				return d
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	d, _ := f.store.DeploymentByID(context.Background(), id)
	f.t.Fatalf("deployment %d: status %q, want one of %v", id, d.Status, statuses)
	return nil
}

func readLog(t *testing.T, d *store.Deployment) string {
	t.Helper()
	b, err := os.ReadFile(d.LogPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDeploySuccess(t *testing.T) {
	f := newFixture(t)
	sha := f.commit("app.txt", "hello\n", "add app")
	p := f.project("app", `echo "branch=$DEPLOY_BRANCH commit=$DEPLOY_COMMIT env=$APP_ENV rule=$DEPLOY_RULE secret=${DEGOPLOY_TEST_SECRET:-none}" > deployed.txt
cat app.txt`, 0)
	rule := &store.TriggerRule{ProjectID: p.ID, Name: "prod", Enabled: true, Event: "push", RefPattern: "main",
		ExtraEnv: map[string]string{"APP_ENV": "production"}}
	if err := f.store.CreateRule(context.Background(), rule); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEGOPLOY_TEST_SECRET", "leaked")

	d := f.wait(f.enqueue(p, rule).ID, store.DeploySuccess, store.DeployFailed)
	log := readLog(t, d)
	if d.Status != store.DeploySuccess || *d.ExitCode != 0 {
		t.Fatalf("status %s\n%s", d.Status, log)
	}
	if d.CommitSHA != sha {
		t.Fatalf("commit = %s, want %s", d.CommitSHA, sha)
	}
	got, _ := os.ReadFile(filepath.Join(p.WorkDir, "deployed.txt"))
	want := "branch=main commit=" + sha + " env=production rule=prod secret=none\n"
	if string(got) != want {
		t.Fatalf("deployed.txt = %q, want %q", got, want)
	}
	if !strings.Contains(log, "hello") || !strings.Contains(log, "Deployment succeeded") {
		t.Fatalf("log:\n%s", log)
	}

	// A second deploy fetches the new commit into the existing checkout.
	sha2 := f.commit("app.txt", "hello v2\n", "update app")
	d2 := f.wait(f.enqueue(p, rule).ID, store.DeploySuccess, store.DeployFailed)
	if d2.Status != store.DeploySuccess || d2.CommitSHA != sha2 {
		t.Fatalf("second deploy: %s %s\n%s", d2.Status, d2.CommitSHA, readLog(t, d2))
	}
}

func TestDeployFailureAndTimeout(t *testing.T) {
	f := newFixture(t)
	failing := f.project("failing", "echo boom >&2\nexit 3", 0)
	d := f.wait(f.enqueue(failing, nil).ID, store.DeploySuccess, store.DeployFailed)
	if d.Status != store.DeployFailed || d.ExitCode == nil || *d.ExitCode != 3 {
		t.Fatalf("failing: %s %v", d.Status, d.ExitCode)
	}
	if log := readLog(t, d); !strings.Contains(log, "boom") || !strings.Contains(log, "exited with code 3") {
		t.Fatalf("log:\n%s", log)
	}

	slow := f.project("slow", "sleep 30 &\nwait", 1)
	start := time.Now()
	d = f.wait(f.enqueue(slow, nil).ID, store.DeployTimeout, store.DeploySuccess, store.DeployFailed)
	if d.Status != store.DeployTimeout {
		t.Fatalf("slow: %s\n%s", d.Status, readLog(t, d))
	}
	if took := time.Since(start); took > 8*time.Second {
		t.Fatalf("timeout took %s; the process group was not killed promptly", took)
	}
}

func TestCoalesceAndCancel(t *testing.T) {
	f := newFixture(t)
	p := f.project("busy", "sleep 3", 0)

	a := f.enqueue(p, nil)
	f.wait(a.ID, store.DeployRunning)
	b := f.enqueue(p, nil)
	c := f.enqueue(p, nil)

	if d := f.wait(b.ID, store.DeploySkipped); d.FinishedAt == nil {
		t.Fatal("skipped deployment should be finished")
	}
	if ok, err := f.svc.Cancel(context.Background(), a.ID); !ok || err != nil {
		t.Fatalf("cancel running: %v %v", ok, err)
	}
	if d := f.wait(a.ID, store.DeployCanceled, store.DeploySuccess); d.Status != store.DeployCanceled {
		t.Fatalf("a = %s", d.Status)
	}
	f.wait(c.ID, store.DeployRunning)
	if ok, _ := f.svc.Cancel(context.Background(), c.ID); !ok {
		t.Fatal("cancel c")
	}
	f.wait(c.ID, store.DeployCanceled)
	if ok, _ := f.svc.Cancel(context.Background(), c.ID); ok {
		t.Fatal("canceling a finished deployment must report false")
	}
}

func TestRefusesNonGitWorkDir(t *testing.T) {
	f := newFixture(t)
	p := f.project("occupied", "true", 0)
	os.MkdirAll(p.WorkDir, 0o755)
	os.WriteFile(filepath.Join(p.WorkDir, "important.txt"), []byte("keep me"), 0o644)

	d := f.wait(f.enqueue(p, nil).ID, store.DeployFailed, store.DeploySuccess)
	if d.Status != store.DeployFailed || !strings.Contains(readLog(t, d), "refusing to clone") {
		t.Fatalf("status %s\n%s", d.Status, readLog(t, d))
	}
	if b, _ := os.ReadFile(filepath.Join(p.WorkDir, "important.txt")); string(b) != "keep me" {
		t.Fatal("existing files must be left alone")
	}
}

func TestLogStreamMatchesFile(t *testing.T) {
	f := newFixture(t)
	p := f.project("chatty", `for i in $(seq 1 200); do echo "line $i ünïcödé"; done`, 0)

	// The deployment id is predictable: subscribe before it starts.
	client := realtime.NewClient(func(int64) bool { return true })
	f.hub.Register(client)
	var mu sync.Mutex
	var stream bytes.Buffer
	var ended bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		for b := range client.Outbox() {
			var m struct {
				Type   string
				Offset int64
				Data   []byte
			}
			json.Unmarshal(b, &m)
			mu.Lock()
			switch m.Type {
			case realtime.TypeLogChunk:
				if int64(stream.Len()) != m.Offset {
					t.Errorf("chunk at offset %d, have %d bytes", m.Offset, stream.Len())
				}
				stream.Write(m.Data)
			case realtime.TypeLogEnd:
				ended = true
			}
			mu.Unlock()
		}
	}()
	f.hub.Subscribe(client, realtime.DeploymentLogTopic(1))

	d := f.wait(f.enqueue(p, nil).ID, store.DeploySuccess, store.DeployFailed)
	if d.ID != 1 {
		t.Fatalf("expected deployment 1, got %d", d.ID)
	}
	time.Sleep(100 * time.Millisecond)
	f.hub.Unregister(client)
	<-done

	mu.Lock()
	defer mu.Unlock()
	if !ended {
		t.Fatal("no log.end message")
	}
	if file := readLog(t, d); stream.String() != file {
		t.Fatalf("streamed %d bytes, file has %d", stream.Len(), len(file))
	}
}
