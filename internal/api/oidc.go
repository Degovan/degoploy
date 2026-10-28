package api

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/degovan/degoploy/internal/audit"
	"github.com/degovan/degoploy/internal/auth"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/store"
)

const (
	oidcCookie     = "degoploy_oidc"
	oidcCookiePath = "/auth/oidc"
	ssoHistory     = 20
)

// ssoLog keeps the most recent SSO sign-ins in memory for the debug page.
type ssoLog struct {
	mu      sync.Mutex
	entries []SSOLogin
}

func (l *ssoLog) add(e SSOLogin) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append([]SSOLogin{e}, l.entries...)
	if len(l.entries) > ssoHistory {
		l.entries = l.entries[:ssoHistory]
	}
}

func (l *ssoLog) list() []SSOLogin {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]SSOLogin{}, l.entries...)
}

func (a *API) registerOIDC(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/oidc/login", a.oidcLogin)
	mux.HandleFunc("GET /auth/oidc/callback", a.oidcCallback)
	a.registerSSO(mux)
}

// oidcLogin starts the authorization-code flow with PKCE.
func (a *API) oidcLogin(w http.ResponseWriter, r *http.Request) {
	_, o, provider := a.signIn()
	if provider == nil {
		a.loginError(w, r, "Single sign-on is not configured.")
		return
	}
	pending := auth.PendingLogin{Next: safeNext(r.URL.Query().Get("next"))}
	if r.URL.Query().Get("test") == "1" {
		// Only someone who manages SSO may run a test; it never signs anyone in.
		ident := auth.FromContext(r.Context())
		if ident == nil || !a.authz.Can(r.Context(), ident.User, auth.ActionUserManage, 0) {
			a.loginError(w, r, "Sign in as an admin to test single sign-on.")
			return
		}
		pending.Test, pending.Next = true, "/sso"
	}
	state, nonce, verifier := auth.RandomString(24), auth.RandomString(24), oauth2.GenerateVerifier()
	target, err := provider.AuthURL(r.Context(), state, nonce, verifier)
	if err != nil {
		slog.Error("oidc login", "err", err)
		msg := fmt.Sprintf("%s is unreachable right now. Try again later or sign in with a password.", o.ProviderName)
		if pending.Test {
			a.testResult(w, r, false, fmt.Sprintf("%s is unreachable: %v", o.ProviderName, err))
			return
		}
		a.loginError(w, r, msg)
		return
	}
	pending.State, pending.Nonce, pending.Verifier = state, nonce, verifier
	id := a.pendingSSO.Put(pending)
	http.SetCookie(w, &http.Cookie{
		Name: oidcCookie, Value: id, Path: oidcCookiePath, MaxAge: 600,
		HttpOnly: true, Secure: a.cfg.SecureCookies(), SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, target, http.StatusFound)
}

func (a *API) oidcCallback(w http.ResponseWriter, r *http.Request) {
	_, o, provider := a.signIn()
	if provider == nil {
		a.loginError(w, r, "Single sign-on is not configured.")
		return
	}
	ip := a.clientIP(r)
	q := r.URL.Query()

	var pending auth.PendingLogin
	var ok bool
	if c, err := r.Cookie(oidcCookie); err == nil {
		pending, ok = a.pendingSSO.Take(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: oidcCookie, Path: oidcCookiePath, MaxAge: -1, HttpOnly: true,
		Secure: a.cfg.SecureCookies(), SameSite: http.SameSiteLaxMode})
	if !ok {
		a.loginError(w, r, "The sign-in attempt expired or was started in another browser. Please try again.")
		return
	}
	// A test reports to the SSO page and is not a sign-in attempt worth auditing.
	fail := func(subject, reason, msg string) {
		if pending.Test {
			a.testResult(w, r, false, firstNonEmpty(reason, msg))
			return
		}
		a.ssoFailed(r, ip, subject, reason)
		a.loginError(w, r, msg)
	}
	if e := q.Get("error"); e != "" {
		msg := firstNonEmpty(q.Get("error_description"), e)
		fail("", fmt.Sprintf("provider returned %s", e), fmt.Sprintf("%s did not sign you in: %s", o.ProviderName, msg))
		return
	}
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(pending.State)) != 1 {
		fail("", "state mismatch", "The sign-in response did not match this browser session. Please try again.")
		return
	}

	claims, err := provider.Exchange(r.Context(), q.Get("code"), pending.Verifier, pending.Nonce)
	if err != nil {
		slog.Error("oidc callback", "err", err)
		fail("", err.Error(), fmt.Sprintf("Signing in with %s failed. Ask an admin to check the SSO settings.", o.ProviderName))
		return
	}

	entry := SSOLogin{
		At: time.Now().UTC().Format(time.RFC3339), Subject: claims.Subject, Email: claims.Email,
		Groups: nonNil(claims.Groups), Claims: claims.Raw,
	}
	role, allowed := auth.MapRole(o.RoleMapping, claims.Groups)
	if pending.Test {
		who := firstNonEmpty(claims.Email, claims.Username, claims.Subject)
		groups := "no groups"
		if len(claims.Groups) > 0 {
			groups = "groups " + strings.Join(claims.Groups, ", ")
		}
		if allowed {
			entry.Role, entry.Outcome = string(role), "test: allowed"
			a.sso.add(entry)
			a.testResult(w, r, true, fmt.Sprintf("%s (%s) would sign in as %s.", who, groups, role))
		} else {
			entry.Outcome = "test: denied, no group in role_mapping"
			a.sso.add(entry)
			a.testResult(w, r, false, fmt.Sprintf("%s (%s) matches no role mapping and would be refused.", who, groups))
		}
		return
	}
	if !allowed {
		entry.Outcome = "denied: no group in role_mapping"
		a.sso.add(entry)
		a.ssoFailed(r, ip, claims.Subject, "no mapped group")
		a.loginError(w, r, "Your account is not in a group that may use degoploy. Ask an admin for access.")
		return
	}
	entry.Role = string(role)

	u, created, err := a.upsertSSOUser(r, claims, role, o.SyncRoleOnLogin)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	entry.Username = u.Username
	if u.Disabled {
		entry.Outcome = "denied: account disabled"
		a.sso.add(entry)
		a.ssoFailed(r, ip, claims.Subject, "account disabled")
		a.loginError(w, r, "Your degoploy account is disabled.")
		return
	}

	if _, err := a.sessions.Start(r.Context(), w, r, u, ip); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if err := a.store.TouchUserLogin(r.Context(), u.ID); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	entry.Outcome = "signed in"
	if created {
		entry.Outcome = "signed in (account created)"
	}
	a.sso.add(entry)
	a.audit.Record(r.Context(), audit.Entry{
		UserID: u.ID, Action: audit.LoginSuccess, IP: ip,
		Detail: map[string]any{"provider": store.ProviderOIDC, "groups": claims.Groups, "role": u.Role, "created": created},
	})
	http.Redirect(w, r, pending.Next, http.StatusFound)
}

// upsertSSOUser finds the account for the provider subject or creates one.
// Accounts are matched only by issuer+subject, never by username or email, so an
// identity provider user can never take over an existing local account.
func (a *API) upsertSSOUser(r *http.Request, c *auth.OIDCClaims, role store.Role, syncRole bool) (*store.User, bool, error) {
	ctx := r.Context()
	displayName := firstNonEmpty(c.Name, c.Username, c.Email)
	u, err := a.store.UserByOIDC(ctx, c.Issuer, c.Subject)
	switch {
	case err == nil:
		next := *u
		next.Email, next.DisplayName = c.Email, firstNonEmpty(displayName, u.DisplayName)
		if syncRole {
			next.Role = role
		}
		if next != *u {
			if err := a.store.UpdateUser(ctx, &next); err != nil {
				return nil, false, err
			}
		}
		return &next, false, nil
	case !errors.Is(err, store.ErrNotFound):
		return nil, false, err
	}

	base := ssoUsername(c)
	for i := 1; i <= 100; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		nu := &store.User{
			Username: name, Email: c.Email, DisplayName: firstNonEmpty(displayName, name),
			AuthProvider: store.ProviderOIDC, OIDCIssuer: c.Issuer, OIDCSubject: c.Subject, Role: role,
		}
		err := a.store.CreateUser(ctx, nu)
		if err == nil {
			return nu, true, nil
		}
		if !errors.Is(err, store.ErrDuplicate) {
			return nil, false, err
		}
		// A concurrent callback may have created this subject's account meanwhile.
		if existing, err := a.store.UserByOIDC(ctx, c.Issuer, c.Subject); err == nil {
			return existing, false, nil
		}
	}
	return nil, false, fmt.Errorf("no free username for %q", base)
}

var usernameUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// ssoUsername derives a valid local username from the provider's claims.
func ssoUsername(c *auth.OIDCClaims) string {
	candidate := firstNonEmpty(c.Username, strings.SplitN(c.Email, "@", 2)[0], c.Name, "user")
	name := strings.Trim(usernameUnsafe.ReplaceAllString(candidate, "-"), "-._")
	if len(name) > 56 {
		name = name[:56]
	}
	if len(name) < 2 {
		name = "user"
	}
	return name
}

func (a *API) ssoFailed(r *http.Request, ip, subject, reason string) {
	a.audit.Record(r.Context(), audit.Entry{Action: audit.LoginFailed, IP: ip,
		Detail: map[string]any{"provider": store.ProviderOIDC, "subject": subject, "reason": reason}})
}

// testResult sends an admin back to the SSO page with the outcome of a test sign-in.
func (a *API) testResult(w http.ResponseWriter, r *http.Request, ok bool, msg string) {
	v := url.Values{"test": {msg}}
	if ok {
		v.Set("ok", "1")
	}
	http.Redirect(w, r, "/sso?"+v.Encode(), http.StatusFound)
}

// loginError sends the browser back to the login page with a message to show.
func (a *API) loginError(w http.ResponseWriter, r *http.Request, msg string) {
	http.Redirect(w, r, "/login?error="+url.QueryEscape(msg), http.StatusFound)
}

// safeNext keeps post-login redirects on this site.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") ||
		strings.HasPrefix(next, "/login") || strings.HasPrefix(next, "/auth/") {
		return "/projects"
	}
	return next
}
