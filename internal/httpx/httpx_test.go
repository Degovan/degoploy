package httpx

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32"), netip.MustParsePrefix("10.0.0.0/8")}
	cases := []struct {
		name, remote, xff, want string
	}{
		{"direct", "203.0.113.5:1234", "", "203.0.113.5"},
		{"untrusted peer ignores xff", "203.0.113.5:1234", "1.2.3.4", "203.0.113.5"},
		{"trusted proxy", "127.0.0.1:80", "198.51.100.7", "198.51.100.7"},
		{"spoofed left entry ignored", "127.0.0.1:80", "6.6.6.6, 198.51.100.7, 10.1.2.3", "198.51.100.7"},
		{"garbage stops walk", "127.0.0.1:80", "nope", "127.0.0.1"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := ClientIP(r, trusted); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}
