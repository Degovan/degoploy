package auth

import (
	"slices"
	"testing"

	"github.com/degovan/degoploy/internal/store"
)

func TestMapRole(t *testing.T) {
	m := map[string]string{"org/admins": "admin", "org/devops": "deployer", "org/qa": "viewer"}
	cases := []struct {
		groups []string
		want   store.Role
		ok     bool
	}{
		{[]string{"org/qa", "org/admins", "org/devops"}, store.RoleAdmin, true}, // highest wins
		{[]string{"org/qa"}, store.RoleViewer, true},
		{[]string{"org/sales"}, "", false},
		{nil, "", false},
	}
	for _, c := range cases {
		if got, ok := MapRole(m, c.groups); got != c.want || ok != c.ok {
			t.Errorf("%v: got %q %v", c.groups, got, ok)
		}
	}
	m["*"] = "viewer"
	if got, ok := MapRole(m, []string{"org/sales"}); got != store.RoleViewer || !ok {
		t.Errorf("wildcard: %q %v", got, ok)
	}
	if got, _ := MapRole(map[string]string{"g": "owner"}, []string{"g"}); got != "" {
		t.Errorf("invalid role mapped: %q", got)
	}
}

func TestNormalizeGroups(t *testing.T) {
	in := []any{"org/a", " org/a ", map[string]any{"owner": "org", "name": "b"}, map[string]any{"name": "org/c"}, 42}
	if got := normalizeGroups(in); !slices.Equal(got, []string{"org/a", "org/b", "org/c"}) {
		t.Fatalf("got %v", got)
	}
	if got := normalizeGroups("solo"); !slices.Equal(got, []string{"solo"}) {
		t.Fatalf("string claim: %v", got)
	}
	if got := normalizeGroups(nil); got != nil {
		t.Fatalf("nil claim: %v", got)
	}
}
