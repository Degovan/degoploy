// Package api implements the JSON API served under /api/v1.
package api

import (
	"net/http"
	"time"

	"github.com/degovan/degoploy/internal/audit"
	"github.com/degovan/degoploy/internal/auth"
	"github.com/degovan/degoploy/internal/config"
	"github.com/degovan/degoploy/internal/deploy"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/project"
	"github.com/degovan/degoploy/internal/realtime"
	"github.com/degovan/degoploy/internal/secrets"
	"github.com/degovan/degoploy/internal/store"
)

type API struct {
	cfg      *config.Config
	store    *store.Store
	sessions *auth.Sessions
	authz    *auth.Authorizer
	audit    *audit.Logger
	projects *project.Manager
	deploys  *deploy.Service
	hub      *realtime.Hub
	secrets  *secrets.Box
	oidc     *auth.OIDCProvider // nil when SSO is disabled
	version  string

	pendingSSO *auth.PendingLogins
	sso        ssoLog

	loginLimiter *httpx.RateLimiter
	hookLimiter  *httpx.RateLimiter
}

type Deps struct {
	Config   *config.Config
	Store    *store.Store
	Sessions *auth.Sessions
	Authz    *auth.Authorizer
	Audit    *audit.Logger
	Projects *project.Manager
	Deploys  *deploy.Service
	Hub      *realtime.Hub
	Secrets  *secrets.Box
	OIDC     *auth.OIDCProvider
	Version  string
}

func New(d Deps) *API {
	a := &API{
		cfg:          d.Config,
		store:        d.Store,
		sessions:     d.Sessions,
		authz:        d.Authz,
		audit:        d.Audit,
		projects:     d.Projects,
		deploys:      d.Deploys,
		version:      d.Version,
		loginLimiter: httpx.NewRateLimiter(6*time.Second, 10),        // burst 10, then 10/min per IP
		hookLimiter:  httpx.NewRateLimiter(500*time.Millisecond, 60), // burst 60, then 120/min per IP
		hub:          d.Hub,
		secrets:      d.Secrets,
		oidc:         d.OIDC,
		pendingSSO:   auth.NewPendingLogins(),
	}
	d.Deploys.OnUpdate = a.publishDeployment
	return a
}

// Register mounts all routes on mux. Session loading happens in an outer middleware.
func (a *API) Register(mux *http.ServeMux) {
	authed := auth.RequireAuth

	mux.HandleFunc("GET /api/v1/health", a.health)
	mux.HandleFunc("GET /api/v1/auth/providers", a.providers)
	mux.HandleFunc("POST /api/v1/auth/login", a.login)
	mux.Handle("POST /api/v1/auth/logout", authed(http.HandlerFunc(a.logout)))
	mux.Handle("GET /api/v1/me", authed(http.HandlerFunc(a.me)))
	a.registerProjects(mux)
	a.registerVariables(mux)
	a.registerRules(mux)
	a.registerDeployments(mux)
	a.registerUsers(mux)
	a.registerOIDC(mux)
	mux.Handle("GET /ws", http.HandlerFunc(a.websocket))
	mux.HandleFunc("POST /hook/{publicID}", a.hook)

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, http.StatusNotFound, "not_found", "no such endpoint")
	})
}

// Sweep evicts idle rate-limiter entries; call periodically.
func (a *API) Sweep() {
	a.loginLimiter.Sweep(30 * time.Minute)
	a.hookLimiter.Sweep(30 * time.Minute)
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, Health{Status: "ok", Version: a.version})
}

func (a *API) clientIP(r *http.Request) string {
	return httpx.ClientIP(r, a.cfg.TrustedPrefixes)
}

func toUser(u *store.User) User {
	out := User{
		ID:           u.ID,
		Username:     u.Username,
		Email:        u.Email,
		DisplayName:  u.DisplayName,
		AuthProvider: u.AuthProvider,
		Role:         string(u.Role),
		Disabled:     u.Disabled,
		CreatedAt:    u.CreatedAt.UTC().Format(time.RFC3339),
	}
	if u.LastLoginAt != nil {
		s := u.LastLoginAt.UTC().Format(time.RFC3339)
		out.LastLoginAt = &s
	}
	return out
}
