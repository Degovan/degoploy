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
	mux.Handle("GET /api/v1/auth/sso", auth.RequireAuth(a.authz.Require(auth.ActionUserManage, http.HandlerFunc(a.ssoStatus))))
}

// oidcLogin starts the authorization-code flow with PKCE.
func (a *API) oidcLogin(w http.ResponseWriter, r *http.Request) {
	if a.oidc == nil {
		a.loginError(w, r, "Single sign-on is not configured.")
		return
	}
	state, nonce, verifier := auth.RandomString(24), auth.RandomString(24), oauth2.GenerateVerifier()
	target, err := a.oidc.AuthURL(r.Context(), state, nonce, verifier)
	if err != nil {
		slog.Error("oidc login", "err", err)
		a.loginError(w, r, fmt.Sprintf("%s is unreachable right now. Try again later or sign in with a password.", a.cfg.OIDC.ProviderName))
		return
	}
	id := a.pendingSSO.Put(auth.PendingLogin{
		State: state, Nonce: nonce, Verifier: verifier, Next: safeNext(r.URL.Query().Get("next")),
	})
	http.SetCookie(w, &http.Cookie{
		Name: oidcCookie, Value: id, Path: oidcCookiePath, MaxAge: 600,
		HttpOnly: true, Secure: a.cfg.SecureCookies(), SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, target, http.StatusFound)
}

func (a *API) oidcCallback(w http.ResponseWriter, r *http.Request) {
	if a.oidc == nil {
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
	if e := q.Get("error"); e != "" {
		msg := firstNonEmpty(q.Get("error_description"), e)
		a.ssoFailed(r, ip, "", fmt.Sprintf("provider returned %s", e))
		a.loginError(w, r, fmt.Sprintf("%s did not sign you in: %s", a.cfg.OIDC.ProviderName, msg))
		return
	}
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(pending.State)) != 1 {
		a.ssoFailed(r, ip, "", "state mismatch")
		a.loginError(w, r, "The sign-in response did not match this browser session. Please try again.")
		return
	}

	claims, err := a.oidc.Exchange(r.Context(), q.Get("code"), pending.Verifier, pending.Nonce)
	if err != nil {
		slog.Error("oidc callback", "err", err)
		a.ssoFailed(r, ip, "", err.Error())
		a.loginError(w, r, fmt.Sprintf("Signing in with %s failed. Ask an admin to check the SSO settings.", a.cfg.OIDC.ProviderName))
		return
	}

	entry := SSOLogin{
		At: time.Now().UTC().Format(time.RFC3339), Subject: claims.Subject, Email: claims.Email,
		Groups: nonNil(claims.Groups), Claims: claims.Raw,
	}
	role, allowed := auth.MapRole(a.cfg.OIDC.RoleMapping, claims.Groups)
	if !allowed {
		entry.Outcome = "denied: no group in role_mapping"
		a.sso.add(entry)
		a.ssoFailed(r, ip, claims.Subject, "no mapped group")
		a.loginError(w, r, "Your account is not in a group that may use degoploy. Ask an admin for access.")
		return
	}
	entry.Role = string(role)

	u, created, err := a.upsertSSOUser(r, claims, role)
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
func (a *API) upsertSSOUser(r *http.Request, c *auth.OIDCClaims, role store.Role) (*store.User, bool, error) {
	ctx := r.Context()
	displayName := firstNonEmpty(c.Name, c.Username, c.Email)
	u, err := a.store.UserByOIDC(ctx, c.Issuer, c.Subject)
	switch {
	case err == nil:
		next := *u
		next.Email, next.DisplayName = c.Email, firstNonEmpty(displayName, u.DisplayName)
		if a.cfg.OIDC.SyncRoleOnLogin {
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

func (a *API) ssoStatus(w http.ResponseWriter, r *http.Request) {
	o := a.cfg.OIDC
	httpx.JSON(w, http.StatusOK, SSOStatus{
		Enabled: o.Enabled, LocalLogin: a.cfg.LocalLogin, ProviderName: o.ProviderName, Issuer: o.Issuer,
		ClientID: o.ClientID, RedirectURL: o.RedirectURL, Scopes: nonNil(o.Scopes), GroupsClaim: o.GroupsClaim,
		RoleMapping: o.RoleMapping, SyncRoleOnLogin: o.SyncRoleOnLogin, RecentLogins: a.sso.list(),
	})
}

func (a *API) ssoFailed(r *http.Request, ip, subject, reason string) {
	a.audit.Record(r.Context(), audit.Entry{Action: audit.LoginFailed, IP: ip,
		Detail: map[string]any{"provider": store.ProviderOIDC, "subject": subject, "reason": reason}})
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
