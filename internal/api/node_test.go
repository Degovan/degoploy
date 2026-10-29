package api_test

import (
	"fmt"
	"testing"

	"github.com/degovan/degoploy/internal/api"
	"github.com/degovan/degoploy/internal/config"
	"github.com/degovan/degoploy/internal/node/nodetest"
	"github.com/degovan/degoploy/internal/store"
)

func TestNodeVersions(t *testing.T) {
	mirror := nodetest.NewMirror(t,
		nodetest.Release{Version: "v22.12.0", LTS: "Jod", Date: "2026-04-01"},
		nodetest.Release{Version: "v22.11.0", LTS: "Jod", Date: "2026-01-01"},
		nodetest.Release{Version: "v20.19.0", LTS: "Iron", Date: "2026-03-01"},
	)
	srv, st := newTestServerWith(t, func(cfg *config.Config, _ string) {
		cfg.DataDir = t.TempDir()
		cfg.NodeMirror = mirror.URL
	})
	createUser(t, st, "admin", "admin-password-1", store.RoleAdmin)
	createUser(t, st, "dep", "deployer-password", store.RoleDeployer)
	admin := login(t, srv.URL, "admin", "admin-password-1")
	dep := login(t, srv.URL, "dep", "deployer-password")

	if code := dep.do("GET", "/api/v1/node", "", nil); code != 403 {
		t.Fatalf("deployer: %d", code)
	}

	var rel []api.NodeRelease
	if code := admin.do("GET", "/api/v1/node/releases", "", &rel); code != 200 || len(rel) != 2 || rel[0].Version != "v22.12.0" || rel[1].LTS != "Iron" {
		t.Fatalf("releases: %d %+v", code, rel)
	}

	var st1 api.NodeStatus
	if code := admin.do("POST", "/api/v1/node/install", `{"spec":"not a version"}`, nil); code != 422 {
		t.Fatalf("bad spec: %d", code)
	}
	if code := admin.do("POST", "/api/v1/node/install", `{"spec":"22.11.0"}`, &st1); code != 200 || len(st1.Installed) != 1 || st1.Installed[0].Version != "v22.11.0" {
		t.Fatalf("install: %d %+v", code, st1)
	}
	if code := admin.do("PUT", "/api/v1/node/default", `{"spec":"22"}`, &st1); code != 200 || st1.Default != "22" || !st1.Installed[0].Default {
		t.Fatalf("default: %d %+v", code, st1)
	}

	// A project pinning 22.11.0 keeps it through an update.
	var p api.Project
	if code := admin.do("POST", "/api/v1/projects", projectJSON, &p); code != 201 || p.NodeVersion != "auto" {
		t.Fatalf("create project: %d %+v", code, p)
	}
	base := fmt.Sprintf("/api/v1/projects/%d", p.ID)
	if code := admin.do("PATCH", base, `{"node_version":"twenty"}`, nil); code != 422 {
		t.Fatalf("bad node_version: %d", code)
	}
	if code := admin.do("PATCH", base, `{"node_version":"22.11.0"}`, &p); code != 200 || p.NodeVersion != "22.11.0" {
		t.Fatalf("pin: %d %+v", code, p)
	}
	var up api.NodeUpdateResult
	if code := admin.do("POST", "/api/v1/node/update", "", &up); code != 200 || len(up.Changes) != 1 || up.Changes[0].To != "v22.12.0" || up.Changes[0].Removed {
		t.Fatalf("update: %d %+v", code, up)
	}
	if len(up.Status.Installed) != 2 || len(up.Status.Installed[1].Pinned) != 1 {
		t.Fatalf("after update: %+v", up.Status)
	}

	if code := admin.do("POST", "/api/v1/node/installing/v22.11.0/cancel", "", nil); code != 404 {
		t.Fatalf("cancel with nothing running: %d", code)
	}
	if code := dep.do("POST", "/api/v1/node/installing/v22.11.0/cancel", "", nil); code != 403 {
		t.Fatalf("deployer cancel: %d", code)
	}
	if code := admin.do("DELETE", "/api/v1/node/versions/v22.11.0", "", &st1); code != 200 || len(st1.Installed) != 1 {
		t.Fatalf("remove: %d %+v", code, st1)
	}
	if code := admin.do("DELETE", "/api/v1/node/versions/v22.11.0", "", nil); code != 404 {
		t.Fatalf("remove again: %d", code)
	}

	var entries struct {
		Entries []api.AuditEntry `json:"entries"`
	}
	admin.do("GET", "/api/v1/audit?action=node.", "", &entries)
	if len(entries.Entries) != 4 {
		t.Fatalf("audit entries = %d", len(entries.Entries))
	}
}
