package admin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/degovan/degoploy/internal/audit"
	"github.com/degovan/degoploy/internal/config"
	"github.com/degovan/degoploy/internal/store"
)

// newSocket serves the admin API on a real Unix socket and returns a client for it.
func newSocket(t *testing.T) (*Client, *store.Store, string) {
	t.Helper()
	ctx := context.Background()
	dataDir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dataDir, "degoploy.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := config.Default()
	cfg.DataDir = dataDir
	cfg.Listen = "127.0.0.1:8090"

	// Short path: Unix socket paths are limited to about 100 bytes.
	dir, err := os.MkdirTemp("", "dgp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "admin.sock")
	l, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	sctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		Serve(sctx, l, NewHandler(Deps{Config: &cfg, Store: st, Audit: audit.New(st), Version: "test", Started: time.Now()}))
		close(done)
	}()
	t.Cleanup(func() { cancel(); <-done })

	c, err := Dial(sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c, st, sock
}

func TestSocketPermissions(t *testing.T) {
	_, _, sock := newSocket(t)
	fi, err := os.Stat(sock)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o660 {
		t.Fatalf("socket mode = %v, want 0660", got)
	}
	if _, err := Listen(sock); err == nil {
		t.Fatal("a second Listen on a live socket succeeded")
	}
}

func TestStaleSocketReplaced(t *testing.T) {
	dir, err := os.MkdirTemp("", "dgp")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, "admin.sock")
	l, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	// A crash leaves the file without a listener behind it.
	l.(interface{ SetUnlinkOnClose(bool) }).SetUnlinkOnClose(false)
	l.Close()
	if _, err := os.Stat(sock); err != nil {
		t.Fatalf("stale socket file missing: %v", err)
	}
	l, err = Listen(sock)
	if err != nil {
		t.Fatalf("Listen over a stale socket: %v", err)
	}
	l.Close()
}

func TestDialMissing(t *testing.T) {
	_, err := Dial(filepath.Join(t.TempDir(), "none.sock"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Dial of a missing socket = %v, want ErrNotExist", err)
	}
}

func TestUsersOverSocket(t *testing.T) {
	c, st, _ := newSocket(t)
	ctx := context.Background()

	s, err := c.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s.Users != 0 || s.Admins != 0 || s.Local || s.Version != "test" {
		t.Fatalf("status = %+v", s)
	}
	if want := CurrentPeer().Name; s.You != want {
		t.Fatalf("peer = %q, want %q", s.You, want)
	}

	if _, err := c.CreateUser(ctx, CreateUserRequest{Username: "x y", Role: "admin", Password: "long-enough-pw"}); !isField(err, "username") {
		t.Fatalf("bad username: %v", err)
	}
	if _, err := c.CreateUser(ctx, CreateUserRequest{Username: "ana", Role: "admin", Password: "short"}); !isField(err, "password") {
		t.Fatalf("short password: %v", err)
	}
	u, err := c.CreateUser(ctx, CreateUserRequest{Username: "ana", Email: "ana@example.com", Role: "admin", Password: "long-enough-pw"})
	if err != nil {
		t.Fatal(err)
	}
	if u.Role != "admin" || u.Provider != "local" {
		t.Fatalf("created %+v", u)
	}
	if _, err := c.CreateUser(ctx, CreateUserRequest{Username: "ana", Role: "viewer", Password: "long-enough-pw"}); !isField(err, "username") {
		t.Fatalf("duplicate: %v", err)
	}

	// The only admin may not be demoted or disabled.
	viewer := "viewer"
	if _, err := c.UpdateUser(ctx, "ana", UpdateUserRequest{Role: &viewer}); !isCode(err, "last_admin") {
		t.Fatalf("demote last admin: %v", err)
	}
	yes := true
	if _, err := c.UpdateUser(ctx, "ana", UpdateUserRequest{Disabled: &yes}); !isCode(err, "last_admin") {
		t.Fatalf("disable last admin: %v", err)
	}
	if _, err := c.UpdateUser(ctx, "nobody", UpdateUserRequest{Role: &viewer}); !isCode(err, "not_found") {
		t.Fatalf("unknown user: %v", err)
	}

	if _, err := c.CreateUser(ctx, CreateUserRequest{Username: "budi", Role: "deployer", Password: "long-enough-pw"}); err != nil {
		t.Fatal(err)
	}
	if u, err = c.UpdateUser(ctx, "budi", UpdateUserRequest{Disabled: &yes}); err != nil || !u.Disabled {
		t.Fatalf("disable: %+v %v", u, err)
	}
	if err := c.SetPassword(ctx, "budi", SetPasswordRequest{Password: "another-long-pw", RequireChange: true}); err != nil {
		t.Fatal(err)
	}
	users, err := c.Users(ctx)
	if err != nil || len(users) != 2 {
		t.Fatalf("users = %+v, %v", users, err)
	}
	if !users[1].MustChangePassword {
		t.Fatalf("require_change not stored: %+v", users[1])
	}

	// Changes are audited as the Linux user behind the socket.
	entries, err := st.ListAudit(ctx, store.AuditFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("got %d audit entries, want 4", len(entries))
	}
	var d map[string]any
	json.Unmarshal([]byte(entries[0].Detail), &d)
	if entries[0].Action != audit.UserPasswordReset || d["via"] != "cli" || d["os_user"] != CurrentPeer().Name || entries[0].UserID != nil {
		t.Fatalf("audit entry = %+v %v", entries[0], d)
	}
}

func TestBackupOverSocket(t *testing.T) {
	c, _, _ := newSocket(t)
	ctx := context.Background()
	if _, err := c.CreateUser(ctx, CreateUserRequest{Username: "ana", Role: "admin", Password: "long-enough-pw"}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	n, err := c.Backup(ctx, &buf, false)
	if err != nil {
		t.Fatal(err)
	}
	gz, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
	}
	if n != 1 || len(names) != 1 || names[0] != "degoploy.db" {
		t.Fatalf("backup files = %d %v", n, names)
	}
}

func TestInProcess(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "degoploy.db"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	closed := false
	c := InProcess(NewHandler(Deps{Config: &cfg, Store: st, Audit: audit.New(st), Local: true}), func() {
		closed = true
		st.Close()
	})
	s, err := c.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Local || s.You != CurrentPeer().Name {
		t.Fatalf("status = %+v", s)
	}
	c.Close()
	if !closed {
		t.Fatal("cleanup did not run")
	}
}

func TestCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "degoploy.yaml")
	os.WriteFile(path, []byte("listen: \":9000\"\ndata_dir: "+t.TempDir()+"\nmax_parallel: 0\n"), 0o600)
	if r := Check(context.Background(), path); r.Error == "" {
		t.Fatalf("max_parallel 0 accepted: %+v", r)
	}
	os.WriteFile(path, []byte("listen: \":9000\"\nbase_url: http://deploy.test\ndeploy_user: degoploy-deploy\n"), 0o600)
	r := Check(context.Background(), path)
	if r.Error != "" || len(r.Warnings) != 1 {
		t.Fatalf("report = %+v", r)
	}
	os.WriteFile(path, []byte("listen: \":9000\"\nbase_url: https://deploy.test\n"), 0o600)
	if r := Check(context.Background(), path); len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "deploy_user") {
		t.Fatalf("missing deploy_user not reported: %+v", r)
	}
}

func isField(err error, field string) bool {
	var e *Error
	return errors.As(err, &e) && e.Field == field
}

func isCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}
