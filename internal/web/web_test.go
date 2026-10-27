package web

import "testing"

func TestAcceptsEncoding(t *testing.T) {
	cases := []struct {
		header, enc string
		want        bool
	}{
		{"gzip, deflate, br, zstd", "br", true},
		{"gzip, deflate", "br", false},
		{"gzip;q=0.8, br;q=0", "br", false},
		{"gzip;q=0.8, br;q=0", "gzip", true},
		{"GZIP", "gzip", true},
		{"", "gzip", false},
	}
	for _, c := range cases {
		if got := acceptsEncoding(c.header, c.enc); got != c.want {
			t.Errorf("acceptsEncoding(%q, %q) = %v, want %v", c.header, c.enc, got, c.want)
		}
	}
}
