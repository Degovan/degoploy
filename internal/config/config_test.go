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

	const comment = "Projects without a working directory of their own go here."
	if added, err := AddSetting(path, "projects_dir", "/srv/degoploy", comment); err != nil || !added {
		t.Fatalf("add: %v %v", added, err)
	}
	if added, err := AddSetting(path, "deploy_user", "degoploy-deploy", "Runs deployments."); err != nil || !added {
		t.Fatalf("add deploy_user: %v %v", added, err)
	}
	raw, _ := os.ReadFile(path)
	want := "data_dir: ./data\n# " + comment + "\nprojects_dir: /srv/degoploy\n# Runs deployments.\ndeploy_user: degoploy-deploy\n"
	if !strings.Contains(string(raw), want) {
		t.Fatalf("settings not in order below data_dir:\n%s", raw)
	}
	cfg, err := Load(path)
	if err != nil || cfg.ProjectsDir != "/srv/degoploy" || cfg.DeployUser != "degoploy-deploy" || cfg.RuntimesDir != cfg.DataDir {
		t.Fatalf("load: %+v %v", cfg, err)
	}
	// A key that is already there, even empty, is the admin's choice.
	os.WriteFile(path, []byte("data_dir: ./data\nprojects_dir: \"\"\n"), 0o640)
	if added, err := AddSetting(path, "projects_dir", "/srv/degoploy", ""); err != nil || added {
		t.Fatalf("add over existing: %v %v", added, err)
	}

	for _, bad := range []string{"/", filepath.Join(dir, "data", "projects"), filepath.Dir(dir)} {
		os.WriteFile(path, []byte("data_dir: "+filepath.Join(dir, "data")+"\nprojects_dir: "+bad+"\n"), 0o640)
		if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "projects_dir") {
			t.Errorf("projects_dir %s: %v", bad, err)
		}
	}
}
