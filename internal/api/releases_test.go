package api_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/degovan/degoploy/internal/api"
	"github.com/degovan/degoploy/internal/store"
)

func TestReleasesAPI(t *testing.T) {
	srv, st := newTestServer(t)
	createUser(t, st, "admin", "admin-password-1", store.RoleAdmin)
	createUser(t, st, "view", "viewer-password1", store.RoleViewer)
	admin := login(t, srv.URL, "admin", "admin-password-1")
	view := login(t, srv.URL, "view", "viewer-password1")

	var p api.Project
	if code := admin.do("POST", "/api/v1/projects", projectJSON, &p); code != 201 {
		t.Fatalf("create: %d", code)
	}
	base := fmt.Sprintf("/api/v1/projects/%d", p.ID)
	if code := admin.do("POST", base+"/releases/1/activate", "", nil); code != 404 {
		t.Fatalf("activate without releases: %d", code)
	}

	var bad struct{ Field string }
	if code := admin.do("PATCH", base, `{"releases":true,"shared_paths":["../etc"]}`, &bad); code != 422 || bad.Field != "shared_paths" {
		t.Fatalf("bad shared path: %d %+v", code, bad)
	}
	if code := admin.do("PATCH", base, `{"releases":true,"keep_releases":3,"shared_paths":[".env","storage/",".env"],"after_activate":"echo hi"}`, &p); code != 200 {
		t.Fatalf("turn releases on: %d", code)
	}
	if !p.Releases || p.KeepReleases != 3 || len(p.SharedPaths) != 2 || p.AfterActivate != "echo hi" || p.LiveRelease != nil {
		t.Fatalf("project: %+v", p)
	}

	// Two releases on disk, as two deployments left them; the older one is live.
	for _, id := range []int64{1, 2} {
		os.MkdirAll(filepath.Join(p.WorkDir, "releases", fmt.Sprint(id)), 0o755)
	}
	os.Symlink("releases/1", filepath.Join(p.WorkDir, "current"))

	var rels []api.Release
	if code := view.do("GET", base+"/releases", "", &rels); code != 200 || len(rels) != 2 || rels[0].ID != 2 || rels[0].Live || !rels[1].Live {
		t.Fatalf("releases: %d %+v", code, rels)
	}
	if code := view.do("POST", base+"/releases/2/activate", "", nil); code != 403 {
		t.Fatalf("viewer activate: %d", code)
	}
	if code := admin.do("POST", base+"/releases/9/activate", "", nil); code != 404 {
		t.Fatalf("missing release: %d", code)
	}
	var d api.Deployment
	if code := admin.do("POST", base+"/releases/2/activate", "", &d); code != 201 || d.Trigger != "rollback" || d.ReleaseID == nil || *d.ReleaseID != 2 {
		t.Fatalf("activate: %d %+v", code, d)
	}
}
