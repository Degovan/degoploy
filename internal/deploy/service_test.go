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
	"github.com/degovan/degoploy/internal/secrets"
	"github.com/degovan/degoploy/internal/store"
	"github.com/degovan/degoploy/internal/toolchain"
	"github.com/degovan/degoploy/internal/toolchain/toolchaintest"
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
	box     *secrets.Box
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
	f.box, err = secrets.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	f.svc = NewService(st, f.manager, f.hub, f.box, 2)
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
	if d.AutoPull == nil || !*d.AutoPull {
		t.Fatalf("auto_pull = %v, want true", d.AutoPull)
	}
	if d.ScriptStartedAt == nil || d.ScriptStartedAt.Before(*d.StartedAt) || d.FinishedAt.Before(*d.ScriptStartedAt) || d.Error != "" {
		t.Fatalf("stages: started %v script %v finished %v error %q", d.StartedAt, d.ScriptStartedAt, d.FinishedAt, d.Error)
	}
	revs, _ := f.store.ListScriptRevisions(context.Background(), p.ID, 1)
	if d.ScriptRevisionID == nil || len(revs) != 1 || *d.ScriptRevisionID != revs[0].ID {
		t.Fatalf("script revision = %v, want the current revision", d.ScriptRevisionID)
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
	failing := f.project("failing", "printf '\\033[31mboom\\033[0m\\n\\n' >&2\nexit 3", 0)
	missing := f.project("missing", "no-such-command-xyz", 0)
	d := f.wait(f.enqueue(failing, nil).ID, store.DeploySuccess, store.DeployFailed)
	if d.Status != store.DeployFailed || d.ExitCode == nil || *d.ExitCode != 3 {
		t.Fatalf("failing: %s %v", d.Status, d.ExitCode)
	}
	if d.Error != "boom" {
		t.Fatalf("error = %q, want the script's last output line", d.Error)
	}
	m := f.wait(f.enqueue(missing, nil).ID, store.DeploySuccess, store.DeployFailed)
	if want := "deploy.sh: line 1: no-such-command-xyz: command not found"; m.Error != want || *m.ExitCode != 127 {
		t.Fatalf("error = %q (exit %v), want %q", m.Error, m.ExitCode, want)
	}
	if log := readLog(t, d); !strings.Contains(log, "boom") || !strings.Contains(log, "exited with code 3") {
		t.Fatalf("log:\n%s", log)
	}

	slow := f.project("slow", "sleep 30 &\nwait", 1)
	start := time.Now()
	d = f.wait(f.enqueue(slow, nil).ID, store.DeployTimeout, store.DeploySuccess, store.DeployFailed)
	if d.Status != store.DeployTimeout || !strings.HasPrefix(d.Error, "timed out after") {
		t.Fatalf("slow: %s %q\n%s", d.Status, d.Error, readLog(t, d))
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
	if !strings.Contains(d.Error, "refusing to clone") || d.ScriptStartedAt != nil {
		t.Fatalf("error %q script started %v", d.Error, d.ScriptStartedAt)
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
				Time   int64
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
				if m.Time == 0 {
					t.Errorf("live chunk at offset %d has no time", m.Offset)
				}
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
	file := readLog(t, d)
	if stream.String() != file {
		t.Fatalf("streamed %d bytes, file has %d", stream.Len(), len(file))
	}

	times, err := ReadLogTimes(d.LogPath)
	if err != nil || len(times) == 0 || times[0][0] != 0 {
		t.Fatalf("times = %v, err %v; want entries starting at offset 0", times, err)
	}
	for i, e := range times {
		if e[0] >= int64(len(file)) || e[1] < d.StartedAt.UnixMilli()-1000 || i > 0 && (e[0] <= times[i-1][0] || e[1] < times[i-1][1]) {
			t.Fatalf("entry %d = %v is out of order or range (log %d bytes)", i, e, len(file))
		}
	}
}

func TestProjectVariables(t *testing.T) {
	f := newFixture(t)
	p := f.project("vars", `echo "url=$APP_URL env=$APP_ENV"
echo "password is $DB_PASSWORD"
printf '%s' "$DB_PASSWORD" > secret.txt
echo "fatal: cannot connect as $DB_PASSWORD" >&2
exit 4`, 0)
	ctx := context.Background()
	for _, v := range []*store.Variable{
		{ProjectID: p.ID, Name: "APP_URL", Value: "https://app.test"},
		{ProjectID: p.ID, Name: "APP_ENV", Value: "staging"},
		{ProjectID: p.ID, Name: "DB_PASSWORD", Value: f.box.Seal("s3cr3t-pw", secrets.ProjectScope(p.ID)), Secret: true},
	} {
		if err := f.store.CreateVariable(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	rule := &store.TriggerRule{ProjectID: p.ID, Name: "prod", Enabled: true, Event: "push", RefPattern: "main",
		ExtraEnv: map[string]string{"APP_ENV": "production"}}
	if err := f.store.CreateRule(ctx, rule); err != nil {
		t.Fatal(err)
	}

	d := f.wait(f.enqueue(p, rule).ID, store.DeploySuccess, store.DeployFailed)
	log := readLog(t, d)
	if d.Status != store.DeployFailed {
		t.Fatalf("status %s\n%s", d.Status, log)
	}
	// The script got the decrypted value; the rule overrides the project variable.
	if b, _ := os.ReadFile(filepath.Join(p.WorkDir, "secret.txt")); string(b) != "s3cr3t-pw" {
		t.Fatalf("script saw DB_PASSWORD = %q", b)
	}
	for _, want := range []string{"url=https://app.test env=production", "password is ***",
		"variables: APP_ENV, APP_URL, DB_PASSWORD"} {
		if !strings.Contains(log, want) {
			t.Fatalf("log lacks %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "s3cr3t-pw") || strings.Contains(d.Error, "s3cr3t-pw") {
		t.Fatalf("secret leaked; error %q, log:\n%s", d.Error, log)
	}
	if d.Error != "fatal: cannot connect as ***" {
		t.Fatalf("error summary = %q", d.Error)
	}

	// A secret sealed with another key fails the deployment before it runs anything.
	if err := f.store.CreateVariable(ctx, &store.Variable{ProjectID: p.ID, Name: "BROKEN",
		Value: f.box.Seal("x", secrets.ProjectScope(p.ID+1)), Secret: true}); err != nil {
		t.Fatal(err)
	}
	d = f.wait(f.enqueue(p, nil).ID, store.DeploySuccess, store.DeployFailed)
	if d.Status != store.DeployFailed || !strings.Contains(d.Error, "secret BROKEN") || d.ScriptStartedAt != nil {
		t.Fatalf("broken secret: status %s error %q script started %v", d.Status, d.Error, d.ScriptStartedAt)
	}
}

func TestDeployRuntimes(t *testing.T) {
	f := newFixture(t)
	nodeMirror := toolchaintest.NewNodeMirror(t,
		toolchaintest.Release{Version: "v22.12.0", LTS: "Jod"},
		toolchaintest.Release{Version: "v20.19.0", LTS: "Iron"},
	)
	phpMirror := toolchaintest.NewPHPMirror(t, "8.4.23", "8.3.30")
	data := filepath.Join(f.root, "data")
	f.svc.Runtimes = toolchain.NewSetOf(
		toolchain.New(toolchain.NodeJS{}, filepath.Join(data, "node"), nodeMirror.URL),
		toolchain.New(&toolchain.PHP{ComposerURL: phpMirror.URL + "/composer.phar"}, filepath.Join(data, "php"), phpMirror.URL),
	)
	ctx := context.Background()
	script := `echo "node=$(node 2>/dev/null) npm=$(npm 2>/dev/null) php=$(php -n -r 'echo PHP_VERSION;' 2>/dev/null) composer=$(command -v composer >/dev/null && echo yes)" > runtimes.txt`
	got := func(p *store.Project) string {
		b, _ := os.ReadFile(filepath.Join(p.WorkDir, "runtimes.txt"))
		return string(b)
	}

	// A repository without package.json or composer.json uses neither.
	p := f.project("web", script, 0)
	d := f.wait(f.enqueue(p, nil).ID, store.DeploySuccess, store.DeployFailed)
	if log := readLog(t, d); strings.Contains(log, "Installing") || strings.Contains(log, "node: ") || strings.Contains(log, "php: ") {
		t.Fatalf("a runtime was set up for a bare repository:\n%s", log)
	}

	// .nvmrc and composer.json pick the versions, installed on first use.
	f.commit(".nvmrc", "lts/iron\n", "pin node")
	f.commit("composer.json", `{"require":{"php":"~8.3.0"}}`, "add composer.json")
	d = f.wait(f.enqueue(p, nil).ID, store.DeploySuccess, store.DeployFailed)
	log := readLog(t, d)
	if d.Status != store.DeploySuccess || got(p) != "node=v20.19.0 npm=npm php=8.3.30 composer=yes\n" {
		t.Fatalf("status %s, runtimes.txt %q\n%s", d.Status, got(p), log)
	}
	for _, want := range []string{"Installing Node.js v20.19.0", "node: v20.19.0 (lts/iron from .nvmrc)",
		"Installing PHP 8.3.30", "php: 8.3.30 (~8.3.0 from composer.json require.php)", "Composer installed"} {
		if !strings.Contains(log, want) {
			t.Fatalf("log lacks %q:\n%s", want, log)
		}
	}
	rt, _ := f.store.ProjectRuntimes(ctx, p.ID)
	if rt["node"].Used != "v20.19.0" || rt["php"].Used != "8.3.30" {
		t.Fatalf("used = %+v", rt)
	}

	// The next deployment reuses both.
	d = f.wait(f.enqueue(p, nil).ID, store.DeploySuccess, store.DeployFailed)
	if log := readLog(t, d); strings.Contains(log, "Installing") || nodeMirror.Downloads.Load() != 1 || phpMirror.Downloads.Load() != 1 {
		t.Fatalf("installed again:\n%s", log)
	}

	// A package.json without a version takes the default version.
	run(t, f.src, "git", "rm", "-q", ".nvmrc")
	f.commit("package.json", `{"name":"web"}`, "add package.json")
	f.store.SetSetting(ctx, store.SettingRuntimeDefault("node"), "22")
	d = f.wait(f.enqueue(p, nil).ID, store.DeploySuccess, store.DeployFailed)
	if log := readLog(t, d); !strings.Contains(log, "node: v22.12.0 (22 from the default version)") {
		t.Fatalf("default version not used:\n%s", log)
	}

	// Off leaves a runtime out; so does turning the runtime off for the server.
	f.store.SetProjectRuntime(ctx, p.ID, "php", toolchain.Off)
	f.store.SetSetting(ctx, store.SettingRuntimeEnabled("node"), "false")
	d = f.wait(f.enqueue(p, nil).ID, store.DeploySuccess, store.DeployFailed)
	if log := readLog(t, d); strings.Contains(log, "node: ") || strings.Contains(log, "php: ") {
		t.Fatalf("runtimes set up while off:\n%s", log)
	}
	f.store.SetSetting(ctx, store.SettingRuntimeEnabled("node"), "")

	// A version that does not exist fails the deployment with a clear reason.
	f.store.SetProjectRuntime(ctx, p.ID, "node", "19")
	d = f.wait(f.enqueue(p, nil).ID, store.DeploySuccess, store.DeployFailed)
	if d.Status != store.DeployFailed || !strings.Contains(d.Error, "Node.js 19 (from project settings)") {
		t.Fatalf("status %s error %q", d.Status, d.Error)
	}
}
