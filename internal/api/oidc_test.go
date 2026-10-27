package api_test

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"

	"github.com/degovan/degoploy/internal/api"
	"github.com/degovan/degoploy/internal/auth/oidctest"
	"github.com/degovan/degoploy/internal/config"
	"github.com/degovan/degoploy/internal/store"
)

type ssoEnv struct {
	t   *testing.T
	srv string
	idp *oidctest.Server
	st  *store.Store
}

func newSSOEnv(t *testing.T, mapping map[string]string, sync bool) *ssoEnv {
	t.Helper()
	idp := oidctest.New("degoploy", "s3cret")
	t.Cleanup(idp.Close)
	srv, st := newTestServerWith(t, func(cfg *config.Config, base string) {
		cfg.OIDC = config.OIDC{
			Enabled: true, ProviderName: "Casdoor", Issuer: idp.URL, ClientID: "degoploy", ClientSecret: "s3cret",
			RedirectURL: base + "/auth/oidc/callback", Scopes: []string{"openid", "profile", "email"},
			GroupsClaim: "groups", RoleMapping: mapping, SyncRoleOnLogin: sync,
		}
	})
	return &ssoEnv{t: t, srv: srv.URL, idp: idp, st: st}
}

// login runs the full redirect dance in a fresh browser-like client and returns
// that client plus the final degoploy location (next page or /login?error=...).
func (e *ssoEnv) login(next string) (*client, *url.URL) {
	e.t.Helper()
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{Jar: jar, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		// Follow hops to the provider and back to the callback; stop at the final page.
		if strings.HasPrefix(req.URL.String(), e.idp.URL) || req.URL.Path == "/auth/oidc/callback" {
			return nil
		}
		return http.ErrUseLastResponse
	}}
	resp, err := hc.Get(e.srv + "/auth/oidc/login?next=" + url.QueryEscape(next))
	if err != nil {
		e.t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		e.t.Fatalf("login flow ended with %d", resp.StatusCode)
	}
	loc, _ := resp.Location()
	c := &client{t: e.t, base: e.srv, http: &http.Client{Jar: jar}}
	var me api.Me
	if c.do("GET", "/api/v1/me", "", &me) == 200 {
		c.csrf = me.CSRFToken
	}
	return c, loc
}

func (e *ssoEnv) me(c *client) api.Me {
	e.t.Helper()
	var me api.Me
	if code := c.do("GET", "/api/v1/me", "", &me); code != 200 {
		e.t.Fatalf("/me: %d", code)
	}
	return me
}

func loginError(loc *url.URL) string {
	if loc.Path != "/login" {
		return ""
	}
	return loc.Query().Get("error")
}

var mapping = map[string]string{"acme/degoploy-admins": "admin", "acme/devops": "deployer"}

func TestSSOLoginCreatesAndSyncsUser(t *testing.T) {
	e := newSSOEnv(t, mapping, true)
	createUser(t, e.st, "dewi", "local-password-1", store.RoleViewer)

	// "dewi" is taken by a local account: the SSO user must get a new username,
	// never be linked to the local one.
	e.idp.SetUser(oidctest.User{Subject: "u-1", Email: "dewi@acme.io", Name: "Dewi Lestari", Username: "dewi",
		Groups: []string{"acme/everyone", "acme/degoploy-admins"}})
	c, loc := e.login("/projects/7")
	if loc.Path != "/projects/7" {
		t.Fatalf("redirected to %s (%s)", loc, loginError(loc))
	}
	me := e.me(c)
	if me.User.Username != "dewi-2" || me.User.AuthProvider != "oidc" || me.User.Role != "admin" || me.User.DisplayName != "Dewi Lestari" {
		t.Fatalf("created user = %+v", me.User)
	}
	local, _ := e.st.UserByUsername(context.Background(), "dewi")
	if local.AuthProvider != "local" || local.OIDCSubject != "" {
		t.Fatalf("local account was modified: %+v", local)
	}

	// Next login with different groups: role follows the provider (sync on).
	e.idp.SetUser(oidctest.User{Subject: "u-1", Email: "dewi@acme.io", Name: "Dewi L.", Username: "dewi",
		Groups: []string{"acme/devops"}})
	c, _ = e.login("/projects")
	if me := e.me(c); me.User.Role != "deployer" || me.User.Username != "dewi-2" || me.User.DisplayName != "Dewi L." {
		t.Fatalf("synced user = %+v", me.User)
	}

	// Disabled accounts cannot sign in via SSO either.
	u, _ := e.st.UserByOIDC(context.Background(), e.idp.URL, "u-1")
	u.Disabled = true
	e.st.UpdateUser(context.Background(), u)
	if _, loc := e.login("/"); !strings.Contains(loginError(loc), "disabled") {
		t.Fatalf("disabled login went to %s", loc)
	}

	// The admin debug page lists recent attempts with their groups and outcome.
	createUser(t, e.st, "root", "root-password-1", store.RoleAdmin)
	admin := login(t, e.srv, "root", "root-password-1")
	var status api.SSOStatus
	if code := admin.do("GET", "/api/v1/auth/sso", "", &status); code != 200 || !status.Enabled || status.ProviderName != "Casdoor" {
		t.Fatalf("sso status: %d %+v", code, status)
	}
	if len(status.RecentLogins) != 3 || status.RecentLogins[0].Outcome != "denied: account disabled" ||
		status.RecentLogins[2].Outcome != "signed in (account created)" || status.RecentLogins[2].Role != "admin" {
		t.Fatalf("recent logins = %+v", status.RecentLogins)
	}
	if status.RecentLogins[2].Claims["email"] != "dewi@acme.io" {
		t.Fatalf("claims missing: %+v", status.RecentLogins[2].Claims)
	}
}

func TestSSORoleMapping(t *testing.T) {
	e := newSSOEnv(t, mapping, false)

	// No mapped group and no "*" fallback: refused.
	e.idp.SetUser(oidctest.User{Subject: "u-2", Username: "eko", Groups: []string{"acme/marketing"}})
	if _, loc := e.login("/"); !strings.Contains(loginError(loc), "not in a group") {
		t.Fatalf("unmapped user went to %s", loc)
	}
	if n, _ := e.st.CountUsers(context.Background()); n != 0 {
		t.Fatalf("refused user must not be created, users = %d", n)
	}

	// Groups delivered only via userinfo, as Casdoor-style owner/name objects.
	e.idp.GroupsInIDToken = false
	e.idp.SetUser(oidctest.User{Subject: "u-3", Email: "fajar@acme.io", Groups: []any{
		map[string]any{"owner": "acme", "name": "devops"},
	}})
	c, loc := e.login("/")
	if loginError(loc) != "" {
		t.Fatalf("userinfo groups: %s", loginError(loc))
	}
	if me := e.me(c); me.User.Role != "deployer" || me.User.Username != "fajar" {
		t.Fatalf("user = %+v", me.User)
	}

	// With sync off, a later group change does not alter the stored role.
	e.idp.GroupsInIDToken = true
	e.idp.SetUser(oidctest.User{Subject: "u-3", Email: "fajar@acme.io", Groups: []string{"acme/degoploy-admins"}})
	c, _ = e.login("/")
	if me := e.me(c); me.User.Role != "deployer" {
		t.Fatalf("role changed without sync: %+v", me.User)
	}
}

func TestSSOWildcardAndFailures(t *testing.T) {
	e := newSSOEnv(t, map[string]string{"acme/degoploy-admins": "admin", "*": "viewer"}, true)

	e.idp.SetUser(oidctest.User{Subject: "u-4", Email: "gita@acme.io"})
	c, _ := e.login("/")
	if me := e.me(c); me.User.Role != "viewer" {
		t.Fatalf("wildcard role = %s", me.User.Role)
	}

	e.idp.Deny = true
	if _, loc := e.login("/"); !strings.Contains(loginError(loc), "user denied access") {
		t.Fatalf("denied: %s", loc)
	}
	e.idp.Deny = false

	e.idp.WrongNonce = true
	if _, loc := e.login("/"); !strings.Contains(loginError(loc), "failed") {
		t.Fatalf("wrong nonce accepted: %s", loc)
	}
	e.idp.WrongNonce = false

	// A callback without the browser's login cookie (e.g. replayed) is rejected.
	resp, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).
		Get(e.srv + "/auth/oidc/callback?code=x&state=y")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	loc, _ := resp.Location()
	if !strings.Contains(loginError(loc), "expired") {
		t.Fatalf("callback without cookie: %s", loc)
	}

	// Open redirects through ?next= are neutralised.
	_, loc = e.login("//evil.example/x")
	if loc.Path != "/projects" || !strings.HasPrefix(loc.String(), e.srv+"/") {
		t.Fatalf("next not sanitised: %s", loc)
	}
}

func TestSSOProviderDown(t *testing.T) {
	srv, _ := newTestServerWith(t, func(cfg *config.Config, base string) {
		cfg.OIDC = config.OIDC{Enabled: true, ProviderName: "Casdoor", Issuer: "http://127.0.0.1:1",
			ClientID: "x", RedirectURL: base + "/auth/oidc/callback", RoleMapping: mapping, GroupsClaim: "groups"}
	})
	resp, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).
		Get(srv.URL + "/auth/oidc/login")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	loc, _ := resp.Location()
	if !strings.Contains(loginError(loc), "unreachable") {
		t.Fatalf("provider down: %s", loc)
	}
}
