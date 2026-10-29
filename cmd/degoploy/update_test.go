package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestOlderVersion(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"v1.2.3", "v1.2.4", true},
		{"v1.2.4", "v1.2.3", false},
		{"v1.9.0", "v1.10.0", true},
		{"v2.0.0", "v1.10.0", false},
		{"v1.2.0-rc.1", "v1.2.0", true},
		{"v1.2.0", "v1.2.0-rc.1", false},
		{"v1.2.0", "v1.2.0", false},
		{"dev", "v1.0.0", true},
		{"v1.0.0", "dev", false},
	} {
		if got := olderVersion(c.a, c.b); got != c.want {
			t.Errorf("olderVersion(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestFetchProgress(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 300<<10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Write(body)
	}))
	defer srv.Close()
	var calls int
	var last, total int64
	got, err := fetch(context.Background(), srv.URL, func(d, t int64) { calls, last, total = calls+1, d, t })
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("fetch = %d bytes, %v", len(got), err)
	}
	if calls < 2 || last != int64(len(body)) || total != int64(len(body)) {
		t.Fatalf("progress: %d calls, last %d of %d", calls, last, total)
	}
}
