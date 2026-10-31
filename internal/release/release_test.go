package release

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCleanShared(t *testing.T) {
	for in, want := range map[string]string{
		".env": ".env", "storage/": "storage/", "./public//uploads/": "public/uploads/",
		"a/../b": "b", "../etc": "", "/etc/passwd": "", ".": "", ".git/config": "", " ": "",
	} {
		if got := CleanShared(in); got != want {
			t.Errorf("CleanShared(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestActivateAndPrune(t *testing.T) {
	base := t.TempDir()
	for _, id := range []int64{1, 2, 3, 4, 5} {
		os.MkdirAll(Dir(base, id), 0o755)
	}
	op := func(action string, id int64, keep int) error {
		return Op{Action: action, Base: base, ID: id, Keep: keep}.Run(io.Discard)
	}
	if err := op(OpActivate, 2, 0); err != nil || Live(base) != 2 {
		t.Fatalf("activate 2: %v, live %d", err, Live(base))
	}
	if err := op(OpActivate, 9, 0); err == nil || Live(base) != 2 {
		t.Fatalf("activating a missing release: %v, live %d", err, Live(base))
	}
	// Keep 3: the live release (2), ID (4) and the newest other one (5).
	if err := op(OpPrune, 4, 3); err != nil {
		t.Fatal(err)
	}
	if got := List(base); !slices.Equal(got, []int64{5, 4, 2}) {
		t.Fatalf("after prune: %v", got)
	}
	if err := op(OpRemove, 2, 0); err != nil || !Exists(base, 2) {
		t.Fatalf("the live release must stay: %v", err)
	}
	if err := op(OpPrepare, 2, 0); err == nil {
		t.Fatal("rebuilding the live release must fail")
	}

	os.Remove(Current(base))
	os.Mkdir(Current(base), 0o755)
	if err := op(OpActivate, 5, 0); err == nil || !strings.Contains(err.Error(), "is a folder") {
		t.Fatalf("current as a folder: %v", err)
	}
}

func TestLinkMovesThenLinks(t *testing.T) {
	base := t.TempDir()
	rel := Dir(base, 1)
	os.MkdirAll(filepath.Join(rel, "storage", "logs"), 0o755)
	os.WriteFile(filepath.Join(rel, "storage", "logs", "a.log"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(rel, ".env"), []byte("FROM_REPO=1"), 0o644)

	shared := []string{"storage/", ".env", "public/uploads/", "config/local.php"}
	if err := (Op{Action: OpLink, Base: base, ID: 1, Shared: shared}).Run(io.Discard); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(base, "shared", "storage", "logs", "a.log")); string(b) != "x" {
		t.Fatal("storage was not moved into shared/")
	}
	if st, err := os.Stat(filepath.Join(base, "shared", "public", "uploads")); err != nil || !st.IsDir() {
		t.Fatal("public/uploads/ was not created as a folder")
	}
	if st, err := os.Stat(filepath.Join(base, "shared", "config", "local.php")); err != nil || st.IsDir() {
		t.Fatal("config/local.php was not created as a file")
	}

	// The next release's own .env gives way to the shared one.
	rel2 := Dir(base, 2)
	os.MkdirAll(rel2, 0o755)
	os.WriteFile(filepath.Join(rel2, ".env"), []byte("NEW=1"), 0o644)
	if err := (Op{Action: OpLink, Base: base, ID: 2, Shared: shared}).Run(io.Discard); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(rel2, ".env")); string(b) != "FROM_REPO=1" {
		t.Fatalf(".env in release 2 = %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(rel2, "storage", "logs", "a.log")); string(b) != "x" {
		t.Fatal("release 2 does not see shared storage")
	}
}
