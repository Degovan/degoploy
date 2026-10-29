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
	// AdminSocket is the Unix socket the CLI uses to reach the running service.
	// Empty means $RUNTIME_DIRECTORY/admin.sock under systemd, else
	// <data_dir>/admin.sock; "off" disables it.
	AdminSocket string `yaml:"admin_socket"`
	OIDC        OIDC   `yaml:"oidc"`
	SMTP        SMTP   `yaml:"smtp"`

	// Parsed from TrustedProxies by Load.
	TrustedPrefixes []netip.Prefix `yaml:"-"`
	// Path is the YAML file Load read, where SaveSignIn writes; empty without one.
	Path string `yaml:"-"`
	// Locked maps sign-in settings that come from the environment (a DEGOPLOY_*
	// variable or a ${VAR} reference in the file) to that source. They cannot be
	// changed from the UI.
	Locked map[string]string `yaml:"-"`
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

// SMTP is the outgoing mail server for invitation emails.
type SMTP struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	// Encryption is "starttls", "tls" (implicit, usually port 465) or "none".
	Encryption  string `yaml:"encryption"`
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
	FromName    string `yaml:"from_name"`
	FromAddress string `yaml:"from_address"`
}

// Configured reports whether enough is set to try sending mail.
func (s SMTP) Configured() bool { return s.Host != "" && s.Port > 0 && s.FromAddress != "" }

// ValidateSMTP checks the SMTP settings. Empty host means mail is off.
func ValidateSMTP(s SMTP) error {
	if s.Host == "" {
		return nil
	}
	var errs []error
	if s.Port < 1 || s.Port > 65535 {
		errs = append(errs, errors.New("smtp: port must be between 1 and 65535"))
	}
	switch s.Encryption {
	case "starttls", "tls", "none":
	default:
		errs = append(errs, fmt.Errorf("smtp: encryption %q; use starttls, tls or none", s.Encryption))
	}
	if !strings.Contains(s.FromAddress, "@") {
		errs = append(errs, errors.New("smtp: from_address must be an email address"))
	}
	return errors.Join(errs...)
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
		SMTP: SMTP{Port: 587, Encryption: "starttls", FromName: "degoploy"},
	}
}

// Load reads path (optional; empty means defaults + env only) and applies env overrides.
func Load(path string) (Config, error) {
	cfg := Default()
	cfg.Path = path
	cfg.Locked = map[string]string{}

	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read config: %w", err)
		}
		if err := yaml.Unmarshal(expandEnv(raw), &cfg); err != nil {
			return cfg, fmt.Errorf("parse config %s: %w", path, err)
		}
		lockEnvRefs(raw, cfg.Locked)
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
		"DEGOPLOY_ADMIN_SOCKET":       &cfg.AdminSocket,
		"DEGOPLOY_OIDC_ISSUER":        &cfg.OIDC.Issuer,
		"DEGOPLOY_OIDC_CLIENT_ID":     &cfg.OIDC.ClientID,
		"DEGOPLOY_OIDC_CLIENT_SECRET": &cfg.OIDC.ClientSecret,
		"DEGOPLOY_OIDC_REDIRECT_URL":  &cfg.OIDC.RedirectURL,
		"DEGOPLOY_SMTP_HOST":          &cfg.SMTP.Host,
		"DEGOPLOY_SMTP_USERNAME":      &cfg.SMTP.Username,
		"DEGOPLOY_SMTP_PASSWORD":      &cfg.SMTP.Password,
		"DEGOPLOY_SMTP_FROM_ADDRESS":  &cfg.SMTP.FromAddress,
	}
	for k, p := range str {
		if v, ok := os.LookupEnv(k); ok {
			*p = v
			lockEnv(cfg, k)
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
			lockEnv(cfg, k)
		}
	}

	if v, ok := os.LookupEnv("DEGOPLOY_MAX_PARALLEL"); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("DEGOPLOY_MAX_PARALLEL: %w", err)
		}
		cfg.MaxParallel = n
	}
	if v, ok := os.LookupEnv("DEGOPLOY_SMTP_PORT"); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("DEGOPLOY_SMTP_PORT: %w", err)
		}
		cfg.SMTP.Port = n
		lockEnv(cfg, "DEGOPLOY_SMTP_PORT")
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
	if c.AdminSocket == "" && c.DataDir != "" {
		dir := c.DataDir
		if rt, _, _ := strings.Cut(os.Getenv("RUNTIME_DIRECTORY"), ":"); rt != "" {
			dir = rt
		}
		c.AdminSocket = filepath.Join(dir, "admin.sock")
	}

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

	if c.OIDC.Enabled && c.OIDC.GroupsClaim == "" {
		c.OIDC.GroupsClaim = "groups"
	}
	if err := ValidateSignIn(c.LocalLogin, c.OIDC); err != nil {
		errs = append(errs, err)
	}
	if err := ValidateSMTP(c.SMTP); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// ValidateSignIn checks the sign-in settings: local_login and the oidc block.
func ValidateSignIn(localLogin bool, o OIDC) error {
	var errs []error
	if o.Enabled {
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
	}
	if !localLogin && !o.Enabled {
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
