package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/degovan/degoploy/internal/audit"
	"github.com/degovan/degoploy/internal/auth"
	"github.com/degovan/degoploy/internal/config"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/store"
)

func (a *API) registerSSO(mux *http.ServeMux) {
	need := func(fn http.HandlerFunc) http.Handler {
		return auth.RequireAuth(a.authz.Require(auth.ActionUserManage, fn))
	}
	mux.Handle("GET /api/v1/auth/sso", need(a.ssoStatus))
	mux.Handle("PATCH /api/v1/auth/sso", need(a.updateSSO))
	mux.Handle("POST /api/v1/auth/sso/discover", need(a.discoverOIDC))
}

func (a *API) ssoStatus(w http.ResponseWriter, r *http.Request) {
	out, err := a.ssoStatusBody(r)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (a *API) ssoStatusBody(r *http.Request) (SSOStatus, error) {
	local, o, _ := a.signIn()
	out := SSOStatus{
		Enabled: o.Enabled, LocalLogin: local, ProviderName: o.ProviderName, Issuer: o.Issuer,
		ClientID: o.ClientID, ClientSecretSet: o.ClientSecret != "", RedirectURL: o.RedirectURL,
		Scopes: nonNil(o.Scopes), GroupsClaim: o.GroupsClaim, RoleMapping: o.RoleMapping,
		SyncRoleOnLogin: o.SyncRoleOnLogin, Locked: a.cfg.Locked, ConfigPath: a.cfg.Path,
		RecentLogins: a.sso.list(),
	}
	if out.RoleMapping == nil {
		out.RoleMapping = map[string]string{}
	}
	if out.Locked == nil {
		out.Locked = map[string]string{}
	}
	users, err := a.store.ListUsers(r.Context())
	if err != nil {
		return out, err
	}
	var lastLocal, lastSSO *time.Time
	latest := func(cur **time.Time, t *time.Time) {
		if t != nil && (*cur == nil || t.After(**cur)) {
			*cur = t
		}
	}
	for _, u := range users {
		if u.AuthProvider == store.ProviderOIDC {
			out.SSOUsers++
			latest(&lastSSO, u.LastLoginAt)
		} else {
			out.LocalUsers++
			latest(&lastLocal, u.LastLoginAt)
		}
	}
	out.LastLocalLogin, out.LastSSOLogin = msTimePtr(lastLocal), msTimePtr(lastSSO)
	return out, nil
}

// errLockout is returned when turning off password sign-in would leave no admin
// able to sign in.
var errLockout = errors.New("no active admin signs in with SSO yet, so turning off passwords would lock every admin out")

// updateSSO saves the sign-in settings to the config file and applies them at
// once, without a restart.
func (a *API) updateSSO(w http.ResponseWriter, r *http.Request) {
	ident := auth.FromContext(r.Context())
	var req UpdateSSORequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	changed, field, err := a.applySSO(r, &req)
	switch {
	case field != "":
		validationError(w, field, err.Error())
		return
	case errors.Is(err, errLockout):
		httpx.Error(w, http.StatusConflict, "lockout", err.Error())
		return
	case errors.Is(err, errNoConfigFile):
		httpx.Error(w, http.StatusConflict, "no_config_file", err.Error())
		return
	case err != nil:
		httpx.InternalError(w, r, err)
		return
	}
	if len(changed) > 0 {
		a.audit.Record(r.Context(), audit.Entry{
			UserID: ident.User.ID, Action: audit.SSOUpdate, TargetType: "sso",
			Detail: map[string]any{"changed": changed}, IP: a.clientIP(r),
		})
	}
	a.ssoStatus(w, r)
}

var errNoConfigFile = errors.New("degoploy was started without a config file (--config), so there is nowhere to save settings")

// applySSO validates req against the current settings, writes the result to the
// config file and swaps it in. A non-empty field means a validation error on it.
func (a *API) applySSO(r *http.Request, req *UpdateSSORequest) (changed []string, field string, err error) {
	a.signInMu.Lock()
	defer a.signInMu.Unlock()

	local := a.cfg.LocalLogin
	o := a.cfg.OIDC
	o.Scopes, o.RoleMapping = slices.Clone(o.Scopes), maps.Clone(o.RoleMapping)
	locked := a.cfg.Locked

	// set applies one field unless it is unchanged; locked fields cannot change.
	set := func(key string, differs bool, apply func()) bool {
		if !differs {
			return true
		}
		if src, ok := locked[key]; ok {
			field, err = key, fmt.Errorf("is set by %s on the server and cannot be changed here", src)
			return false
		}
		apply()
		changed = append(changed, key)
		return true
	}
	boolField := func(key string, p *bool, dst *bool) bool {
		return p == nil || set(key, *p != *dst, func() { *dst = *p })
	}
	strField := func(key string, p *string, dst *string) bool {
		if p == nil {
			return true
		}
		v := strings.TrimSpace(*p)
		return set(key, v != *dst, func() { *dst = v })
	}
	ok := boolField("local_login", req.LocalLogin, &local) &&
		boolField("oidc.enabled", req.Enabled, &o.Enabled) &&
		strField("oidc.provider_name", req.ProviderName, &o.ProviderName) &&
		strField("oidc.issuer", req.Issuer, &o.Issuer) &&
		strField("oidc.client_id", req.ClientID, &o.ClientID) &&
		strField("oidc.redirect_url", req.RedirectURL, &o.RedirectURL) &&
		strField("oidc.groups_claim", req.GroupsClaim, &o.GroupsClaim) &&
		boolField("oidc.sync_role_on_login", req.SyncRoleOnLogin, &o.SyncRoleOnLogin)
	if !ok {
		return nil, field, err
	}
	if req.Scopes != nil {
		scopes := normalizeScopes(req.Scopes)
		if !set("oidc.scopes", !slices.Equal(scopes, o.Scopes), func() { o.Scopes = scopes }) {
			return nil, field, err
		}
	}
	if req.RoleMapping != nil {
		mapping := map[string]string{}
		for g, role := range req.RoleMapping {
			if g = strings.TrimSpace(g); g != "" {
				mapping[g] = role
			}
		}
		if !set("oidc.role_mapping", !maps.Equal(mapping, o.RoleMapping), func() { o.RoleMapping = mapping }) {
			return nil, field, err
		}
	}
	var sealed *string
	if req.ClientSecret != nil && strings.TrimSpace(*req.ClientSecret) != "" {
		secret := strings.TrimSpace(*req.ClientSecret)
		if !set("oidc.client_secret", true, func() { o.ClientSecret = secret }) {
			return nil, field, err
		}
		// The file gets the secret encrypted with the server key, never in plain text.
		v := config.SealedPrefix + a.secrets.Seal(secret, config.OIDCSecretScope)
		sealed = &v
	}
	if len(changed) == 0 {
		return nil, "", nil
	}

	if o.Issuer != "" {
		if u, perr := url.Parse(o.Issuer); perr != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return nil, "oidc.issuer", errors.New("must be an http(s) URL")
		}
	}
	if o.Enabled && o.GroupsClaim == "" {
		o.GroupsClaim = "groups"
	}
	if verr := config.ValidateSignIn(local, o); verr != nil {
		return nil, "sso", verr
	}
	if !local && a.cfg.LocalLogin {
		users, lerr := a.store.ListUsers(r.Context())
		if lerr != nil {
			return nil, "", lerr
		}
		ssoAdmin := slices.ContainsFunc(users, func(u *store.User) bool {
			return u.AuthProvider == store.ProviderOIDC && u.Role == store.RoleAdmin && !u.Disabled
		})
		if !ssoAdmin {
			return nil, "", errLockout
		}
	}

	if a.cfg.Path == "" {
		return nil, "", errNoConfigFile
	}
	if serr := config.SaveSignIn(a.cfg.Path, config.SignIn{LocalLogin: local, OIDC: o, ClientSecret: sealed}, locked); serr != nil {
		return nil, "", serr
	}
	a.cfg.LocalLogin, a.cfg.OIDC = local, o
	a.oidc = nil
	if o.Enabled {
		a.oidc = auth.NewOIDCProvider(o)
	}
	return changed, "", nil
}

// normalizeScopes trims and de-duplicates scopes and puts openid first.
func normalizeScopes(in []string) []string {
	out := []string{"openid"}
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// discoverOIDC fetches an issuer's discovery document so the SSO page can show
// whether the URL is right before it is saved.
func (a *API) discoverOIDC(w http.ResponseWriter, r *http.Request) {
	var req DiscoverRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	issuer := strings.TrimRight(strings.TrimSpace(req.Issuer), "/")
	if u, err := url.Parse(issuer); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		validationError(w, "issuer", "must be an http(s) URL")
		return
	}
	req2, err := http.NewRequestWithContext(r.Context(), http.MethodGet, issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		validationError(w, "issuer", err.Error())
		return
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req2)
	if err != nil {
		validationError(w, "issuer", fmt.Sprintf("could not reach the issuer: %v", err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		validationError(w, "issuer", fmt.Sprintf("no discovery document (HTTP %d)", resp.StatusCode))
		return
	}
	var doc struct {
		Issuer                string   `json:"issuer"`
		AuthorizationEndpoint string   `json:"authorization_endpoint"`
		TokenEndpoint         string   `json:"token_endpoint"`
		UserinfoEndpoint      string   `json:"userinfo_endpoint"`
		JWKSURI               string   `json:"jwks_uri"`
		ScopesSupported       []string `json:"scopes_supported"`
		ClaimsSupported       []string `json:"claims_supported"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		validationError(w, "issuer", "the discovery document is not valid JSON")
		return
	}
	if strings.TrimRight(doc.Issuer, "/") != issuer {
		validationError(w, "issuer", fmt.Sprintf("the provider calls itself %q; use exactly that", doc.Issuer))
		return
	}
	httpx.JSON(w, http.StatusOK, OIDCDiscovery{
		Issuer: doc.Issuer, AuthorizationEndpoint: doc.AuthorizationEndpoint, TokenEndpoint: doc.TokenEndpoint,
		UserinfoEndpoint: doc.UserinfoEndpoint, JWKSURI: doc.JWKSURI,
		ScopesSupported: nonNil(doc.ScopesSupported), ClaimsSupported: nonNil(doc.ClaimsSupported),
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	})
}
