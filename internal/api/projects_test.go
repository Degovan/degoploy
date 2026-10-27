package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"github.com/degovan/degoploy/internal/api"
	"github.com/degovan/degoploy/internal/auth"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/store"
)

type client struct {
	t    *testing.T
	base string
	http *http.Client
	csrf string
}

func login(t *testing.T, base, user, pw string) *client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &client{t: t, base: base, http: &http.Client{Jar: jar}}
	var me api.Me
	if code := c.do("POST", "/api/v1/auth/login", fmt.Sprintf(`{"username":%q,"password":%q}`, user, pw), &me); code != 200 {
		t.Fatalf("login %s: %d", user, code)
	}
	c.csrf = me.CSRFToken
	return c
}

func (c *client) do(method, path, body string, out any) int {
	c.t.Helper()
	resp := do(c.t, c.http, method, c.base+path, body, map[string]string{auth.CSRFHeader: c.csrf})
	if out != nil {
		json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

const projectJSON = `{"name":"Shop","slug":"shop","provider":"gitea","repo_url":"git@gitea.example.com:acme/shop.git",
	"work_dir":"/srv/shop","script_source":"managed","script_path":"","auto_pull":true,"timeout_sec":300,"enabled":true}`

func TestProjectPermissions(t *testing.T) {
	srv, st := newTestServer(t)
	createUser(t, st, "admin", "admin-password-1", store.RoleAdmin)
	createUser(t, st, "dep", "deployer-password", store.RoleDeployer)
	createUser(t, st, "view", "viewer-password1", store.RoleViewer)
	admin := login(t, srv.URL, "admin", "admin-password-1")
	dep := login(t, srv.URL, "dep", "deployer-password")
	view := login(t, srv.URL, "view", "viewer-password1")

	if code := dep.do("POST", "/api/v1/projects", projectJSON, nil); code != 403 {
		t.Fatalf("deployer create: %d", code)
	}
	var p api.Project
	if code := admin.do("POST", "/api/v1/projects", projectJSON, &p); code != 201 {
		t.Fatalf("admin create: %d", code)
	}
	base := fmt.Sprintf("/api/v1/projects/%d", p.ID)

	var list []api.Project
	if code := view.do("GET", "/api/v1/projects", "", &list); code != 200 || len(list) != 1 {
		t.Fatalf("viewer list: %d %v", code, list)
	}

	checks := []struct {
		c      *client
		who    string
		method string
		path   string
		body   string
		want   int
	}{
		{view, "viewer", "GET", base, "", 200},
		{view, "viewer", "GET", base + "/script", "", 403},
		{dep, "deployer", "GET", base + "/script", "", 200},
		{dep, "deployer", "PUT", base + "/script", `{"content":"echo hi"}`, 403},
		{dep, "deployer", "GET", base + "/secrets", "", 403},
		{dep, "deployer", "PATCH", base, `{"name":"x"}`, 403},
		{dep, "deployer", "DELETE", base, "", 403},
		{admin, "admin", "GET", base + "/secrets", "", 200},
		{admin, "admin", "PUT", base + "/script", `{"content":"echo hi"}`, 200},
		{admin, "admin", "GET", "/api/v1/projects/999", "", 404},
	}
	for _, c := range checks {
		if got := c.c.do(c.method, c.path, c.body, nil); got != c.want {
			t.Errorf("%s %s %s = %d, want %d", c.who, c.method, c.path, got, c.want)
		}
	}

	var secrets api.ProjectSecrets
	admin.do("GET", base+"/secrets", "", &secrets)
	if !strings.HasPrefix(secrets.WebhookPath, "/hook/") || len(secrets.WebhookSecret) != 64 || !strings.HasPrefix(secrets.PublicKey, "ssh-ed25519 ") {
		t.Fatalf("secrets = %+v", secrets)
	}
	var rotated api.ProjectSecrets
	admin.do("POST", base+"/regenerate-secret", "", &rotated)
	if rotated.WebhookSecret == secrets.WebhookSecret || rotated.PublicKey != secrets.PublicKey {
		t.Fatal("regenerate-secret should change only the secret")
	}

	var revs []api.ScriptRevision
	admin.do("GET", base+"/script/revisions", "", &revs)
	if len(revs) != 2 || revs[0].Content != "echo hi\n" || revs[0].CreatedBy != "admin" {
		t.Fatalf("revisions = %+v", revs)
	}

	var verr httpx.ErrorBody
	if code := admin.do("PATCH", base, `{"repo_url":"--upload-pack=x"}`, &verr); code != 422 || verr.Field != "repo_url" {
		t.Fatalf("invalid patch: %d %+v", code, verr)
	}
	var updated api.Project
	if code := admin.do("PATCH", base, `{"name":"Shop v2","enabled":false}`, &updated); code != 200 || updated.Name != "Shop v2" || updated.Enabled || updated.RepoURL == "" {
		t.Fatalf("patch: %d %+v", code, updated)
	}

	if code := admin.do("DELETE", base, "", nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code := admin.do("GET", base, "", nil); code != 404 {
		t.Fatalf("get after delete: %d", code)
	}
}
