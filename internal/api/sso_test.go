package api_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/degovan/degoploy/internal/api"
	"github.com/degovan/degoploy/internal/auth/oidctest"
	"github.com/degovan/degoploy/internal/config"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/store"
)

func TestSSOSettings(t *testing.T) {
	idp := oidctest.New("degoploy", "s3cret")
	t.Cleanup(idp.Close)
	path := filepath.Join(t.TempDir(), "degoploy.yaml")
	os.WriteFile(path, []byte("# my settings\nlocal_login: true # break-glass\n"), 0o600)
	var base string
	srv, st := newTestServerWith(t, func(cfg *config.Config, b string) {
		base = b
		cfg.Path = path
		// As if DEGOPLOY_OIDC_CLIENT_ID were set.
		cfg.OIDC.ClientID = "degoploy"
		cfg.Locked = map[string]string{"oidc.client_id": "DEGOPLOY_OIDC_CLIENT_ID"}
	})
	createUser(t, st, "root", "root-password-1", store.RoleAdmin)
	admin := login(t, srv.URL, "root", "root-password-1")

	var s api.SSOStatus
	if code := admin.do("GET", "/api/v1/auth/sso", "", &s); code != 200 || s.ConfigPath != path || s.LocalUsers != 1 {
		t.Fatalf("status: %d %+v", code, s)
	}

	var e httpx.ErrorBody
	if code := admin.do("PATCH", "/api/v1/auth/sso", `{"client_id":"other"}`, &e); code != 422 || e.Field != "oidc.client_id" {
		t.Fatalf("locked field: %d %+v", code, e)
	}
	if code := admin.do("PATCH", "/api/v1/auth/sso", `{"enabled":true,"issuer":"`+idp.URL+`"}`, &e); code != 422 {
		t.Fatalf("enable without redirect or mapping: %d %+v", code, e)
	}

	body := `{"enabled":true,"provider_name":"Degovan SSO","issuer":"` + idp.URL + `","client_secret":"s3cret",
		"redirect_url":"` + base + `/auth/oidc/callback","scopes":["profile","email","profile"],
		"role_mapping":{"*":"viewer"," ":"admin"}}`
	if code := admin.do("PATCH", "/api/v1/auth/sso", body, &s); code != 200 {
		t.Fatalf("enable: %d", code)
	}
	if !s.Enabled || !s.ClientSecretSet || strings.Join(s.Scopes, " ") != "openid profile email" || len(s.RoleMapping) != 1 {
		t.Fatalf("after enable = %+v", s)
	}

	// Saved to the file: comments kept, the secret encrypted, the locked key untouched.
	raw, _ := os.ReadFile(path)
	text := string(raw)
	if !strings.Contains(text, "# break-glass") || !strings.Contains(text, "client_secret: sealed:v1:") ||
		strings.Contains(text, "s3cret") || strings.Contains(text, "client_id") {
		t.Fatalf("config file:\n%s", text)
	}

	// Applied at once: the login page sees SSO, and a test sign-in works without
	// replacing the admin's session.
	var p api.AuthProviders
	if admin.do("GET", "/api/v1/auth/providers", "", &p); !p.OIDC.Enabled || p.OIDC.Name != "Degovan SSO" {
		t.Fatalf("providers = %+v", p)
	}
	idp.SetUser(oidctest.User{Subject: "u-1", Email: "rina@degovan.id", Groups: []string{"degovan/devops"}})
	hc := &http.Client{Jar: admin.http.Jar, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if strings.HasPrefix(req.URL.String(), idp.URL) || req.URL.Path == "/auth/oidc/callback" {
			return nil
		}
		return http.ErrUseLastResponse
	}}
	resp, err := hc.Get(srv.URL + "/auth/oidc/login?test=1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	loc, _ := resp.Location()
	if loc == nil || loc.Path != "/sso" || loc.Query().Get("ok") != "1" || !strings.Contains(loc.Query().Get("test"), "would sign in as viewer") {
		t.Fatalf("test sign-in redirect = %v", loc)
	}
	var me api.Me
	if admin.do("GET", "/api/v1/me", "", &me); me.User.Username != "root" {
		t.Fatalf("test sign-in replaced the session: %+v", me.User)
	}
	if n, _ := st.CountUsers(t.Context()); n != 1 {
		t.Fatalf("test sign-in created a user (%d users)", n)
	}

	var d api.OIDCDiscovery
	if code := admin.do("POST", "/api/v1/auth/sso/discover", `{"issuer":"`+idp.URL+`/"}`, &d); code != 200 || d.TokenEndpoint == "" {
		t.Fatalf("discover: %d %+v", code, d)
	}
	if code := admin.do("POST", "/api/v1/auth/sso/discover", `{"issuer":"ftp://x"}`, &e); code != 422 {
		t.Fatalf("bad issuer: %d", code)
	}

	// Without an admin who signs in with SSO, passwords cannot be turned off.
	if code := admin.do("PATCH", "/api/v1/auth/sso", `{"local_login":false}`, &e); code != 409 || e.Code != "lockout" {
		t.Fatalf("lockout: %d %+v", code, e)
	}

	var page api.AuditPage
	if admin.do("GET", "/api/v1/audit?action=sso.", "", &page); page.Total != 1 || !page.Entries[0].Sensitive {
		t.Fatalf("audit = %+v", page)
	}
}
