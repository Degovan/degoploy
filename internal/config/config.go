// Package config loads degoploy settings from a YAML file and DEGOPLOY_* environment variables.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

type Config struct {
	Listen         string        `yaml:"listen"`
	DataDir        string        `yaml:"data_dir"`
	BaseURL        string        `yaml:"base_url"`
	MaxParallel    int           `yaml:"max_parallel"`
	TrustedProxies []string      `yaml:"trusted_proxies"`
	LocalLogin     bool          `yaml:"local_login"`
	SessionTTL     time.Duration `yaml:"session_ttl"`
	// SecretKey is the base64 AES-256 key that encrypts project secrets. Empty
	// means a key file in the data dir, created on first start.
	SecretKey string `yaml:"secret_key"`
	OIDC      OIDC   `yaml:"oidc"`

	// Parsed from TrustedProxies by Load.
	TrustedPrefixes []netip.Prefix `yaml:"-"`
}

type OIDC struct {
	Enabled         bool              `yaml:"enabled"`
	ProviderName    string            `yaml:"provider_name"`
	Issuer          string            `yaml:"issuer"`
	ClientID        string            `yaml:"client_id"`
	ClientSecret    string            `yaml:"client_secret"`
	RedirectURL     string            `yaml:"redirect_url"`
	Scopes          []string          `yaml:"scopes"`
	GroupsClaim     string            `yaml:"groups_claim"`
	RoleMapping     map[string]string `yaml:"role_mapping"`
	SyncRoleOnLogin bool              `yaml:"sync_role_on_login"`
}

func Default() Config {
	return Config{
		Listen:         ":8080",
		DataDir:        "./data",
		MaxParallel:    2,
		TrustedProxies: []string{"127.0.0.1/32", "::1/128"},
		LocalLogin:     true,
		SessionTTL:     7 * 24 * time.Hour,
		OIDC: OIDC{
			ProviderName:    "SSO",
			Scopes:          []string{"openid", "profile", "email"},
			GroupsClaim:     "groups",
			SyncRoleOnLogin: true,
		},
	}
}

// Load reads path (optional; empty means defaults + env only) and applies env overrides.
func Load(path string) (Config, error) {
	cfg := Default()

	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read config: %w", err)
		}
		if err := yaml.Unmarshal(expandEnv(raw), &cfg); err != nil {
			return cfg, fmt.Errorf("parse config %s: %w", path, err)
		}
	}

	if err := applyEnv(&cfg); err != nil {
		return cfg, err
	}
	if err := cfg.finalize(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

var envRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// expandEnv replaces only ${VAR} references so literal '$' in values stays intact.
func expandEnv(raw []byte) []byte {
	return envRef.ReplaceAllFunc(raw, func(m []byte) []byte {
		return []byte(os.Getenv(string(m[2 : len(m)-1])))
	})
}

func applyEnv(cfg *Config) error {
	str := map[string]*string{
		"DEGOPLOY_LISTEN":             &cfg.Listen,
		"DEGOPLOY_DATA_DIR":           &cfg.DataDir,
		"DEGOPLOY_BASE_URL":           &cfg.BaseURL,
		"DEGOPLOY_SECRET_KEY":         &cfg.SecretKey,
		"DEGOPLOY_OIDC_ISSUER":        &cfg.OIDC.Issuer,
		"DEGOPLOY_OIDC_CLIENT_ID":     &cfg.OIDC.ClientID,
		"DEGOPLOY_OIDC_CLIENT_SECRET": &cfg.OIDC.ClientSecret,
		"DEGOPLOY_OIDC_REDIRECT_URL":  &cfg.OIDC.RedirectURL,
	}
	for k, p := range str {
		if v, ok := os.LookupEnv(k); ok {
			*p = v
		}
	}

	bools := map[string]*bool{
		"DEGOPLOY_LOCAL_LOGIN":  &cfg.LocalLogin,
		"DEGOPLOY_OIDC_ENABLED": &cfg.OIDC.Enabled,
	}
	for k, p := range bools {
		if v, ok := os.LookupEnv(k); ok {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return fmt.Errorf("%s: %w", k, err)
			}
			*p = b
		}
	}

	if v, ok := os.LookupEnv("DEGOPLOY_MAX_PARALLEL"); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("DEGOPLOY_MAX_PARALLEL: %w", err)
		}
		cfg.MaxParallel = n
	}
	if v, ok := os.LookupEnv("DEGOPLOY_TRUSTED_PROXIES"); ok {
		cfg.TrustedProxies = splitList(v)
	}
	return nil
}

func (c *Config) finalize() error {
	var errs []error

	if c.DataDir == "" {
		errs = append(errs, errors.New("data_dir is required"))
	} else if abs, err := filepath.Abs(c.DataDir); err == nil {
		c.DataDir = abs
	}
	if c.MaxParallel < 1 {
		errs = append(errs, errors.New("max_parallel must be >= 1"))
	}
	if c.SessionTTL < time.Minute {
		errs = append(errs, errors.New("session_ttl must be >= 1m"))
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")

	c.TrustedPrefixes = c.TrustedPrefixes[:0]
	for _, s := range c.TrustedProxies {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			addr, aerr := netip.ParseAddr(s)
			if aerr != nil {
				errs = append(errs, fmt.Errorf("trusted_proxies: invalid %q", s))
				continue
			}
			p = netip.PrefixFrom(addr, addr.BitLen())
		}
		c.TrustedPrefixes = append(c.TrustedPrefixes, p.Masked())
	}

	if c.OIDC.Enabled {
		o := c.OIDC
		if o.Issuer == "" || o.ClientID == "" || o.RedirectURL == "" {
			errs = append(errs, errors.New("oidc: issuer, client_id and redirect_url are required when enabled"))
		}
		if len(o.RoleMapping) == 0 {
			errs = append(errs, errors.New("oidc: role_mapping is empty, so nobody could sign in with SSO"))
		}
		for group, role := range o.RoleMapping {
			switch role {
			case "admin", "deployer", "viewer":
			default:
				errs = append(errs, fmt.Errorf("oidc: role_mapping[%q] = %q; use admin, deployer or viewer", group, role))
			}
		}
		if c.OIDC.GroupsClaim == "" {
			c.OIDC.GroupsClaim = "groups"
		}
	}
	if !c.LocalLogin && !c.OIDC.Enabled {
		errs = append(errs, errors.New("at least one of local_login or oidc must be enabled"))
	}
	return errors.Join(errs...)
}

// SecureCookies reports whether cookies should carry the Secure flag.
func (c *Config) SecureCookies() bool {
	return strings.HasPrefix(c.BaseURL, "https://")
}

func (c *Config) DBPath() string { return filepath.Join(c.DataDir, "degoploy.db") }

// SecretKeyPath is the key file used when SecretKey is not set.
func (c *Config) SecretKeyPath() string { return filepath.Join(c.DataDir, "secret.key") }

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
