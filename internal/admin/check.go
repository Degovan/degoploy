package admin

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/degovan/degoploy/internal/auth"
	"github.com/degovan/degoploy/internal/config"
)

// Report is the outcome of checking a configuration file.
type Report struct {
	Config   string      `json:"config"`
	Values   [][2]string `json:"values"`
	Warnings []string    `json:"warnings"`
	// Error is set when the file does not load; Values and Warnings are empty then.
	Error string `json:"error,omitempty"`
}

// Check loads the configuration at path (empty: defaults and environment only)
// and, with SSO enabled, tries the provider's discovery.
func Check(ctx context.Context, path string) Report {
	rep := Report{Config: path, Warnings: []string{}}
	if path == "" {
		rep.Config = "(defaults + environment)"
	}
	cfg, err := config.Load(path)
	if err != nil {
		rep.Error = err.Error()
		return rep
	}
	set := func(k, v string) { rep.Values = append(rep.Values, [2]string{k, v}) }
	warn := func(format string, a ...any) { rep.Warnings = append(rep.Warnings, fmt.Sprintf(format, a...)) }

	set("listen", cfg.Listen)
	set("data_dir", cfg.DataDir)
	set("projects_dir", orNone(cfg.ProjectsDir))
	set("base_url", orNone(cfg.BaseURL))
	set("admin_socket", cfg.AdminSocket)
	set("local_login", strconv.FormatBool(cfg.LocalLogin))
	set("max_parallel", strconv.Itoa(cfg.MaxParallel))

	if cfg.BaseURL == "" {
		warn("base_url is empty: webhook URLs are derived from the browser address and cookies are not marked Secure")
	} else if !cfg.SecureCookies() {
		warn("base_url is not https: session cookies are not marked Secure")
	}
	if st, err := os.Stat(cfg.DataDir); err == nil && st.Mode().Perm()&0o077 != 0 {
		warn("data_dir permissions are %v; deploy keys live there, use 0700", st.Mode().Perm())
	}

	if !cfg.OIDC.Enabled {
		set("oidc", "disabled")
		return rep
	}
	set("oidc", fmt.Sprintf("%s (%s)", cfg.OIDC.ProviderName, cfg.OIDC.Issuer))
	if cfg.OIDC.ClientSecret == "" {
		warn("oidc.client_secret is empty (is the environment variable set?)")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	// AuthURL performs discovery; the state values are throwaway.
	if _, err := auth.NewOIDCProvider(cfg.OIDC).AuthURL(ctx, "check", "check", "check-verifier-check-verifier-check-verifier"); err != nil {
		warn("oidc discovery failed: %v", err)
	} else {
		set("oidc_discovery", "ok")
	}
	return rep
}

func orNone(s string) string {
	if s == "" {
		return "(not set)"
	}
	return s
}
