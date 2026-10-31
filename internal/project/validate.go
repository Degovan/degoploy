package project

import (
	"cmp"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/degovan/degoploy/internal/release"
	"github.com/degovan/degoploy/internal/store"
	"github.com/degovan/degoploy/internal/toolchain"
)

// ValidationError points at the input field that was rejected.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, format string, args ...any) error {
	return &ValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
}

// Input is the full set of user-editable project fields.
type Input struct {
	Name         string
	Slug         string // only used on create
	Provider     string
	RepoURL      string
	WorkDir      string
	ScriptSource string
	ScriptPath   string
	AutoPull     bool
	TimeoutSec   int
	Enabled      bool
	// Releases deploys each run into a release folder of its own (package release).
	Releases      bool
	KeepReleases  int
	SharedPaths   []string
	AfterActivate string
	// Runtimes holds runtime settings to change, by runtime name:
	// toolchain.Auto, toolchain.Off or a version spec. Others keep theirs.
	Runtimes map[string]string
	// DefaultBranch (create only) adds a first rule deploying pushes to this branch.
	DefaultBranch string
}

var (
	slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	// user@host:path (scp-like) — the common GitHub/Gitea SSH form.
	scpRe = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9.-]+:[^\s:][^\s]*$`)
	// ssh://, https:// or http:// URLs.
	urlRe = regexp.MustCompile(`^(ssh|https?)://[^\s/]+/[^\s]+$`)
)

const (
	minTimeout = 10
	maxTimeout = 24 * 60 * 60

	maxKeepReleases  = 50
	maxSharedPaths   = 50
	maxAfterActivate = 64 << 10
)

func (in *Input) normalize() {
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.TrimSpace(strings.ToLower(in.Slug))
	in.Provider = strings.TrimSpace(strings.ToLower(in.Provider))
	in.RepoURL = strings.TrimSpace(in.RepoURL)
	in.WorkDir = strings.TrimSpace(in.WorkDir)
	in.ScriptSource = strings.TrimSpace(in.ScriptSource)
	in.ScriptPath = strings.TrimSpace(in.ScriptPath)
	for name, spec := range in.Runtimes {
		spec = strings.TrimSpace(spec)
		switch low := strings.ToLower(spec); low {
		case "", toolchain.Auto:
			spec = toolchain.Auto
		case toolchain.Off, "none":
			spec = toolchain.Off
		}
		in.Runtimes[name] = spec
	}
	if in.WorkDir != "" {
		in.WorkDir = filepath.Clean(in.WorkDir)
	}
	if in.KeepReleases == 0 {
		in.KeepReleases = store.DefaultKeepReleases
	}
	var shared []string
	for _, p := range in.SharedPaths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if c := release.CleanShared(p); c == "" || !slices.Contains(shared, c) {
			shared = append(shared, cmp.Or(c, p))
		}
	}
	in.SharedPaths = shared
	in.AfterActivate = strings.ReplaceAll(in.AfterActivate, "\r\n", "\n")
	if strings.TrimSpace(in.AfterActivate) == "" {
		in.AfterActivate = ""
	}
	if in.ScriptSource == store.ScriptManaged {
		in.ScriptPath = ""
	} else if in.ScriptPath != "" {
		in.ScriptPath = filepath.ToSlash(filepath.Clean(in.ScriptPath))
	}
}

// validate checks a normalized Input. dataDir is degoploy's own data directory
// and projectsDir the folder that holds projects' working directories.
func (in *Input) validate(dataDir, projectsDir string, creating bool) error {
	if in.Name == "" || len(in.Name) > 100 {
		return invalid("name", "must be 1–100 characters")
	}
	if creating && !slugRe.MatchString(in.Slug) {
		return invalid("slug", "use 1–63 lowercase letters, digits or dashes, starting with a letter or digit")
	}
	if in.Provider != "github" && in.Provider != "gitea" {
		return invalid("provider", "must be github or gitea")
	}
	if err := validateRepoURL(in.RepoURL); err != nil {
		return err
	}
	if err := validateWorkDir(in.WorkDir, dataDir, projectsDir); err != nil {
		return err
	}

	switch in.ScriptSource {
	case store.ScriptManaged:
	case store.ScriptRepo:
		if !in.AutoPull {
			return invalid("auto_pull", "must be enabled when the script lives in the repository")
		}
		p := in.ScriptPath
		if p == "" || p == "." || filepath.IsAbs(p) || p == ".." || strings.HasPrefix(p, "../") {
			return invalid("script_path", "must be a path inside the repository, e.g. .degoploy/deploy.sh")
		}
	default:
		return invalid("script_source", "must be managed or repo")
	}

	if in.Releases && !in.AutoPull {
		return invalid("auto_pull", "must be enabled for releases: each release is a fresh checkout")
	}
	if in.KeepReleases < 1 || in.KeepReleases > maxKeepReleases {
		return invalid("keep_releases", "must be between 1 and %d", maxKeepReleases)
	}
	if len(in.SharedPaths) > maxSharedPaths {
		return invalid("shared_paths", "at most %d paths", maxSharedPaths)
	}
	for _, p := range in.SharedPaths {
		if release.CleanShared(p) != p {
			return invalid("shared_paths", "%q must be a path inside the repository, such as .env or storage/", p)
		}
	}
	for i, p := range in.SharedPaths {
		for _, q := range in.SharedPaths[i+1:] {
			a, b := strings.TrimSuffix(p, "/"), strings.TrimSuffix(q, "/")
			if strings.HasPrefix(b+"/", a+"/") || strings.HasPrefix(a+"/", b+"/") {
				return invalid("shared_paths", "%s and %s overlap; share only one of them", p, q)
			}
		}
	}
	if len(in.AfterActivate) > maxAfterActivate {
		return invalid("after_activate", "must be at most 64 KiB")
	}

	for name, spec := range in.Runtimes {
		if !slices.Contains(toolchain.Runtimes, name) {
			return invalid("runtimes", "unknown runtime %q", name)
		}
		if err := toolchain.ValidSetting(spec); err != nil {
			return invalid("runtimes."+name, "%s", err)
		}
	}

	if in.TimeoutSec < minTimeout || in.TimeoutSec > maxTimeout {
		return invalid("timeout_sec", "must be between %d and %d seconds", minTimeout, maxTimeout)
	}
	return nil
}

func validateRepoURL(u string) error {
	switch {
	case u == "":
		return invalid("repo_url", "is required")
	case strings.HasPrefix(u, "-"):
		return invalid("repo_url", "must not start with '-'")
	case scpRe.MatchString(u), urlRe.MatchString(u):
		return nil
	}
	return invalid("repo_url", "use git@host:owner/repo.git, ssh://… or https://…")
}

func validateWorkDir(dir, dataDir, projectsDir string) error {
	if dir == "" {
		return invalid("work_dir", "is required")
	}
	if !filepath.IsAbs(dir) {
		return invalid("work_dir", "must be an absolute path")
	}
	if dir == "/" {
		return invalid("work_dir", "must not be the filesystem root")
	}
	if dataDir != "" && (within(dir, dataDir) || within(dataDir, dir)) {
		return invalid("work_dir", "must not overlap degoploy's data directory (%s)", dataDir)
	}
	if projectsDir != "" && within(projectsDir, dir) {
		return invalid("work_dir", "must not contain the projects folder (%s); leave it empty to use %s/<slug>", projectsDir, projectsDir)
	}
	return nil
}

// within reports whether path is dir itself or inside it (both cleaned, absolute).
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}
