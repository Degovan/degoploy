// Package toolchain installs language runtimes (Node.js, PHP) for deploy
// scripts and picks the version a project asks for, the way nvm does but
// without a shell to source. Builds are downloaded on first use, checked
// where their source publishes checksums, and unpacked under
// <data_dir>/<runtime>/<version>; versions the operating system installed are
// used as they are. The runner puts the chosen version's bin directory first
// on PATH.
package toolchain

import (
	"context"
	"errors"
	"io"
	"runtime"
)

// Project settings that are not a version.
const (
	// Auto uses the runtime when the repository does (.nvmrc, composer.json,
	// …): the version it names, else the default version.
	Auto = "auto"
	// Off leaves the runtime out; scripts get whatever the server has.
	Off = "off"
)

// Kind is one runtime: where its releases come from and how they are laid out.
type Kind interface {
	// Name is the runtime's id in URLs, settings and directory names.
	Name() string
	// Title is its display name.
	Title() string
	DefaultMirror() string
	// Releases lists what can be downloaded for this machine.
	Releases(ctx context.Context, m *Manager) ([]Release, error)
	// Archive is where a release is downloaded from and its SHA256, or ""
	// when the source publishes none.
	Archive(ctx context.Context, m *Manager, v Version) (url, sum string, err error)
	// StripTop reports whether the archive keeps everything in one top directory.
	StripTop() bool
	// Finish turns an unpacked archive into a version directory with a bin/
	// directory and adds what scripts expect next to it.
	Finish(ctx context.Context, m *Manager, root string, v Version, w io.Writer) error
	// Prepare readies a version for a deployment, such as tools shared by all
	// versions; it runs before each use.
	Prepare(ctx context.Context, m *Manager, bin string, v Version, w io.Writer) error
	// Detect reads the version a repository asks for. uses reports that the
	// repository uses the runtime even without naming a version.
	Detect(dir string) (spec, source string, uses bool)
	// System lists versions the operating system installed.
	System() []SystemVersion
	// Env is added to the environment of scripts that use the runtime.
	Env(m *Manager) []string
}

// Release is a version that can be downloaded.
type Release struct {
	Version Version
	LTS     string // Node.js LTS codename, such as "Jod"; empty otherwise
	Date    string
}

// SystemVersion is a runtime the operating system installed, such as
// /usr/bin/php8.3 from apt.
type SystemVersion struct {
	Version Version
	Path    string // the executable
}

// ErrCanceled is returned by an install that Cancel stopped.
var ErrCanceled = errors.New("canceled")

// goarch names this machine's architecture the way most release files do.
func goarch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	}
	return runtime.GOARCH
}
