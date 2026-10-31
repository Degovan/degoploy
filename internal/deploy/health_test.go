package deploy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/degovan/degoploy/internal/release"
	"github.com/degovan/degoploy/internal/store"
)

// healthServer answers with the status in the returned pointer.
func healthServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var status atomic.Int32
	status.Store(http.StatusOK)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(int(status.Load()))
	}))
	t.Cleanup(srv.Close)
	return srv, &status
}

func fastHealthChecks(t *testing.T) {
	old := healthInterval
	healthInterval = 20 * time.Millisecond
	t.Cleanup(func() { healthInterval = old })
}

func (f *fixture) withHealth(p *store.Project, url string) {
	f.t.Helper()
	p.HealthURL, p.HealthTimeout = url, 1
	if err := f.store.UpdateProject(context.Background(), p); err != nil {
		f.t.Fatal(err)
	}
}

func TestHealthCheckRollsBack(t *testing.T) {
	fastHealthChecks(t)
	srv, status := healthServer(t)
	f := newFixture(t)
	p := f.releaseProject("shop", "true", 5, nil, `echo "after-activate in release $DEPLOY_RELEASE"`)
	f.withHealth(p, srv.URL+"/up")
	base := p.WorkDir

	d1 := f.deploy(p, store.DeploySuccess)
	if log := readLog(t, d1); !strings.Contains(log, "Checking health: GET "+srv.URL+"/up") || !strings.Contains(log, "OK after") {
		t.Fatalf("log:\n%s", log)
	}

	// The new release answers 503: current goes back to d1, whose
	// after-activate commands run again, and the new release is removed.
	status.Store(http.StatusServiceUnavailable)
	f.commit("app.txt", "v2\n", "v2")
	d2 := f.deploy(p, store.DeployFailed)
	log := readLog(t, d2)
	if release.Live(base) != d1.ID || release.Exists(base, d2.ID) {
		t.Fatalf("live %d, release %d exists %v\n%s", release.Live(base), d2.ID, release.Exists(base, d2.ID), log)
	}
	if !strings.Contains(d2.Error, "503 Service Unavailable") || !strings.Contains(d2.Error, "rolled back to release "+itoa(d1.ID)) {
		t.Fatalf("error = %q", d2.Error)
	}
	if strings.Count(log, "503 Service Unavailable (after") != 1 {
		t.Fatalf("repeated results should be logged once:\n%s", log)
	}
	if !strings.Contains(log, "after-activate in release "+itoa(d2.ID)) || !strings.Contains(log, "after-activate in release "+itoa(d1.ID)) {
		t.Fatalf("after-activate should run for both releases:\n%s", log)
	}

	// A redirect counts as an answer.
	status.Store(http.StatusFound)
	d3 := f.deploy(p, store.DeploySuccess)
	if release.Live(base) != d3.ID {
		t.Fatalf("live %d, want %d", release.Live(base), d3.ID)
	}
}

// The first release has nothing to go back to: it stays live and the deployment fails.
func TestHealthCheckFirstRelease(t *testing.T) {
	fastHealthChecks(t)
	srv, status := healthServer(t)
	status.Store(http.StatusInternalServerError)
	f := newFixture(t)
	p := f.releaseProject("new", "true", 5, nil, "")
	f.withHealth(p, srv.URL)
	d := f.deploy(p, store.DeployFailed)
	if release.Live(p.WorkDir) != d.ID || !strings.Contains(readLog(t, d), "no earlier release") {
		t.Fatalf("live %d\n%s", release.Live(p.WorkDir), readLog(t, d))
	}
}

// In place, a failed check only fails the deployment.
func TestHealthCheckInPlace(t *testing.T) {
	fastHealthChecks(t)
	f := newFixture(t)
	p := f.project("inplace", "true", 0)
	f.withHealth(p, "http://127.0.0.1:1/up")
	d := f.deploy(p, store.DeployFailed)
	if !strings.Contains(d.Error, "health check failed") || !strings.Contains(d.Error, "no answer") {
		t.Fatalf("error = %q\n%s", d.Error, readLog(t, d))
	}
}
