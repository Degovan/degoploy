package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/store"
)

const (
	CookieName = "degoploy_session"
	CSRFHeader = "X-CSRF-Token"
)

type ctxKey struct{}

type Identity struct {
	User    *store.User
	Session *store.Session
}

// FromContext returns the authenticated identity, or nil.
func FromContext(ctx context.Context) *Identity {
	id, _ := ctx.Value(ctxKey{}).(*Identity)
	return id
}

func WithIdentity(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

type Sessions struct {
	store  *store.Store
	ttl    time.Duration
	secure bool
}

func NewSessions(s *store.Store, ttl time.Duration, secure bool) *Sessions {
	return &Sessions{store: s, ttl: ttl, secure: secure}
}

// Start creates a session for u and sets the cookie.
func (m *Sessions) Start(ctx context.Context, w http.ResponseWriter, r *http.Request, u *store.User, ip string) (*store.Session, error) {
	token := randomToken(32)
	sess := &store.Session{
		IDHash:    hashToken(token),
		UserID:    u.ID,
		CSRFToken: randomToken(32),
		ExpiresAt: time.Now().Add(m.ttl),
		UserAgent: truncate(r.UserAgent(), 255),
		IP:        ip,
	}
	if err := m.store.CreateSession(ctx, sess); err != nil {
		return nil, err
	}
	m.setCookie(w, token, sess.ExpiresAt)
	return sess, nil
}

func (m *Sessions) End(ctx context.Context, w http.ResponseWriter, sess *store.Session) error {
	m.setCookie(w, "", time.Unix(0, 0))
	if sess == nil {
		return nil
	}
	return m.store.DeleteSession(ctx, sess.IDHash)
}

// Load attaches the identity from the session cookie (if valid) to the request context.
// It also slides the expiry once less than half the TTL remains.
func (m *Sessions) Load(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		sess, u, err := m.store.SessionWithUser(r.Context(), hashToken(c.Value))
		if err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				slog.Error("load session", "err", err)
			}
			next.ServeHTTP(w, r)
			return
		}
		if u.Disabled {
			next.ServeHTTP(w, r)
			return
		}
		if time.Until(sess.ExpiresAt) < m.ttl/2 {
			sess.ExpiresAt = time.Now().Add(m.ttl)
			if err := m.store.ExtendSession(r.Context(), sess.IDHash, sess.ExpiresAt); err == nil {
				m.setCookie(w, c.Value, sess.ExpiresAt)
			}
		}
		next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), &Identity{User: u, Session: sess})))
	})
}

// RequireAuth rejects anonymous requests and enforces the CSRF header on unsafe methods.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := FromContext(r.Context())
		if id == nil {
			httpx.Error(w, http.StatusUnauthorized, "unauthenticated", "login required")
			return
		}
		if !isSafeMethod(r.Method) && !validCSRF(id.Session.CSRFToken, r.Header.Get(CSRFHeader)) {
			httpx.Error(w, http.StatusForbidden, "csrf", "missing or invalid CSRF token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Sessions) setCookie(w http.ResponseWriter, value string, expires time.Time) {
	c := &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	}
	if value == "" {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func validCSRF(expected, got string) bool {
	return got != "" && subtle.ConstantTimeCompare([]byte(expected), []byte(got)) == 1
}

func randomToken(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
