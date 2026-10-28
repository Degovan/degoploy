package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"go.yaml.in/yaml/v3"
)

// SealedPrefix marks a client secret that the server encrypted with its secret
// key before writing it to the file (see SaveSignIn). The value after the
// prefix is opened with secrets.Box and OIDCSecretScope.
const SealedPrefix = "sealed:"

// OIDCSecretScope binds a sealed client secret to its place in the config.
const OIDCSecretScope = "config/oidc.client_secret"

// envKeys maps DEGOPLOY_* variables to the sign-in setting they override.
var envKeys = map[string]string{
	"DEGOPLOY_LOCAL_LOGIN":        "local_login",
	"DEGOPLOY_OIDC_ENABLED":       "oidc.enabled",
	"DEGOPLOY_OIDC_ISSUER":        "oidc.issuer",
	"DEGOPLOY_OIDC_CLIENT_ID":     "oidc.client_id",
	"DEGOPLOY_OIDC_CLIENT_SECRET": "oidc.client_secret",
	"DEGOPLOY_OIDC_REDIRECT_URL":  "oidc.redirect_url",
}

func lockEnv(cfg *Config, envName string) {
	if key, ok := envKeys[envName]; ok && cfg.Locked != nil {
		cfg.Locked[key] = envName
	}
}

// lockEnvRefs records sign-in settings whose value in the file is a ${VAR}
// reference: saving from the UI would replace the reference with its value.
func lockEnvRefs(raw []byte, locked map[string]string) {
	var doc yaml.Node
	if yaml.Unmarshal(raw, &doc) != nil || len(doc.Content) == 0 {
		return
	}
	root := doc.Content[0]
	check := func(key string, n *yaml.Node) {
		if n != nil && n.Kind == yaml.ScalarNode {
			if m := envRef.FindString(n.Value); m != "" {
				locked[key] = m
			}
		}
	}
	check("local_login", mapValue(root, "local_login"))
	if o := mapValue(root, "oidc"); o != nil && o.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(o.Content); i += 2 {
			check("oidc."+o.Content[i].Value, o.Content[i+1])
		}
	}
}

// SignIn is what SaveSignIn writes. ClientSecret nil keeps the file's value.
type SignIn struct {
	LocalLogin   bool
	OIDC         OIDC
	ClientSecret *string
}

// SaveSignIn writes local_login and the oidc block into the YAML file at path,
// keeping comments, the other settings, and every key in locked as it is. The
// file is replaced atomically and keeps its permissions.
func SaveSignIn(path string, s SignIn, locked map[string]string) error {
	if path == "" {
		return errors.New("no config file: start degoploy with --config to save settings")
	}
	raw, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read config: %w", err)
	}
	var doc yaml.Node
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return fmt.Errorf("parse config %s: %w", path, err)
		}
	}
	if len(doc.Content) == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("config %s: top level is not a mapping", path)
	}

	set := func(m *yaml.Node, prefix, key string, value *yaml.Node) {
		if _, ok := locked[prefix+key]; !ok {
			setMapValue(m, key, value)
		}
	}
	set(root, "", "local_login", boolNode(s.LocalLogin))

	o := mapValue(root, "oidc")
	if o == nil || o.Kind != yaml.MappingNode {
		o = &yaml.Node{Kind: yaml.MappingNode}
		setMapValue(root, "oidc", o)
	}
	set(o, "oidc.", "enabled", boolNode(s.OIDC.Enabled))
	set(o, "oidc.", "provider_name", strNode(s.OIDC.ProviderName))
	set(o, "oidc.", "issuer", strNode(s.OIDC.Issuer))
	set(o, "oidc.", "client_id", strNode(s.OIDC.ClientID))
	if s.ClientSecret != nil {
		set(o, "oidc.", "client_secret", strNode(*s.ClientSecret))
	}
	set(o, "oidc.", "redirect_url", strNode(s.OIDC.RedirectURL))
	scopes := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, sc := range s.OIDC.Scopes {
		scopes.Content = append(scopes.Content, strNode(sc))
	}
	set(o, "oidc.", "scopes", scopes)
	set(o, "oidc.", "groups_claim", strNode(s.OIDC.GroupsClaim))
	mapping := &yaml.Node{Kind: yaml.MappingNode}
	groups := make([]string, 0, len(s.OIDC.RoleMapping))
	for g := range s.OIDC.RoleMapping {
		groups = append(groups, g)
	}
	// Listed groups first, the "*" fallback last.
	sort.Slice(groups, func(i, j int) bool {
		if (groups[i] == "*") != (groups[j] == "*") {
			return groups[j] == "*"
		}
		return groups[i] < groups[j]
	})
	for _, g := range groups {
		key := strNode(g)
		key.Style = yaml.DoubleQuotedStyle
		mapping.Content = append(mapping.Content, key, strNode(s.OIDC.RoleMapping[g]))
	}
	set(o, "oidc.", "role_mapping", mapping)
	set(o, "oidc.", "sync_role_on_login", boolNode(s.OIDC.SyncRoleOnLogin))

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	enc.Close()
	return writeAtomic(path, buf.Bytes())
}

func writeAtomic(path string, data []byte) error {
	mode := fs.FileMode(0o600)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".degoploy-config-*")
	if err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write config: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return fmt.Errorf("write config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("write config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// setMapValue replaces key's value in place (keeping its comments) or appends it.
func setMapValue(m *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			old := m.Content[i+1]
			value.LineComment, value.HeadComment, value.FootComment = old.LineComment, old.HeadComment, old.FootComment
			m.Content[i+1] = value
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}

func strNode(s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s} }

func boolNode(b bool) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(b)}
}
