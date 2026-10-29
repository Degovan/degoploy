package toolchain

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// NodeJS is Node.js, from nodejs.org's official builds, checked against each
// release's SHASUMS256.txt.
type NodeJS struct {
	// SystemPaths are checked for a Node.js the system installed.
	SystemPaths []string
}

func (NodeJS) Name() string          { return "node" }
func (NodeJS) Title() string         { return "Node.js" }
func (NodeJS) Prefix() string        { return "v" }
func (NodeJS) DefaultMirror() string { return "https://nodejs.org/dist" }
func (NodeJS) StripTop() bool        { return true }

// Platform is the build name for this machine, such as "linux-x64".
func (NodeJS) Platform() string {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x64"
	case "386":
		arch = "x86"
	}
	return runtime.GOOS + "-" + arch
}

func (n NodeJS) Releases(ctx context.Context, m *Manager) ([]Release, error) {
	body, err := m.get(ctx, m.mirror+"/index.json")
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Version string          `json:"version"`
		Date    string          `json:"date"`
		Files   []string        `json:"files"`
		LTS     json.RawMessage `json:"lts"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("read %s/index.json: %w", m.mirror, err)
	}
	var out []Release
	for _, r := range raw {
		v, err := ParseVersion(r.Version)
		if err != nil || !slices.Contains(r.Files, n.Platform()) {
			continue
		}
		var lts string
		json.Unmarshal(r.LTS, &lts) // false for current releases
		out = append(out, Release{Version: v, LTS: lts, Date: r.Date})
	}
	return out, nil
}

func (n NodeJS) Archive(ctx context.Context, m *Manager, v Version) (string, string, error) {
	name := "node-" + v.String() + "-" + n.Platform() + ".tar.gz"
	base := m.mirror + "/" + v.String()
	sums, err := m.get(ctx, base+"/SHASUMS256.txt")
	if err != nil {
		return "", "", err
	}
	for _, line := range strings.Split(string(sums), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[1] == name {
			return base + "/" + name, f[0], nil
		}
	}
	return "", "", fmt.Errorf("%s is not listed in %s/SHASUMS256.txt", name, base)
}

// Finish turns on corepack, so pnpm and yarn follow the packageManager field
// of package.json. Node.js 25 and later no longer ship it; it comes from npm.
func (NodeJS) Finish(ctx context.Context, m *Manager, root string, v Version, w io.Writer) error {
	enableCorepack(ctx, filepath.Join(root, "bin"), "", w)
	return nil
}

// Prepare gives a system Node.js the same pnpm and yarn shims, in degoploy's
// own directory, since the system's bin directory belongs to root.
func (NodeJS) Prepare(ctx context.Context, m *Manager, bin string, v Version, w io.Writer) error {
	if _, err := os.Lstat(filepath.Join(bin, "pnpm")); err == nil {
		return nil
	}
	if target, err := os.Readlink(filepath.Join(bin, "node")); err == nil {
		enableCorepack(ctx, filepath.Dir(target), bin, w)
	}
	return nil
}

// enableCorepack runs corepack from bin, installing it with npm when missing,
// and puts the shims in into (bin when empty).
func enableCorepack(ctx context.Context, bin, into string, w io.Writer) {
	env := append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	run := func(name string, args ...string) error {
		cmd := exec.CommandContext(ctx, filepath.Join(bin, name), args...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	if _, err := os.Stat(filepath.Join(bin, "corepack")); err != nil {
		if into != "" {
			return // a system Node.js without corepack: not ours to change
		}
		fmt.Fprintf(w, "installing corepack from npm, for pnpm and yarn\n")
		if err := run("npm", "install", "--global", "--no-fund", "--no-audit", "corepack"); err != nil {
			fmt.Fprintf(w, "corepack could not be installed, so pnpm and yarn are not set up: %s\n", err)
			return
		}
	}
	args := []string{"enable"}
	if into != "" {
		args = append(args, "--install-directory", into)
	}
	if err := run("corepack", args...); err != nil {
		fmt.Fprintf(w, "corepack enable failed, so pnpm and yarn are not set up: %s\n", err)
	}
}

// Detect reads .nvmrc, .node-version, then volta.node and engines.node in
// package.json, as nvm and similar tools do. A package.json alone means the
// project uses Node.js without naming a version.
func (NodeJS) Detect(dir string) (spec, source string, uses bool) {
	for _, name := range []string{".nvmrc", ".node-version"} {
		if b, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			if s := firstLine(string(b)); s != "" {
				return s, name, true
			}
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return "", "", false
	}
	var pkg struct {
		Volta struct {
			Node string `json:"node"`
		} `json:"volta"`
		Engines struct {
			Node string `json:"node"`
		} `json:"engines"`
	}
	if json.Unmarshal(b, &pkg) != nil {
		return "", "", true
	}
	if s := strings.TrimSpace(pkg.Volta.Node); s != "" {
		return s, "package.json volta.node", true
	}
	if s := strings.TrimSpace(pkg.Engines.Node); s != "" {
		return s, "package.json engines.node", true
	}
	return "", "", true
}

func (n NodeJS) System() []SystemVersion {
	var out []SystemVersion
	for _, p := range n.SystemPaths {
		real, err := filepath.EvalSymlinks(p)
		if err != nil {
			continue
		}
		b, err := exec.Command(real, "--version").Output()
		if err != nil {
			continue
		}
		v, err := ParseVersion(strings.TrimSpace(string(b)))
		if err != nil || slices.ContainsFunc(out, func(s SystemVersion) bool { return s.Version == v }) {
			continue
		}
		out = append(out, SystemVersion{Version: v, Path: real})
	}
	return out
}

// Env keeps corepack from asking before it downloads pnpm or yarn.
func (NodeJS) Env(*Manager) []string { return []string{"COREPACK_ENABLE_DOWNLOAD_PROMPT=0"} }
