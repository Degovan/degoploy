package deploy

import (
	"bytes"
	"testing"
)

func TestRedactor(t *testing.T) {
	cases := []struct {
		name    string
		secrets []string
		writes  []string
		want    string
	}{
		{"whole", []string{"hunter2"}, []string{"password is hunter2\n"}, "password is ***\n"},
		{"split across writes", []string{"hunter2"}, []string{"pw=hun", "ter", "2 ok\n"}, "pw=*** ok\n"},
		{"false start is released", []string{"hunter2"}, []string{"hunt", "ing\n"}, "hunting\n"},
		{"held tail flushed at end", []string{"hunter2"}, []string{"ends with hunt"}, "ends with hunt"},
		{"longest wins", []string{"abc", "abcdef"}, []string{"x abcdef y abc\n"}, "x *** y ***\n"},
		{"each line of a multi-line value", []string{"-----BEGIN KEY-----\nAAAA\n-----END KEY-----"},
			[]string{"key: AAAA\n"}, "key: ***\n"},
		{"too short to mask", []string{"ab"}, []string{"ab ab\n"}, "ab ab\n"},
		{"no secrets", nil, []string{"plain\n"}, "plain\n"},
		{"repeated", []string{"s3cret"}, []string{"s3cret", "s3cret\n"}, "******\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			r := newRedactor(&out, c.secrets)
			for _, w := range c.writes {
				if n, err := r.Write([]byte(w)); err != nil || n != len(w) {
					t.Fatalf("Write(%q) = %d, %v", w, n, err)
				}
			}
			if err := r.Flush(); err != nil {
				t.Fatal(err)
			}
			if out.String() != c.want {
				t.Fatalf("got %q, want %q", out.String(), c.want)
			}
		})
	}
}
