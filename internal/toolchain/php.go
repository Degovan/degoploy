package toolchain

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
)

// PHP is the PHP CLI with Composer. Versions the system installed (apt,
// ondrej's PPA: /usr/bin/php8.3) are used first, since they match the
// server's PHP-FPM and its extensions. Others are static builds from
// static-php.dev with a broad set of extensions, checked against the SHA256
// it publishes for recent builds, and always by running them.
type PHP struct {
	// SystemGlobs find PHP binaries the system installed.
	SystemGlobs []string
	// ComposerURL is where composer.phar comes from; its SHA256 is read from
	// ComposerURL + ".sha256sum".
	ComposerURL string

	composerMu sync.Mutex
}

// DefaultComposerURL is Composer's latest stable release.
const DefaultComposerURL = "https://getcomposer.org/download/latest-stable/composer.phar"

func (*PHP) Name() string          { return "php" }
func (*PHP) Title() string         { return "PHP" }
func (*PHP) DefaultMirror() string { return "https://dl.static-php.dev/v3/php-bin/bulk" }
func (*PHP) StripTop() bool        { return false }

// Line is the release line a version belongs to: PHP's are minor versions.
func (*PHP) Line(v Version) Range {
	return Range{{{">=", Version{v[0], v[1], 0}}, {"<", Version{v[0], v[1] + 1, 0}}}}
}

var phpRelease = regexp.MustCompile(`^php-(\d+\.\d+\.\d+)-cli-linux-([a-z0-9_]+)\.tar\.gz$`)

// Releases reads the mirror's file listing.
func (*PHP) Releases(ctx context.Context, m *Manager) ([]Release, error) {
	body, err := m.get(ctx, m.mirror+"/?format=json")
	if err != nil {
		return nil, err
	}
	var files []struct {
		Name         string `json:"name"`
		LastModified string `json:"last_modified"`
	}
	if err := json.Unmarshal(body, &files); err != nil {
		return nil, fmt.Errorf("read the file list at %s: %w", m.mirror, err)
	}
	var out []Release
	for _, f := range files {
		match := phpRelease.FindStringSubmatch(f.Name)
		if match == nil || match[2] != goarch() {
			continue
		}
		v, err := ParseVersion(match[1])
		if err != nil {
			continue
		}
		date, _, _ := strings.Cut(f.LastModified, " ")
		out = append(out, Release{Version: v, Date: date})
	}
	return out, nil
}

// Archive reads the build's .sha256 file; older builds have none.
func (*PHP) Archive(ctx context.Context, m *Manager, v Version) (string, string, error) {
	url := fmt.Sprintf("%s/php-%s-cli-linux-%s.tar.gz", m.mirror, m.Label(v), goarch())
	resp, err := m.open(ctx, url+".sha256")
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return url, "", nil
		}
		return "", "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", "", err
	}
	sum, _, _ := strings.Cut(strings.TrimSpace(string(b)), " ")
	if len(sum) != 64 {
		return "", "", fmt.Errorf("%s.sha256 holds no SHA256", url)
	}
	return url, strings.ToLower(sum), nil
}

// Finish moves the single php binary into bin/ and runs it, which is the
// check this source allows: it must start and report the expected version.
func (*PHP) Finish(ctx context.Context, m *Manager, root string, v Version, w io.Writer) error {
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(root, "php"), filepath.Join(bin, "php")); err != nil {
		return fmt.Errorf("the download has no php binary: %w", err)
	}
	got, err := phpVersion(ctx, filepath.Join(bin, "php"))
	if err != nil {
		return fmt.Errorf("the downloaded php does not run on this machine: %w", err)
	}
	if got != v {
		return fmt.Errorf("the downloaded php reports version %s, not %s", m.Label(got), m.Label(v))
	}
	fmt.Fprintf(w, "php %s runs\n", m.Label(got))
	return nil
}

// Prepare puts composer next to php: a small script that runs the shared
// composer.phar with this version's php, downloading Composer the first time.
func (p *PHP) Prepare(ctx context.Context, m *Manager, bin string, v Version, w io.Writer) error {
	phar, err := p.composer(ctx, m, w)
	if err != nil {
		return err
	}
	script := "#!/bin/sh\nexec \"$(dirname \"$0\")/php\" " + shellQuote(phar) + " \"$@\"\n"
	path := filepath.Join(bin, "composer")
	if b, err := os.ReadFile(path); err == nil && string(b) == script {
		return nil
	}
	return os.WriteFile(path, []byte(script), 0o755)
}

// composer downloads composer.phar once, checked against its published SHA256.
func (p *PHP) composer(ctx context.Context, m *Manager, w io.Writer) (string, error) {
	p.composerMu.Lock()
	defer p.composerMu.Unlock()
	dir := filepath.Join(m.dir, "composer")
	phar := filepath.Join(dir, "composer.phar")
	if _, err := os.Stat(phar); err == nil {
		return phar, nil
	}
	url := p.ComposerURL
	if url == "" {
		url = DefaultComposerURL
	}
	sums, err := m.get(ctx, url+".sha256sum")
	if err != nil {
		return "", err
	}
	want, _, _ := strings.Cut(strings.TrimSpace(string(sums)), " ")
	if len(want) != 64 {
		return "", fmt.Errorf("%s.sha256sum holds no SHA256", url)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp := phar + ".download"
	defer os.Remove(tmp)
	fmt.Fprintf(w, "downloading Composer from %s\n", url)
	if _, err := m.download(ctx, url, tmp, want, func(int64) {}, func(int64, int64) {}); err != nil {
		return "", err
	}
	fmt.Fprintf(w, "checksum verified, Composer installed\n")
	return phar, os.Rename(tmp, phar)
}

// Detect reads .php-version, then config.platform.php and require.php in
// composer.json. A composer.json alone means the project uses PHP without
// naming a version.
func (*PHP) Detect(dir string) (spec, source string, uses bool) {
	if b, err := os.ReadFile(filepath.Join(dir, ".php-version")); err == nil {
		if s := firstLine(string(b)); s != "" {
			return s, ".php-version", true
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	if err != nil {
		return "", "", false
	}
	var c struct {
		Require map[string]string `json:"require"`
		Config  struct {
			Platform map[string]string `json:"platform"`
		} `json:"config"`
	}
	if json.Unmarshal(b, &c) != nil {
		return "", "", true
	}
	if s := strings.TrimSpace(c.Config.Platform["php"]); s != "" {
		return s, "composer.json config.platform.php", true
	}
	if s := strings.TrimSpace(c.Require["php"]); s != "" {
		return s, "composer.json require.php", true
	}
	return "", "", true
}

var phpBinary = regexp.MustCompile(`^php(\d+(\.\d+)?)?$`)

func (p *PHP) System() []SystemVersion {
	var out []SystemVersion
	for _, g := range p.SystemGlobs {
		paths, _ := filepath.Glob(g)
		for _, path := range paths {
			if !phpBinary.MatchString(filepath.Base(path)) {
				continue
			}
			real, err := filepath.EvalSymlinks(path)
			if err != nil {
				continue
			}
			v, err := phpVersion(context.Background(), real)
			if err != nil || slices.ContainsFunc(out, func(s SystemVersion) bool { return s.Version == v }) {
				continue
			}
			out = append(out, SystemVersion{Version: v, Path: real})
		}
	}
	return out
}

// Env keeps Composer from asking questions in a script.
func (*PHP) Env(*Manager) []string { return []string{"COMPOSER_NO_INTERACTION=1"} }

// phpVersion runs a php binary and reads PHP_VERSION, without a distribution
// suffix such as "-1ubuntu2".
func phpVersion(ctx context.Context, bin string) (Version, error) {
	out, err := exec.CommandContext(ctx, bin, "-n", "-r", "echo PHP_VERSION;").Output()
	if err != nil {
		return Version{}, err
	}
	s := strings.TrimSpace(string(out))
	if i := strings.IndexFunc(s, func(r rune) bool { return (r < '0' || r > '9') && r != '.' }); i >= 0 {
		s = s[:i]
	}
	return ParseVersion(s)
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
