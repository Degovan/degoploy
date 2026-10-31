package deploy

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/degovan/degoploy/internal/release"
	"github.com/degovan/degoploy/internal/store"
)

// releaseProject is f.project with releases on.
func (f *fixture) releaseProject(slug, script string, keep int, shared []string, after string) *store.Project {
	f.t.Helper()
	p := f.project(slug, script, 0)
	p.Releases, p.KeepReleases, p.SharedPaths, p.AfterActivate = true, keep, shared, after
	if err := f.store.UpdateProject(context.Background(), p); err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f *fixture) deploy(p *store.Project, want string) *store.Deployment {
	f.t.Helper()
	d := f.wait(f.enqueue(p, nil).ID, store.DeploySuccess, store.DeployFailed)
	if d.Status != want {
		f.t.Fatalf("deployment %d: %s, want %s\n%s", d.ID, d.Status, want, readLog(f.t, d))
	}
	return d
}

func TestReleases(t *testing.T) {
	f := newFixture(t)
	f.commit("app.txt", "v1\n", "v1")
	os.MkdirAll(filepath.Join(f.src, "storage"), 0o755)
	f.commit("storage/.keep", "", "storage")
	p := f.releaseProject("site", `[ -f fail ] && exit 3
echo "dir=$PWD workdir=$DEPLOY_WORKDIR release=$DEPLOY_RELEASE" > built.txt
echo "$DEPLOY_RELEASE" >> storage/deploys`, 2, []string{".env", "storage/"}, `echo "after: $(cat app.txt) in $PWD"`)
	base := p.WorkDir

	d1 := f.deploy(p, store.DeploySuccess)
	dir1 := release.Dir(base, d1.ID)
	if release.Live(base) != d1.ID || d1.ReleaseID == nil || *d1.ReleaseID != d1.ID {
		t.Fatalf("live = %d, release_id = %v, want %d", release.Live(base), d1.ReleaseID, d1.ID)
	}
	built, _ := os.ReadFile(filepath.Join(base, "current", "built.txt"))
	if want := "dir=" + dir1 + " workdir=" + dir1 + " release=" + itoa(d1.ID) + "\n"; string(built) != want {
		t.Fatalf("built.txt = %q, want %q", built, want)
	}
	log := readLog(t, d1)
	if !strings.Contains(log, "after: v1 in "+release.Current(base)) || !strings.Contains(log, "shared/.env: created empty") ||
		!strings.Contains(log, "shared/storage/: moved here") {
		t.Fatalf("log:\n%s", log)
	}
	if target, _ := os.Readlink(filepath.Join(dir1, ".env")); target != filepath.Join(base, "shared", ".env") {
		t.Fatalf(".env links to %q", target)
	}

	// The second release shares storage with the first.
	f.commit("app.txt", "v2\n", "v2")
	d2 := f.deploy(p, store.DeploySuccess)
	deploys, _ := os.ReadFile(filepath.Join(base, "shared", "storage", "deploys"))
	if string(deploys) != itoa(d1.ID)+"\n"+itoa(d2.ID)+"\n" || release.Live(base) != d2.ID {
		t.Fatalf("storage/deploys = %q, live %d", deploys, release.Live(base))
	}

	// A failed build is removed and the live release stays.
	f.commit("fail", "", "break it")
	d3 := f.deploy(p, store.DeployFailed)
	if release.Live(base) != d2.ID || release.Exists(base, d3.ID) {
		t.Fatalf("after a failed build: live %d, release %d exists %v\n%s", release.Live(base), d3.ID, release.Exists(base, d3.ID), readLog(t, d3))
	}

	// Rolling back switches current and runs the after-activate commands.
	rb, err := f.svc.Enqueue(context.Background(), Request{Project: p, Trigger: store.TriggerRollback, ReleaseID: d1.ID})
	if err != nil {
		t.Fatal(err)
	}
	rb = f.wait(rb.ID, store.DeploySuccess, store.DeployFailed)
	log = readLog(t, rb)
	if rb.Status != store.DeploySuccess || release.Live(base) != d1.ID || !strings.Contains(log, "after: v1") {
		t.Fatalf("rollback: %s, live %d\n%s", rb.Status, release.Live(base), log)
	}
	if rb.AutoPull == nil || *rb.AutoPull || rb.ScriptStartedAt == nil {
		t.Fatalf("rollback stages: auto_pull %v, script started %v", rb.AutoPull, rb.ScriptStartedAt)
	}

	// Keeping 2: the next release prunes all but itself and one more, never
	// the live one it replaced.
	os.Remove(filepath.Join(f.src, "fail"))
	run(t, f.src, "git", "rm", "-q", "fail")
	f.commit("app.txt", "v3\n", "v3")
	d4 := f.deploy(p, store.DeploySuccess)
	if got := release.List(base); !slices.Equal(got, []int64{d4.ID, d2.ID}) {
		t.Fatalf("releases = %v, want [%d %d]", got, d4.ID, d2.ID)
	}
	wt := run(t, release.Repo(base), "git", "worktree", "list")
	if strings.Contains(wt, filepath.Join("releases", itoa(d1.ID))) {
		t.Fatalf("worktree of a removed release is still listed:\n%s", wt)
	}

	// Rolling back to a release that was removed fails and changes nothing.
	gone, _ := f.svc.Enqueue(context.Background(), Request{Project: p, Trigger: store.TriggerRollback, ReleaseID: d1.ID})
	gone = f.wait(gone.ID, store.DeploySuccess, store.DeployFailed)
	if gone.Status != store.DeployFailed || !strings.Contains(gone.Error, "no longer on the server") || release.Live(base) != d4.ID {
		t.Fatalf("rollback to a removed release: %s %q", gone.Status, gone.Error)
	}
}

// A failing after-activate command fails the deployment but the release stays live.
func TestReleaseAfterActivateFails(t *testing.T) {
	f := newFixture(t)
	p := f.releaseProject("hook", "true", 5, nil, "echo reloading; exit 4")
	d := f.deploy(p, store.DeployFailed)
	if release.Live(p.WorkDir) != d.ID || d.ExitCode == nil || *d.ExitCode != 4 || d.Error != "reloading" {
		t.Fatalf("live %d, exit %v, error %q\n%s", release.Live(p.WorkDir), d.ExitCode, d.Error, readLog(t, d))
	}
}

func TestReleasesRefuseInPlaceCheckout(t *testing.T) {
	f := newFixture(t)
	p := f.project("moved", "true", 0)
	f.deploy(p, store.DeploySuccess)
	p.Releases = true
	if err := f.store.UpdateProject(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	d := f.deploy(p, store.DeployFailed)
	if !strings.Contains(d.Error, "deployed in place") {
		t.Fatalf("error = %q", d.Error)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
