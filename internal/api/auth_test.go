package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/degovan/degoploy/internal/api"
	"github.com/degovan/degoploy/internal/audit"
	"github.com/degovan/degoploy/internal/auth"
	"github.com/degovan/degoploy/internal/config"
	"github.com/degovan/degoploy/internal/deploy"
	"github.com/degovan/degoploy/internal/project"
	"github.com/degovan/degoploy/internal/realtime"
	"github.com/degovan/degoploy/internal/secrets"
	"github.com/degovan/degoploy/internal/store"
)

func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	return newTestServerWith(t, nil)
}

// newTestServerWith lets a test adjust the config once the server URL is known.
func newTestServerWith(t *testing.T, configure func(cfg *config.Config, baseURL string)) (*httptest.Server, *store.Store) {
	t.Helper()
	ctx := context.Background()
	dataDir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dataDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	var handler http.Handler
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
	t.Cleanup(srv.Close)

	cfg := config.Default()
	if configure != nil {
		configure(&cfg, srv.URL)
	}
	var sso *auth.OIDCProvider
	if cfg.OIDC.Enabled {
		sso = auth.NewOIDCProvider(cfg.OIDC)
	}
	hub := realtime.NewHub()
	projects := project.NewManager(st, dataDir)
	box, err := secrets.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	deploys := deploy.NewService(st, projects, hub, box, 2)
	if err := deploys.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deploys.Shutdown(5 * time.Second) })
	sessions := auth.NewSessions(st, cfg.SessionTTL, false)
	a := api.New(api.Deps{
		Config: &cfg, Store: st, Sessions: sessions,
		Authz: &auth.Authorizer{}, Audit: audit.New(st), Version: "test",
		Projects: projects, Deploys: deploys, Hub: hub, OIDC: sso, Secrets: box,
	})
	mux := http.NewServeMux()
	a.Register(mux)
	handler = sessions.Load(mux)
	return srv, st
}

func createUser(t *testing.T, st *store.Store, name, pw string, role store.Role) {
	t.Helper()
	hash, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	err = st.CreateUser(context.Background(), &store.User{
		Username: name, AuthProvider: store.ProviderLocal, PasswordHash: hash, Role: role,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func do(t *testing.T, c *http.Client, method, url, body string, hdr map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestLoginSessionAndCSRF(t *testing.T) {
	srv, st := newTestServer(t)
	createUser(t, st, "alice", "correct-horse-battery", store.RoleDeployer)

	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}

	if r := do(t, c, "GET", srv.URL+"/api/v1/me", "", nil); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous /me: got %d", r.StatusCode)
	}

	if r := do(t, c, "POST", srv.URL+"/api/v1/auth/login", `{"username":"alice","password":"wrong-password"}`, nil); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad password: got %d", r.StatusCode)
	}

	r := do(t, c, "POST", srv.URL+"/api/v1/auth/login", `{"username":"ALICE","password":"correct-horse-battery"}`, nil)
	if r.StatusCode != http.StatusOK {
		t.Fatalf("login: got %d", r.StatusCode)
	}
	var me api.Me
	if err := json.NewDecoder(r.Body).Decode(&me); err != nil {
		t.Fatal(err)
	}
	if me.User.Username != "alice" || me.CSRFToken == "" {
		t.Fatalf("unexpected login response: %+v", me)
	}
	want := []string{"project.view", "project.deploy"}
	if strings.Join(me.Permissions, ",") != strings.Join(want, ",") {
		t.Fatalf("permissions = %v, want %v", me.Permissions, want)
	}

	if r := do(t, c, "GET", srv.URL+"/api/v1/me", "", nil); r.StatusCode != http.StatusOK {
		t.Fatalf("/me after login: got %d", r.StatusCode)
	}

	if r := do(t, c, "POST", srv.URL+"/api/v1/auth/logout", "", nil); r.StatusCode != http.StatusForbidden {
		t.Fatalf("logout without CSRF: got %d", r.StatusCode)
	}
	if r := do(t, c, "POST", srv.URL+"/api/v1/auth/logout", "", map[string]string{auth.CSRFHeader: me.CSRFToken}); r.StatusCode != http.StatusNoContent {
		t.Fatalf("logout: got %d", r.StatusCode)
	}
	if r := do(t, c, "GET", srv.URL+"/api/v1/me", "", nil); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("/me after logout: got %d", r.StatusCode)
	}

	entries, err := st.ListAudit(context.Background(), store.AuditFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, e := range entries {
		actions = append(actions, e.Action)
	}
	if got := strings.Join(actions, ","); got != "auth.logout,auth.login,auth.login_failed" {
		t.Fatalf("audit actions = %s", got)
	}
}

func TestLoginRequiresJSONContentType(t *testing.T) {
	srv, st := newTestServer(t)
	createUser(t, st, "bob", "correct-horse-battery", store.RoleViewer)

	r := do(t, http.DefaultClient, "POST", srv.URL+"/api/v1/auth/login",
		`{"username":"bob","password":"correct-horse-battery"}`,
		map[string]string{"Content-Type": "text/plain"})
	if r.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain login: got %d", r.StatusCode)
	}
}
