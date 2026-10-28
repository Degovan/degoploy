package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/degovan/degoploy/internal/config"
	"github.com/degovan/degoploy/internal/store"
)

// OIDCClaims is what degoploy uses from an identity provider login.
type OIDCClaims struct {
	Issuer   string
	Subject  string
	Email    string
	Name     string
	Username string // preferred_username
	Groups   []string
	Raw      map[string]any // ID token claims merged with userinfo
}

// OIDCProvider wraps discovery, the authorization-code + PKCE flow and ID token
// verification. Discovery runs lazily (and is retried) so degoploy still starts
// while the identity provider is down.
type OIDCProvider struct {
	cfg    config.OIDC
	client *http.Client

	mu       sync.Mutex
	provider *oidc.Provider
	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
}

func NewOIDCProvider(cfg config.OIDC) *OIDCProvider {
	return &OIDCProvider{cfg: cfg, client: &http.Client{Timeout: 15 * time.Second}}
}

func (p *OIDCProvider) setup(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.provider != nil {
		return nil
	}
	prov, err := oidc.NewProvider(oidc.ClientContext(ctx, p.client), p.cfg.Issuer)
	if err != nil {
		return fmt.Errorf("OIDC discovery for %s: %w", p.cfg.Issuer, err)
	}
	scopes := slices.Clone(p.cfg.Scopes)
	if !slices.Contains(scopes, oidc.ScopeOpenID) {
		scopes = append([]string{oidc.ScopeOpenID}, scopes...)
	}
	p.provider = prov
	p.oauth = &oauth2.Config{
		ClientID:     p.cfg.ClientID,
		ClientSecret: p.cfg.ClientSecret,
		RedirectURL:  p.cfg.RedirectURL,
		Endpoint:     prov.Endpoint(),
		Scopes:       scopes,
	}
	p.verifier = prov.Verifier(&oidc.Config{ClientID: p.cfg.ClientID})
	return nil
}

// AuthURL returns the provider URL that starts a login.
func (p *OIDCProvider) AuthURL(ctx context.Context, state, nonce, pkceVerifier string) (string, error) {
	if err := p.setup(ctx); err != nil {
		return "", err
	}
	return p.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(pkceVerifier)), nil
}

// Exchange redeems the authorization code, verifies the ID token (signature,
// issuer, audience, expiry, nonce) and returns the user's claims.
func (p *OIDCProvider) Exchange(ctx context.Context, code, pkceVerifier, nonce string) (*OIDCClaims, error) {
	if err := p.setup(ctx); err != nil {
		return nil, err
	}
	ctx = oidc.ClientContext(ctx, p.client)
	tok, err := p.oauth.Exchange(ctx, code, oauth2.VerifierOption(pkceVerifier))
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok || rawID == "" {
		return nil, errors.New("token response has no id_token")
	}
	idt, err := p.verifier.Verify(ctx, rawID)
	if err != nil {
		return nil, fmt.Errorf("verify id_token: %w", err)
	}
	if idt.Nonce != nonce {
		return nil, errors.New("id_token nonce does not match this login")
	}
	raw := map[string]any{}
	if err := idt.Claims(&raw); err != nil {
		return nil, fmt.Errorf("decode id_token claims: %w", err)
	}

	// Userinfo fills claims missing from the ID token (e.g. groups when the
	// provider issues minimal ID tokens). The ID token always wins on conflicts.
	if info, err := p.provider.UserInfo(ctx, oauth2.StaticTokenSource(tok)); err == nil && info.Subject == idt.Subject {
		extra := map[string]any{}
		if info.Claims(&extra) == nil {
			for k, v := range extra {
				if _, exists := raw[k]; !exists {
					raw[k] = v
				}
			}
		}
	}

	return &OIDCClaims{
		Issuer:   idt.Issuer,
		Subject:  idt.Subject,
		Email:    str(raw["email"]),
		Name:     str(raw["name"]),
		Username: str(raw["preferred_username"]),
		Groups:   normalizeGroups(raw[p.cfg.GroupsClaim]),
		Raw:      raw,
	}, nil
}

// normalizeGroups accepts a list of strings, a single string, or a list of
// objects with "name" (and optionally "owner", as Casdoor emits) and returns
// group names; owner/name objects become "owner/name".
func normalizeGroups(v any) []string {
	var out []string
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	switch t := v.(type) {
	case string:
		add(t)
	case []any:
		for _, item := range t {
			switch g := item.(type) {
			case string:
				add(g)
			case map[string]any:
				name, owner := str(g["name"]), str(g["owner"])
				if owner != "" && name != "" && !strings.Contains(name, "/") {
					add(owner + "/" + name)
				} else {
					add(name)
				}
			}
		}
	}
	return out
}

var roleRank = map[store.Role]int{store.RoleViewer: 1, store.RoleDeployer: 2, store.RoleAdmin: 3}

// MapRole picks the most privileged role among the user's groups. The "*" entry,
// if present, applies when no group matches. ok is false when the user may not
// sign in at all.
func MapRole(mapping map[string]string, groups []string) (role store.Role, ok bool) {
	for _, g := range groups {
		if r := store.Role(mapping[g]); r.Valid() && roleRank[r] > roleRank[role] {
			role = r
		}
	}
	if role == "" {
		if r := store.Role(mapping["*"]); r.Valid() {
			role = r
		}
	}
	return role, role != ""
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// PendingLogins remembers in-flight SSO logins (state, nonce, PKCE verifier)
// between the redirect to the provider and its callback.
type PendingLogins struct {
	mu sync.Mutex
	m  map[string]PendingLogin
}

type PendingLogin struct {
	State    string
	Nonce    string
	Verifier string
	Next     string
	Expires  time.Time
	// Test checks the SSO settings for an admin: the callback reports the
	// result on the SSO page instead of signing anyone in.
	Test bool
}

const (
	pendingLoginTTL = 10 * time.Minute
	maxPending      = 10000
)

func NewPendingLogins() *PendingLogins { return &PendingLogins{m: map[string]PendingLogin{}} }

// Put stores l and returns the id to keep in the browser cookie.
func (p *PendingLogins) Put(l PendingLogin) string {
	id := randomToken(24)
	l.Expires = time.Now().Add(pendingLoginTTL)
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	for k, v := range p.m {
		if now.After(v.Expires) || len(p.m) >= maxPending {
			delete(p.m, k)
		}
	}
	p.m[id] = l
	return id
}

// Take returns and forgets a pending login; each id is usable once.
func (p *PendingLogins) Take(id string) (PendingLogin, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	l, ok := p.m[id]
	delete(p.m, id)
	if !ok || time.Now().After(l.Expires) {
		return PendingLogin{}, false
	}
	return l, true
}

// RandomString returns a URL-safe random string with n bytes of entropy.
func RandomString(n int) string { return randomToken(n) }
