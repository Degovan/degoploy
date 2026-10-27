package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestBackup(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(ctx, filepath.Join(dir, "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateUser(ctx, &User{Username: "ana", AuthProvider: ProviderLocal, Role: RoleAdmin, PasswordHash: "x"}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "backup.db")
	if err := s.Backup(ctx, out); err != nil {
		t.Fatal(err)
	}
	if err := s.Backup(ctx, out); err == nil {
		t.Fatal("backup over an existing file must fail")
	}
	b, err := Open(ctx, out)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if u, err := b.UserByUsername(ctx, "ana"); err != nil || u.Role != RoleAdmin {
		t.Fatalf("restored user: %+v %v", u, err)
	}
}
