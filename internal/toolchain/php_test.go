package toolchain

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/degovan/degoploy/internal/toolchain/toolchaintest"
)

func newPHP(t *testing.T, mirror *toolchaintest.Mirror, systemGlob string) *Manager {
	t.Helper()
	k := &PHP{ComposerURL: mirror.URL + "/composer.phar"}
	if systemGlob != "" {
		k.SystemGlobs = []string{systemGlob}
	}
	return New(k, filepath.Join(t.TempDir(), "php"), mirror.URL)
}

func TestPHPInstallAndComposer(t *testing.T) {
	ctx := context.Background()
	mirror := toolchaintest.NewPHPMirror(t, "8.4.23", "8.4.20!", "8.3.30", "8.2.32!")
	m := newPHP(t, mirror, "")

	if got, err := m.Latest(ctx, "^8.2"); err != nil || got != v("8.4.23") {
		t.Fatalf("Latest(^8.2) = %v %v", got, err)
	}
	if got, _ := m.Latest(ctx, "8.3"); got != v("8.3.30") {
		t.Fatalf("Latest(8.3) = %v", got)
	}
	if _, err := m.Latest(ctx, "lts"); err == nil {
		t.Fatal("PHP resolved lts")
	}

	var log strings.Builder
	if err := m.Install(ctx, v("8.3.30"), &log); err != nil {
		t.Fatalf("Install(8.3.30) = %v\n%s", err, log.String())
	}
	got, bin, err := m.Use(ctx, "8.3", &log)
	if err != nil || got != v("8.3.30") {
		t.Fatalf("Use(8.3) = %v %v\n%s", got, err, log.String())
	}
	for _, want := range []string{"checksum verified", "php 8.3.30 runs", "Composer installed"} {
		if !strings.Contains(log.String(), want) {
			t.Fatalf("log lacks %q:\n%s", want, log.String())
		}
	}
	if filepath.Base(filepath.Dir(bin)) != "8.3.30" {
		t.Fatalf("bin = %s", bin)
	}
	// composer runs the shared composer.phar with this version's php.
	out, err := exec.Command(filepath.Join(bin, "composer"), "install").Output()
	if err != nil || !strings.Contains(string(out), "php 8.3.30") || !strings.Contains(string(out), "composer.phar install") {
		t.Fatalf("composer: %q %v", out, err)
	}

	// The next use downloads nothing.
	n := mirror.Downloads.Load()
	if _, _, err := m.Use(ctx, "8.3", nil); err != nil || mirror.Downloads.Load() != n {
		t.Fatalf("second use: %v, downloads %d → %d", err, n, mirror.Downloads.Load())
	}

	// A build without a published checksum is still checked by running it.
	log.Reset()
	if err := m.Install(ctx, v("8.4.20"), &log); err != nil || !strings.Contains(log.String(), "no checksum for this build") {
		t.Fatalf("Install(8.4.20) = %v\n%s", err, log.String())
	}

	// Update moves 8.4.x within its line only.
	changes, err := m.Update(ctx, nil, nil)
	if err != nil || len(changes) != 1 || changes[0].From != v("8.4.20") || changes[0].To != v("8.4.23") {
		t.Fatalf("Update = %+v %v", changes, err)
	}
}

func TestPHPRejectsWrongVersion(t *testing.T) {
	mirror := toolchaintest.NewPHPMirror(t, "8.4.23")
	m := newPHP(t, mirror, "")
	// The archive for 8.4.23 claims to be another version.
	err := m.kind.Finish(context.Background(), m, writePHP(t, "8.4.1"), v("8.4.23"), nil)
	if err == nil || !strings.Contains(err.Error(), "reports version 8.4.1") {
		t.Fatalf("Finish = %v", err)
	}
}

// writePHP makes a directory holding a fake php binary of version.
func writePHP(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s' '" + version + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "php"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPHPSystemVersions(t *testing.T) {
	ctx := context.Background()
	mirror := toolchaintest.NewPHPMirror(t, "8.3.30", "8.2.32")
	sys := t.TempDir()
	// What ondrej's PPA installs: php8.2 and php8.3, plus php-fpm8.3 that is not a CLI.
	for name, version := range map[string]string{"php8.2": "8.2.7", "php8.3": "8.3.6"} {
		os.WriteFile(filepath.Join(sys, name), []byte("#!/bin/sh\nprintf '%s' '"+version+"-1ubuntu2'\n"), 0o755)
	}
	os.WriteFile(filepath.Join(sys, "php-fpm8.3"), []byte("#!/bin/sh\nexit 1\n"), 0o755)
	m := newPHP(t, mirror, filepath.Join(sys, "php*"))

	have, err := m.Installed()
	if err != nil || len(have) != 2 || !have[0].System || have[0].Version != v("8.3.6") {
		t.Fatalf("Installed = %+v %v", have, err)
	}
	// A system version that fits wins over a newer download.
	got, bin, err := m.Use(ctx, "8.3", nil)
	if err != nil || got != v("8.3.6") || mirror.Downloads.Load() != 0 {
		t.Fatalf("Use(8.3) = %v %v, %d downloads", got, err, mirror.Downloads.Load())
	}
	if target, _ := os.Readlink(filepath.Join(bin, "php")); target != filepath.Join(sys, "php8.3") {
		t.Fatalf("php links to %q", target)
	}
	if _, err := os.Stat(filepath.Join(bin, "composer")); err != nil {
		t.Fatal("no composer next to the system php")
	}
	if err := m.Remove(v("8.3.6")); err == nil || !strings.Contains(err.Error(), "comes with the system") {
		t.Fatalf("Remove(system) = %v", err)
	}
	// Update leaves system versions alone.
	if changes, err := m.Update(ctx, nil, nil); err != nil || len(changes) != 0 {
		t.Fatalf("Update = %+v %v", changes, err)
	}
}

func TestDetectPHP(t *testing.T) {
	var p PHP
	dir := t.TempDir()
	if s, _, uses := p.Detect(dir); s != "" || uses {
		t.Fatalf("empty dir: %q %v", s, uses)
	}
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"laravel/framework":"^11.0"}}`), 0o644)
	if s, _, uses := p.Detect(dir); s != "" || !uses {
		t.Fatalf("composer.json without php: %q %v", s, uses)
	}
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"php":"^8.2"}}`), 0o644)
	if s, src, _ := p.Detect(dir); s != "^8.2" || src != "composer.json require.php" {
		t.Fatalf("require.php: %q %q", s, src)
	}
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"php":"^8.2"},"config":{"platform":{"php":"8.3.6"}}}`), 0o644)
	if s, src, _ := p.Detect(dir); s != "8.3" || src != "composer.json config.platform.php" {
		t.Fatalf("platform: %q %q", s, src)
	}
	os.WriteFile(filepath.Join(dir, ".php-version"), []byte("8.4\n"), 0o644)
	if s, src, _ := p.Detect(dir); s != "8.4" || src != ".php-version" {
		t.Fatalf(".php-version: %q %q", s, src)
	}
}
