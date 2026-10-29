package main

import "testing"

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
