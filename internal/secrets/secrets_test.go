package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSealOpen(t *testing.T) {
	key, err := LoadKey("", filepath.Join(t.TempDir(), "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	sealed := b.Seal("hunter2", "project/1")
	if strings.Contains(sealed, "hunter2") || !strings.HasPrefix(sealed, "v1:") {
		t.Fatalf("sealed = %q", sealed)
	}
	if b.Seal("hunter2", "project/1") == sealed {
		t.Fatal("two seals of the same value are identical; nonce not random")
	}
	if got, err := b.Open(sealed, "project/1"); err != nil || got != "hunter2" {
		t.Fatalf("Open = %q, %v", got, err)
	}
	if _, err := b.Open(sealed, "project/2"); err == nil {
		t.Fatal("opened a value sealed for another scope")
	}
	other, _ := New(make([]byte, keySize))
	if _, err := other.Open(sealed, "project/1"); err == nil {
		t.Fatal("opened a value with the wrong key")
	}
}

func TestLoadKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys", "secret.key")
	first, err := LoadKey("", path)
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode = %v, %v; want 0600", st.Mode().Perm(), err)
	}
	again, err := LoadKey("", path)
	if err != nil || string(again) != string(first) {
		t.Fatalf("second load returned a different key (%v)", err)
	}

	// A configured key wins over the file.
	encoded := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	if key, err := LoadKey(encoded, path); err != nil || len(key) != keySize || string(key) == string(first) {
		t.Fatalf("LoadKey(encoded) = %x, %v", key, err)
	}
	if _, err := LoadKey("c2hvcnQ=", path); err == nil {
		t.Fatal("accepted a short key")
	}
}
