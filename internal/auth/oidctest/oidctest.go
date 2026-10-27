// Package oidctest is a minimal OpenID Connect provider for tests: discovery,
// authorization code + PKCE, token, userinfo and JWKS, with RS256 ID tokens.
// The authorize endpoint signs in NextUser without any UI.
package oidctest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

type User struct {
	Subject  string
	Email    string
	Name     string
	Username string
	Groups   any // []string or Casdoor-style []map[string]any
}

type Server struct {
	*httptest.Server
	ClientID     string
	ClientSecret string

	mu       sync.Mutex
	NextUser User
	// GroupsInIDToken false leaves groups to the userinfo endpoint only.
	GroupsInIDToken bool
	// Deny makes /authorize answer with error=access_denied.
	Deny bool
	// WrongNonce issues ID tokens with a nonce that does not match the request.
	WrongNonce bool

	key    *rsa.PrivateKey
	codes  map[string]authRequest
	tokens map[string]User
}

type authRequest struct {
	user        User
	nonce       string
	challenge   string
	redirectURI string
}

func New(clientID, clientSecret string) *Server {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	s := &Server{
		ClientID: clientID, ClientSecret: clientSecret, GroupsInIDToken: true,
		key: key, codes: map[string]authRequest{}, tokens: map[string]User{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", s.discovery)
	mux.HandleFunc("GET /authorize", s.authorize)
	mux.HandleFunc("POST /token", s.token)
	mux.HandleFunc("GET /userinfo", s.userinfo)
	mux.HandleFunc("GET /jwks", s.jwks)
	s.Server = httptest.NewServer(mux)
	return s
}

func (s *Server) SetUser(u User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.NextUser = u
}

func (s *Server) discovery(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"issuer":                                s.URL,
		"authorization_endpoint":                s.URL + "/authorize",
		"token_endpoint":                        s.URL + "/token",
		"userinfo_endpoint":                     s.URL + "/userinfo",
		"jwks_uri":                              s.URL + "/jwks",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"code_challenge_methods_supported":      []string{"S256"},
	})
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redirect, err := url.Parse(q.Get("redirect_uri"))
	if err != nil || q.Get("client_id") != s.ClientID || q.Get("code_challenge_method") != "S256" {
		http.Error(w, "bad authorize request", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	deny, user := s.Deny, s.NextUser
	rq := redirect.Query()
	rq.Set("state", q.Get("state"))
	if deny {
		rq.Set("error", "access_denied")
		rq.Set("error_description", "user denied access")
	} else {
		code := random()
		s.codes[code] = authRequest{user: user, nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), redirectURI: q.Get("redirect_uri")}
		rq.Set("code", code)
	}
	s.mu.Unlock()
	redirect.RawQuery = rq.Encode()
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	id, secret, ok := r.BasicAuth()
	if !ok {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	if id != s.ClientID || secret != s.ClientSecret {
		http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
		return
	}
	s.mu.Lock()
	req, ok := s.codes[r.PostForm.Get("code")]
	delete(s.codes, r.PostForm.Get("code"))
	wrongNonce, groupsInID := s.WrongNonce, s.GroupsInIDToken
	s.mu.Unlock()

	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if !ok || req.redirectURI != r.PostForm.Get("redirect_uri") ||
		base64.RawURLEncoding.EncodeToString(sum[:]) != req.challenge {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	}

	nonce := req.nonce
	if wrongNonce {
		nonce = "not-the-nonce"
	}
	claims := map[string]any{
		"iss": s.URL, "sub": req.user.Subject, "aud": s.ClientID, "nonce": nonce,
		"iat": time.Now().Unix(), "exp": time.Now().Add(5 * time.Minute).Unix(),
		"email": req.user.Email, "name": req.user.Name, "preferred_username": req.user.Username,
	}
	if groupsInID && req.user.Groups != nil {
		claims["groups"] = req.user.Groups
	}
	signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: s.key, KeyID: "k1"}}, nil)
	idToken, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	access := random()
	s.mu.Lock()
	s.tokens[access] = req.user
	s.mu.Unlock()
	writeJSON(w, map[string]any{"access_token": access, "token_type": "Bearer", "expires_in": 300, "id_token": idToken})
}

func (s *Server) userinfo(w http.ResponseWriter, r *http.Request) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	s.mu.Lock()
	u, ok := s.tokens[h[min(len(prefix), len(h)):]]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	info := map[string]any{"sub": u.Subject, "email": u.Email, "name": u.Name, "preferred_username": u.Username}
	if u.Groups != nil {
		info["groups"] = u.Groups
	}
	writeJSON(w, info)
}

func (s *Server) jwks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
		{Key: &s.key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"},
	}})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func random() string {
	b := make([]byte, 18)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
