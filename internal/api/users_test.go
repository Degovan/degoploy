package api_test

import (
	"fmt"
	"testing"

	"github.com/degovan/degoploy/internal/api"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/store"
)

func TestUserManagement(t *testing.T) {
	srv, st := newTestServer(t)
	createUser(t, st, "root", "root-password-1", store.RoleAdmin)
	createUser(t, st, "dep", "deployer-password", store.RoleDeployer)
	admin := login(t, srv.URL, "root", "root-password-1")
	dep := login(t, srv.URL, "dep", "deployer-password")

	if code := dep.do("GET", "/api/v1/users", "", nil); code != 403 {
		t.Fatalf("deployer list users: %d", code)
	}
	if code := dep.do("GET", "/api/v1/audit", "", nil); code != 403 {
		t.Fatalf("deployer audit: %d", code)
	}

	var verr httpx.ErrorBody
	cases := []struct{ body, field string }{
		{`{"username":"x","email":"","display_name":"","role":"viewer","password":"long-enough-pw"}`, "username"},
		{`{"username":"citra","email":"nope","display_name":"","role":"viewer","password":"long-enough-pw"}`, "email"},
		{`{"username":"citra","email":"","display_name":"","role":"owner","password":"long-enough-pw"}`, "role"},
		{`{"username":"citra","email":"","display_name":"","role":"viewer","password":"short"}`, "password"},
		{`{"username":"DEP","email":"","display_name":"","role":"viewer","password":"long-enough-pw"}`, "username"},
	}
	for _, c := range cases {
		verr = httpx.ErrorBody{}
		if code := admin.do("POST", "/api/v1/users", c.body, &verr); code != 422 || verr.Field != c.field {
			t.Errorf("create %s: %d %+v", c.field, code, verr)
		}
	}

	var citra api.User
	if code := admin.do("POST", "/api/v1/users", `{"username":"citra","email":"citra@example.com","display_name":"Citra","role":"viewer","password":"citra-password-1"}`, &citra); code != 201 {
		t.Fatalf("create: %d", code)
	}
	citraClient := login(t, srv.URL, "citra", "citra-password-1")

	var updated api.User
	if code := admin.do("PATCH", fmt.Sprintf("/api/v1/users/%d", citra.ID), `{"role":"deployer"}`, &updated); code != 200 || updated.Role != "deployer" {
		t.Fatalf("promote: %d %+v", code, updated)
	}
	var me api.Me
	citraClient.do("GET", "/api/v1/me", "", &me)
	if len(me.Permissions) != 2 {
		t.Fatalf("role change should apply immediately, perms = %v", me.Permissions)
	}

	// Disabling signs the user out at once and blocks login.
	admin.do("PATCH", fmt.Sprintf("/api/v1/users/%d", citra.ID), `{"disabled":true}`, nil)
	if code := citraClient.do("GET", "/api/v1/me", "", nil); code != 401 {
		t.Fatalf("disabled user's session: %d", code)
	}
	if code := (&client{t: t, base: srv.URL, http: citraClient.http}).do("POST", "/api/v1/auth/login",
		`{"username":"citra","password":"citra-password-1"}`, nil); code != 401 {
		t.Fatalf("disabled login: %d", code)
	}
	admin.do("PATCH", fmt.Sprintf("/api/v1/users/%d", citra.ID), `{"disabled":false}`, nil)

	// Admin password reset revokes the target's sessions; the new password works.
	citraClient = login(t, srv.URL, "citra", "citra-password-1")
	if code := admin.do("PUT", fmt.Sprintf("/api/v1/users/%d/password", citra.ID), `{"password":"brand-new-password"}`, nil); code != 204 {
		t.Fatalf("reset: %d", code)
	}
	if code := citraClient.do("GET", "/api/v1/me", "", nil); code != 401 {
		t.Fatalf("session after reset: %d", code)
	}
	citraClient = login(t, srv.URL, "citra", "brand-new-password")

	// Self-service password change keeps the current session, revokes others.
	other := login(t, srv.URL, "citra", "brand-new-password")
	if code := citraClient.do("POST", "/api/v1/me/password", `{"current_password":"wrong","new_password":"another-password-1"}`, &verr); code != 422 || verr.Field != "current_password" {
		t.Fatalf("wrong current: %d %+v", code, verr)
	}
	if code := citraClient.do("POST", "/api/v1/me/password", `{"current_password":"brand-new-password","new_password":"another-password-1"}`, nil); code != 204 {
		t.Fatalf("change own: %d", code)
	}
	if code := citraClient.do("GET", "/api/v1/me", "", nil); code != 200 {
		t.Fatalf("current session after change: %d", code)
	}
	if code := other.do("GET", "/api/v1/me", "", nil); code != 401 {
		t.Fatalf("other session after change: %d", code)
	}

	// Admins cannot lock themselves out.
	var root api.User
	admin.do("GET", "/api/v1/me", "", &me)
	root = me.User
	for _, body := range []string{`{"role":"viewer"}`, `{"disabled":true}`} {
		if code := admin.do("PATCH", fmt.Sprintf("/api/v1/users/%d", root.ID), body, nil); code != 409 {
			t.Errorf("self %s: %d", body, code)
		}
	}
	if code := admin.do("DELETE", fmt.Sprintf("/api/v1/users/%d", root.ID), "", nil); code != 409 {
		t.Fatalf("self delete: %d", code)
	}
	if code := admin.do("DELETE", fmt.Sprintf("/api/v1/users/%d", citra.ID), "", nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}

	// Audit: filter by prefix and paginate.
	var page api.AuditPage
	admin.do("GET", "/api/v1/audit?action=user.&limit=3", "", &page)
	if len(page.Entries) != 3 || page.NextBefore == nil || page.Entries[0].Action != "user.delete" {
		t.Fatalf("audit page 1 = %+v", page)
	}
	if page.Entries[0].Username != "root" || page.Entries[0].Detail["username"] != "citra" {
		t.Fatalf("audit entry = %+v", page.Entries[0])
	}
	var rest api.AuditPage
	admin.do("GET", fmt.Sprintf("/api/v1/audit?action=user.&limit=50&before=%d", *page.NextBefore), "", &rest)
	total := len(page.Entries) + len(rest.Entries)
	if total != 6 || rest.NextBefore != nil { // create, 3 updates (promote, disable, enable), reset, delete
		t.Fatalf("user.* entries = %d (next %v)", total, rest.NextBefore)
	}
	var mine api.AuditPage
	admin.do("GET", fmt.Sprintf("/api/v1/audit?user=%d&action=auth.password_change", citra.ID), "", &mine)
	if len(mine.Entries) != 0 {
		// citra was deleted, so their entries now have no user; filter by user id finds none.
		t.Fatalf("entries for deleted user = %d", len(mine.Entries))
	}
}

func TestResetPasswordRequireChange(t *testing.T) {
	srv, st := newTestServer(t)
	createUser(t, st, "root", "root-password-1", store.RoleAdmin)
	createUser(t, st, "dep", "deployer-password", store.RoleDeployer)
	admin := login(t, srv.URL, "root", "root-password-1")
	dep := login(t, srv.URL, "dep", "deployer-password")
	login(t, srv.URL, "dep", "deployer-password")

	var sessions api.UserSessions
	if code := admin.do("GET", "/api/v1/users/2/sessions", "", &sessions); code != 200 || sessions.Active != 2 {
		t.Fatalf("dep sessions: %d %+v", code, sessions)
	}
	// Your own count leaves out the session you are using.
	if code := admin.do("GET", "/api/v1/users/1/sessions", "", &sessions); code != 200 || sessions.Active != 0 {
		t.Fatalf("own sessions: %d %+v", code, sessions)
	}

	if code := admin.do("PUT", "/api/v1/users/2/password", `{"password":"temporary-pass-1","require_change":true}`, nil); code != 204 {
		t.Fatalf("reset: %d", code)
	}
	if code := dep.do("GET", "/api/v1/me", "", nil); code != 401 {
		t.Fatalf("old session after reset: %d", code)
	}

	// Until they pick their own password, only /me, the password change and logout work.
	dep = login(t, srv.URL, "dep", "temporary-pass-1")
	var me api.Me
	if code := dep.do("GET", "/api/v1/me", "", &me); code != 200 || !me.User.MustChangePassword {
		t.Fatalf("me after reset: %d %+v", code, me.User)
	}
	var e httpx.ErrorBody
	if code := dep.do("GET", "/api/v1/projects", "", &e); code != 403 || e.Code != "password_change_required" {
		t.Fatalf("projects before change: %d %+v", code, e)
	}
	if code := dep.do("POST", "/api/v1/me/password", `{"current_password":"temporary-pass-1","new_password":"my-own-password"}`, nil); code != 204 {
		t.Fatalf("change: %d", code)
	}
	if code := dep.do("GET", "/api/v1/projects", "", nil); code != 200 {
		t.Fatalf("projects after change: %d", code)
	}

	// Resetting yourself never locks you out.
	if code := admin.do("PUT", "/api/v1/users/1/password", `{"password":"root-password-2","require_change":true}`, nil); code != 204 {
		t.Fatalf("self reset: %d", code)
	}
	if code := admin.do("GET", "/api/v1/me", "", &me); code != 200 || me.User.MustChangePassword {
		t.Fatalf("self reset flag: %d %+v", code, me.User)
	}
}
