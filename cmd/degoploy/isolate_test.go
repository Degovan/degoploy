package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSudoRule(t *testing.T) {
	visudo, err := exec.LookPath("visudo")
	if err != nil {
		t.Skip("visudo not installed")
	}
	path := filepath.Join(t.TempDir(), "degoploy")
	os.WriteFile(path, []byte(sudoRule(deployUser)), 0o440)
	if out, err := exec.Command(visudo, "-cqf", path).CombinedOutput(); err != nil {
		t.Fatalf("visudo: %v\n%s\n%s", err, out, sudoRule(deployUser))
	}
}

func TestSudoLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules")
	os.WriteFile(path, []byte("# comment\ndegoploy ALL=(root) NOPASSWD: /usr/bin/systemctl restart app, \\\n    /usr/bin/systemctl reload nginx\n\n%admin ALL=(ALL) ALL\n"), 0o440)
	got := sudoLines(path)
	want := []string{"degoploy ALL=(root) NOPASSWD: /usr/bin/systemctl restart app,  /usr/bin/systemctl reload nginx", "%admin ALL=(ALL) ALL"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("lines = %q", got)
	}
}

func TestChownTreeCounts(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755)
	os.WriteFile(filepath.Join(dir, "a", "f"), nil, 0o644)
	os.Symlink("/etc/passwd", filepath.Join(dir, "link"))
	// Giving our own files to ourselves changes nothing but counts them all.
	n, err := chownTree(dir, os.Getuid(), os.Getuid())
	if err != nil || n != 5 {
		t.Fatalf("n = %d, %v", n, err)
	}
	if n, _ := chownTree(dir, os.Getuid()+12345, os.Getuid()); n != 0 {
		t.Fatalf("other owner: n = %d", n)
	}
}
