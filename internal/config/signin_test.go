package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `# degoploy settings
listen: "127.0.0.1:8090" # loopback only
data_dir: ./data
local_login: true

oidc:
  enabled: false
  provider_name: Casdoor
  issuer: https://sso.example.com
  client_id: degoploy
  client_secret: ${TEST_OIDC_SECRET}
  redirect_url: https://deploy.example.com/auth/oidc/callback
  # Groups that may sign in.
  role_mapping:
    "*": viewer
`

func TestSaveSignIn(t *testing.T) {
	t.Setenv("TEST_OIDC_SECRET", "from-env")
	path := filepath.Join(t.TempDir(), "degoploy.yaml")
	if err := os.WriteFile(path, []byte(sample), 0o640); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Locked["oidc.client_secret"] != "${TEST_OIDC_SECRET}" || cfg.OIDC.ClientSecret != "from-env" {
		t.Fatalf("locked = %v, secret = %q", cfg.Locked, cfg.OIDC.ClientSecret)
	}

	o := cfg.OIDC
	o.Enabled, o.ProviderName = true, "Degovan SSO"
	o.Scopes = []string{"openid", "profile", "email", "groups"}
	o.RoleMapping = map[string]string{"*": "viewer", "degovan/admins": "admin"}
	secret := "ignored-because-locked"
	if err := SaveSignIn(path, SignIn{LocalLogin: true, OIDC: o, ClientSecret: &secret}, cfg.Locked); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(path)
	text := string(raw)
	for _, want := range []string{"# degoploy settings", "# loopback only", "# Groups that may sign in.",
		"client_secret: ${TEST_OIDC_SECRET}", `listen: "127.0.0.1:8090"`, "provider_name: Degovan SSO",
		"scopes: [openid, profile, email, groups]"} {
		if !strings.Contains(text, want) {
			t.Errorf("saved file lacks %q:\n%s", want, text)
		}
	}
	if strings.Index(text, `"degovan/admins"`) > strings.Index(text, `"*"`) {
		t.Errorf("fallback should come last:\n%s", text)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v", st.Mode().Perm())
	}

	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !again.OIDC.Enabled || again.OIDC.RoleMapping["degovan/admins"] != "admin" || again.OIDC.ClientSecret != "from-env" {
		t.Fatalf("reloaded = %+v", again.OIDC)
	}
}

func TestEnvLocks(t *testing.T) {
	t.Setenv("DEGOPLOY_OIDC_ISSUER", "https://env.example.com")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Locked["oidc.issuer"] != "DEGOPLOY_OIDC_ISSUER" {
		t.Fatalf("locked = %v", cfg.Locked)
	}
	if err := SaveSignIn("", SignIn{LocalLogin: true}, nil); err == nil {
		t.Fatal("saving without a config file should fail")
	}
}
