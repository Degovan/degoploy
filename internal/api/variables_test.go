package api_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/degovan/degoploy/internal/api"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/store"
)

func TestVariables(t *testing.T) {
	srv, st := newTestServer(t)
	createUser(t, st, "admin", "admin-password-1", store.RoleAdmin)
	createUser(t, st, "dep", "deployer-password", store.RoleDeployer)
	admin := login(t, srv.URL, "admin", "admin-password-1")
	dep := login(t, srv.URL, "dep", "deployer-password")

	var p api.Project
	if code := admin.do("POST", "/api/v1/projects", projectJSON, &p); code != 201 {
		t.Fatalf("create project: %d", code)
	}
	base := fmt.Sprintf("/api/v1/projects/%d/variables", p.ID)

	var url, pw api.Variable
	if code := admin.do("POST", base, `{"name":"APP_URL","value":"https://shop.test","secret":false}`, &url); code != 201 {
		t.Fatalf("create plain: %d", code)
	}
	if code := admin.do("POST", base, `{"name":"DB_PASSWORD","value":"s3cr3t-pw","secret":true}`, &pw); code != 201 {
		t.Fatalf("create secret: %d", code)
	}
	if pw.Value != "" || !pw.Secret || pw.UpdatedBy != "admin" {
		t.Fatalf("secret response = %+v; the value must not come back", pw)
	}

	// Stored sealed, never as plaintext.
	stored, err := st.VariableByID(context.Background(), p.ID, pw.ID)
	if err != nil || strings.Contains(stored.Value, "s3cr3t-pw") || !strings.HasPrefix(stored.Value, "v1:") {
		t.Fatalf("stored secret = %q, %v", stored.Value, err)
	}

	var e httpx.ErrorBody
	for _, c := range []struct{ body, field string }{
		{`{"name":"APP_URL","value":"x","secret":false}`, "name"},
		{`{"name":"DEPLOY_ID","value":"x","secret":false}`, "name"},
		{`{"name":"1BAD","value":"x","secret":false}`, "name"},
		{`{"name":"EMPTY","value":"","secret":true}`, "value"},
		{`{"name":"MISSING","secret":false}`, "value"},
	} {
		if code := admin.do("POST", base, c.body, &e); code != 422 || e.Field != c.field {
			t.Fatalf("create %s: %d %+v, want 422 on %s", c.body, code, e, c.field)
		}
	}

	// Deployers see names and plain values are masked; they cannot change anything.
	var list []api.Variable
	if code := dep.do("GET", base, "", &list); code != 200 || len(list) != 2 {
		t.Fatalf("deployer list: %d %+v", code, list)
	}
	if list[0].Name != "APP_URL" || list[0].Value != "••••••" || list[1].Value != "" {
		t.Fatalf("deployer list = %+v", list)
	}
	if code := dep.do("POST", base, `{"name":"X","value":"y","secret":false}`, nil); code != 403 {
		t.Fatalf("deployer create: %d", code)
	}
	if code := dep.do("DELETE", fmt.Sprintf("%s/%d", base, url.ID), "", nil); code != 403 {
		t.Fatalf("deployer delete: %d", code)
	}

	// Renaming a secret without a value keeps it; turning it plain needs one.
	secretPath := fmt.Sprintf("%s/%d", base, pw.ID)
	if code := admin.do("PUT", secretPath, `{"name":"DATABASE_PASSWORD","secret":true}`, &pw); code != 200 || pw.Name != "DATABASE_PASSWORD" {
		t.Fatalf("rename secret: %d %+v", code, pw)
	}
	if after, _ := st.VariableByID(context.Background(), p.ID, pw.ID); after.Value != stored.Value {
		t.Fatal("renaming changed the sealed value")
	}
	if code := admin.do("PUT", secretPath, `{"name":"DATABASE_PASSWORD","secret":false}`, &e); code != 422 || e.Field != "value" {
		t.Fatalf("secret to plain without value: %d %+v", code, e)
	}

	// A plain variable becomes a secret in place.
	urlPath := fmt.Sprintf("%s/%d", base, url.ID)
	if code := admin.do("PUT", urlPath, `{"name":"APP_URL","secret":true}`, &url); code != 200 || !url.Secret || url.Value != "" {
		t.Fatalf("plain to secret: %d %+v", code, url)
	}
	if v, _ := st.VariableByID(context.Background(), p.ID, url.ID); !strings.HasPrefix(v.Value, "v1:") {
		t.Fatalf("converted secret stored as %q", v.Value)
	}

	if code := admin.do("DELETE", secretPath, "", nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code := admin.do("GET", base, "", &list); code != 200 || len(list) != 1 {
		t.Fatalf("after delete: %d %+v", code, list)
	}
}
