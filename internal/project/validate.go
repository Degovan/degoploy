package project

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/degovan/degoploy/internal/store"
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
)

func (in *Input) normalize() {
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.TrimSpace(strings.ToLower(in.Slug))
	in.Provider = strings.TrimSpace(strings.ToLower(in.Provider))
	in.RepoURL = strings.TrimSpace(in.RepoURL)
	in.WorkDir = strings.TrimSpace(in.WorkDir)
	in.ScriptSource = strings.TrimSpace(in.ScriptSource)
	in.ScriptPath = strings.TrimSpace(in.ScriptPath)
	if in.WorkDir != "" {
		in.WorkDir = filepath.Clean(in.WorkDir)
	}
	if in.ScriptSource == store.ScriptManaged {
		in.ScriptPath = ""
	} else if in.ScriptPath != "" {
		in.ScriptPath = filepath.ToSlash(filepath.Clean(in.ScriptPath))
	}
}

// validate checks a normalized Input. dataDir is degoploy's own data directory.
func (in *Input) validate(dataDir string, creating bool) error {
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
	if err := validateWorkDir(in.WorkDir, dataDir); err != nil {
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

func validateWorkDir(dir, dataDir string) error {
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
	return nil
}

// within reports whether path is dir itself or inside it (both cleaned, absolute).
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}
