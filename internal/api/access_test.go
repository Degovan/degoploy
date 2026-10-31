package api_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/degovan/degoploy/internal/api"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/store"
)

func TestSelectedProjects(t *testing.T) {
	srv, st := newTestServer(t)
	createUser(t, st, "root", "root-password-1", store.RoleAdmin)
	createUser(t, st, "budi", "budi-password-1", store.RoleDeployer)
	createUser(t, st, "view", "viewer-password1", store.RoleViewer)
	admin := login(t, srv.URL, "root", "root-password-1")
	budi := login(t, srv.URL, "budi", "budi-password-1")
	view := login(t, srv.URL, "view", "viewer-password1")

	var shop, blog api.Project
	admin.do("POST", "/api/v1/projects", projectJSON, &shop)
	admin.do("POST", "/api/v1/projects", strings.NewReplacer(`"Shop"`, `"Blog"`, `"shop"`, `"blog"`).Replace(projectJSON), &blog)
	if shop.ID == 0 || blog.ID == 0 {
		t.Fatal("projects not created")
	}
	var users []api.User
	admin.do("GET", "/api/v1/users", "", &users)
	budiID := users[slices.IndexFunc(users, func(u api.User) bool { return u.Username == "budi" })].ID
	userPath := fmt.Sprintf("/api/v1/users/%d", budiID)

	var verr httpx.ErrorBody
	invalid := []struct{ body, field string }{
		{fmt.Sprintf(`{"selected_projects":true,"projects":[{"project_id":%d,"role":"admin"}]}`, shop.ID), "projects"},
		{`{"selected_projects":true,"projects":[{"project_id":999,"role":"viewer"}]}`, "projects"},
		{fmt.Sprintf(`{"selected_projects":true,"projects":[{"project_id":%d,"role":"viewer"},{"project_id":%[1]d,"role":"deployer"}]}`, shop.ID), "projects"},
		{`{"role":"admin","selected_projects":true}`, "selected_projects"},
	}
	for _, c := range invalid {
		verr = httpx.ErrorBody{}
		if code := admin.do("PATCH", userPath, c.body, &verr); code != 422 || verr.Field != c.field {
			t.Errorf("%s: %d %+v", c.body, code, verr)
		}
	}

	var budiUser api.User
	body := fmt.Sprintf(`{"selected_projects":true,"projects":[{"project_id":%d,"role":"viewer"}]}`, shop.ID)
	if code := admin.do("PATCH", userPath, body, &budiUser); code != 200 || !budiUser.SelectedProjects || len(budiUser.Projects) != 1 {
		t.Fatalf("select shop: %d %+v", code, budiUser)
	}

	var list []api.Project
	budi.do("GET", "/api/v1/projects", "", &list)
	if len(list) != 1 || list[0].ID != shop.ID || !slices.Equal(list[0].Permissions, []string{"project.view"}) {
		t.Fatalf("budi's projects = %+v", list)
	}
	checks := []struct {
		c      *client
		method string
		path   string
		want   int
	}{
		{budi, "GET", fmt.Sprintf("/api/v1/projects/%d", blog.ID), 404},
		{budi, "GET", fmt.Sprintf("/api/v1/projects/%d/deployments", blog.ID), 404},
		{budi, "POST", fmt.Sprintf("/api/v1/projects/%d/deployments", shop.ID), 403},
		{budi, "GET", fmt.Sprintf("/api/v1/projects/%d/script", shop.ID), 403},
		{view, "GET", fmt.Sprintf("/api/v1/projects/%d", blog.ID), 200},
	}
	for _, c := range checks {
		if got := c.c.do(c.method, c.path, `{"ref":"main"}`, nil); got != c.want {
			t.Errorf("%s %s = %d, want %d", c.method, c.path, got, c.want)
		}
	}

	// The project role applies, not the global one, and takes effect at once.
	body = fmt.Sprintf(`{"projects":[{"project_id":%d,"role":"deployer"}]}`, shop.ID)
	admin.do("PATCH", userPath, body, nil)
	var p api.Project
	budi.do("GET", fmt.Sprintf("/api/v1/projects/%d", shop.ID), "", &p)
	if !slices.Contains(p.Permissions, "project.deploy") {
		t.Fatalf("deployer on shop, permissions = %v", p.Permissions)
	}

	var entries api.AuditPage
	admin.do("GET", "/api/v1/audit?action=user.update", "", &entries)
	if len(entries.Entries) == 0 || !strings.Contains(fmt.Sprint(entries.Entries[0].Detail["changes"]), "shop (deployer)") {
		t.Fatalf("audit = %+v", entries.Entries)
	}

	// A promotion to admin drops the selection; a demotion does not bring it back.
	admin.do("PATCH", userPath, `{"role":"admin"}`, nil)
	admin.do("PATCH", userPath, `{"role":"viewer"}`, &budiUser)
	if budiUser.SelectedProjects || len(budiUser.Projects) != 0 {
		t.Fatalf("after promotion = %+v", budiUser)
	}
	budi.do("GET", "/api/v1/projects", "", &list)
	if len(list) != 2 {
		t.Fatalf("every project again, got %d", len(list))
	}
}
