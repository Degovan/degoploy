package api_test

import (
	"fmt"
	"testing"

	"github.com/degovan/degoploy/internal/api"
	"github.com/degovan/degoploy/internal/config"
	"github.com/degovan/degoploy/internal/store"
	"github.com/degovan/degoploy/internal/toolchain/toolchaintest"
)

func TestRuntimes(t *testing.T) {
	nodeMirror := toolchaintest.NewNodeMirror(t,
		toolchaintest.Release{Version: "v22.12.0", LTS: "Jod", Date: "2026-04-01"},
		toolchaintest.Release{Version: "v22.11.0", LTS: "Jod", Date: "2026-01-01"},
		toolchaintest.Release{Version: "v20.19.0", LTS: "Iron", Date: "2026-03-01"},
	)
	phpMirror := toolchaintest.NewPHPMirror(t, "8.4.23", "8.3.30")
	srv, st := newTestServerWith(t, func(cfg *config.Config, _ string) {
		cfg.DataDir = t.TempDir()
		cfg.NodeMirror = nodeMirror.URL
		cfg.PHPMirror = phpMirror.URL
		cfg.ComposerURL = phpMirror.URL + "/composer.phar"
	})
	createUser(t, st, "admin", "admin-password-1", store.RoleAdmin)
	createUser(t, st, "dep", "deployer-password", store.RoleDeployer)
	admin := login(t, srv.URL, "admin", "admin-password-1")
	dep := login(t, srv.URL, "dep", "deployer-password")

	if code := dep.do("GET", "/api/v1/runtimes", "", nil); code != 403 {
		t.Fatalf("deployer: %d", code)
	}
	var all []api.RuntimeStatus
	if code := admin.do("GET", "/api/v1/runtimes", "", &all); code != 200 || len(all) != 2 || all[0].Name != "node" || all[1].Name != "php" || all[1].Verified || !all[1].Enabled {
		t.Fatalf("runtimes: %d %+v", code, all)
	}
	if code := admin.do("GET", "/api/v1/runtimes/ruby/releases", "", nil); code != 404 {
		t.Fatalf("unknown runtime: %d", code)
	}

	var rel []api.RuntimeRelease
	if code := admin.do("GET", "/api/v1/runtimes/node/releases", "", &rel); code != 200 || len(rel) != 2 || rel[0].Version != "v22.12.0" || rel[1].LTS != "Iron" {
		t.Fatalf("node releases: %d %+v", code, rel)
	}
	if code := admin.do("GET", "/api/v1/runtimes/php/releases", "", &rel); code != 200 || len(rel) != 2 || rel[0].Version != "8.4.23" {
		t.Fatalf("php releases: %d %+v", code, rel)
	}

	var s api.RuntimeStatus
	if code := admin.do("POST", "/api/v1/runtimes/node/install", `{"spec":"not a version"}`, nil); code != 422 {
		t.Fatalf("bad spec: %d", code)
	}
	if code := admin.do("POST", "/api/v1/runtimes/node/install", `{"spec":"22.11.0"}`, &s); code != 200 || len(s.Installed) != 1 || s.Installed[0].Version != "v22.11.0" {
		t.Fatalf("install node: %d %+v", code, s)
	}
	if code := admin.do("POST", "/api/v1/runtimes/php/install", `{"spec":"8.3"}`, &s); code != 200 || len(s.Installed) != 1 || s.Installed[0].Version != "8.3.30" {
		t.Fatalf("install php: %d %+v", code, s)
	}
	if code := admin.do("PUT", "/api/v1/runtimes/node/default", `{"spec":"22"}`, &s); code != 200 || s.Default != "22" || !s.Installed[0].Default {
		t.Fatalf("default: %d %+v", code, s)
	}
	if code := admin.do("PUT", "/api/v1/runtimes/php/enabled", `{"enabled":false}`, &s); code != 200 || s.Enabled {
		t.Fatalf("turn off php: %d %+v", code, s)
	}

	// Projects carry a setting per runtime; a project pinning 22.11.0 keeps it through an update.
	var p api.Project
	if code := admin.do("POST", "/api/v1/projects", projectJSON, &p); code != 201 || len(p.Runtimes) != 2 || p.Runtimes[0].Spec != "auto" || p.Runtimes[1].Name != "php" {
		t.Fatalf("create project: %d %+v", code, p)
	}
	base := fmt.Sprintf("/api/v1/projects/%d", p.ID)
	if code := admin.do("PATCH", base, `{"runtimes":{"node":"twenty"}}`, nil); code != 422 {
		t.Fatalf("bad node spec: %d", code)
	}
	if code := admin.do("PATCH", base, `{"runtimes":{"ruby":"3"}}`, nil); code != 422 {
		t.Fatalf("unknown runtime: %d", code)
	}
	if code := admin.do("PATCH", base, `{"runtimes":{"node":"22.11.0","php":"off"}}`, &p); code != 200 || p.Runtimes[0].Spec != "22.11.0" || p.Runtimes[1].Spec != "off" {
		t.Fatalf("pin: %d %+v", code, p)
	}
	var up api.RuntimeUpdateResult
	if code := admin.do("POST", "/api/v1/runtimes/node/update", "", &up); code != 200 || len(up.Changes) != 1 || up.Changes[0].To != "v22.12.0" || up.Changes[0].Removed {
		t.Fatalf("update: %d %+v", code, up)
	}
	if len(up.Status.Installed) != 2 || len(up.Status.Installed[1].Pinned) != 1 {
		t.Fatalf("after update: %+v", up.Status)
	}

	if code := admin.do("POST", "/api/v1/runtimes/node/installing/v22.11.0/cancel", "", nil); code != 404 {
		t.Fatalf("cancel with nothing running: %d", code)
	}
	if code := admin.do("DELETE", "/api/v1/runtimes/node/versions/v22.11.0", "", &s); code != 200 || len(s.Installed) != 1 {
		t.Fatalf("remove: %d %+v", code, s)
	}
	if code := admin.do("DELETE", "/api/v1/runtimes/node/versions/v22.11.0", "", nil); code != 404 {
		t.Fatalf("remove again: %d", code)
	}

	var entries struct {
		Entries []api.AuditEntry `json:"entries"`
	}
	admin.do("GET", "/api/v1/audit?action=runtime.", "", &entries)
	if len(entries.Entries) != 6 {
		t.Fatalf("audit entries = %d", len(entries.Entries))
	}
}
