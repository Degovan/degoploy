// Package httpx holds small HTTP helpers shared by handlers and middleware.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type ErrorBody struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
	// Field names the offending input field for validation errors.
	Field string `json:"field,omitempty"`
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("write json response", "err", err)
	}
}

func Error(w http.ResponseWriter, status int, code, msg string) {
	JSON(w, status, ErrorBody{Error: msg, Code: code})
}

// InternalError logs err and replies with a generic 500.
func InternalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("internal error", "method", r.Method, "path", r.URL.Path, "err", err)
	Error(w, http.StatusInternalServerError, "internal", "internal server error")
}

// DecodeJSON reads a JSON body (max 1 MiB) into dst, rejecting unknown fields.
// Requiring application/json also blocks cross-site form posts, which cannot set
// that content type without a CORS preflight. On failure it writes an error
// response and returns false.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(strings.ToLower(ct), "application/json") {
		Error(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		Error(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("invalid JSON body: %v", err))
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		Error(w, http.StatusBadRequest, "bad_request", "body must contain a single JSON object")
		return false
	}
	return true
}

// ClientIP returns the caller address, honouring X-Forwarded-For only when the
// direct peer is a trusted proxy. It walks the chain right-to-left and returns
// the first address that is not itself trusted.
func ClientIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	peer = peer.Unmap()
	if !isTrusted(peer, trusted) {
		return peer.String()
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return peer.String()
	}
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			break
		}
		a = a.Unmap()
		if !isTrusted(a, trusted) {
			return a.String()
		}
		peer = a
	}
	return peer.String()
}

func isTrusted(a netip.Addr, trusted []netip.Prefix) bool {
	for _, p := range trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
