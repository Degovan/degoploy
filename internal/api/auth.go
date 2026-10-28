package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/degovan/degoploy/internal/audit"
	"github.com/degovan/degoploy/internal/auth"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/store"
)

func (a *API) providers(w http.ResponseWriter, r *http.Request) {
	local, o, _ := a.signIn()
	httpx.JSON(w, http.StatusOK, AuthProviders{
		Local: local,
		OIDC:  OIDCProvider{Enabled: o.Enabled, Name: o.ProviderName},
	})
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	if local, _, _ := a.signIn(); !local {
		httpx.Error(w, http.StatusForbidden, "local_login_disabled", "local login is disabled")
		return
	}
	ip := a.clientIP(r)
	if !a.loginLimiter.Allow(ip) {
		httpx.Error(w, http.StatusTooManyRequests, "rate_limited", "too many login attempts, try again later")
		return
	}

	var req LoginRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)

	u, err := a.store.UserByUsername(r.Context(), req.Username)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		httpx.InternalError(w, r, err)
		return
	}
	hash := ""
	if u != nil && u.AuthProvider == store.ProviderLocal {
		hash = u.PasswordHash
	}
	if !auth.VerifyPassword(hash, req.Password) || u.Disabled {
		var uid int64
		if u != nil {
			uid = u.ID
		}
		a.audit.Record(r.Context(), audit.Entry{
			UserID: uid, Action: audit.LoginFailed, IP: ip,
			Detail: map[string]any{"username": req.Username},
		})
		httpx.Error(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
		return
	}

	// Drop any session the browser already had before issuing a new one.
	if prev := auth.FromContext(r.Context()); prev != nil {
		a.sessions.End(r.Context(), w, prev.Session)
	}
	sess, err := a.sessions.Start(r.Context(), w, r, u, ip)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if err := a.store.TouchUserLogin(r.Context(), u.ID); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	loginAt := time.Now().UTC()
	u.LastLoginAt = &loginAt
	a.audit.Record(r.Context(), audit.Entry{
		UserID: u.ID, Action: audit.LoginSuccess, IP: ip,
		Detail: map[string]any{"provider": store.ProviderLocal},
	})

	httpx.JSON(w, http.StatusOK, Me{
		User:        toUser(u),
		CSRFToken:   sess.CSRFToken,
		Permissions: a.permissions(r, u),
	})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
	if err := a.sessions.End(r.Context(), w, id.Session); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	a.audit.Record(r.Context(), audit.Entry{UserID: id.User.ID, Action: audit.Logout, IP: a.clientIP(r)})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
	httpx.JSON(w, http.StatusOK, Me{
		User:        toUser(id.User),
		CSRFToken:   id.Session.CSRFToken,
		Permissions: a.permissions(r, id.User),
	})
}

func (a *API) permissions(r *http.Request, u *store.User) []string {
	perms := a.authz.Permissions(r.Context(), u)
	out := make([]string, len(perms))
	for i, p := range perms {
		out[i] = string(p)
	}
	return out
}
