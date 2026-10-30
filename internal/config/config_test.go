package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectsDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "degoploy.yaml")
	if err := os.WriteFile(path, []byte(sample), 0o640); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_OIDC_SECRET", "x")

	if added, err := AddProjectsDir(path, "/srv/degoploy"); err != nil || !added {
		t.Fatalf("add: %v %v", added, err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "data_dir: ./data\n# Projects without") || !strings.Contains(string(raw), "projects_dir: /srv/degoploy\n") {
		t.Fatalf("projects_dir not below data_dir:\n%s", raw)
	}
	cfg, err := Load(path)
	if err != nil || cfg.ProjectsDir != "/srv/degoploy" {
		t.Fatalf("load: %q %v", cfg.ProjectsDir, err)
	}
	// A key that is already there, even empty, is the admin's choice.
	os.WriteFile(path, []byte("data_dir: ./data\nprojects_dir: \"\"\n"), 0o640)
	if added, err := AddProjectsDir(path, "/srv/degoploy"); err != nil || added {
		t.Fatalf("add over existing: %v %v", added, err)
	}

	for _, bad := range []string{"/", filepath.Join(dir, "data", "projects"), filepath.Dir(dir)} {
		os.WriteFile(path, []byte("data_dir: "+filepath.Join(dir, "data")+"\nprojects_dir: "+bad+"\n"), 0o640)
		if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "projects_dir") {
			t.Errorf("projects_dir %s: %v", bad, err)
		}
	}
}
