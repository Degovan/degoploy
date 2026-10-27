package project

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/degovan/degoploy/internal/fsutil"
	"github.com/degovan/degoploy/internal/store"
)

// MaxScriptSize bounds script content stored or read by degoploy.
const MaxScriptSize = 256 << 10

// Script is the deploy script as degoploy sees it right now.
type Script struct {
	Source  string // managed | repo
	Path    string // absolute path on disk
	Content string
	Exists  bool
}

// DefaultScript is written for new managed-script projects.
func DefaultScript(p *store.Project) string {
	return `#!/usr/bin/env bash
# degoploy deploy script for "` + p.Name + `".
#
# Runs with bash inside the project's working directory after the repository has
# been updated (auto_pull). A non-zero exit marks the deployment as failed.
#
# Available environment:
#   DEPLOY_ID, DEPLOY_PROJECT, DEPLOY_RULE, DEPLOY_TRIGGER
#   DEPLOY_REF, DEPLOY_BRANCH or DEPLOY_TAG, DEPLOY_COMMIT, DEPLOY_AUTHOR
#   DEPLOY_WORKDIR, GIT_SSH_COMMAND (uses this project's deploy key)
set -euo pipefail

echo "Deploying ${DEPLOY_PROJECT} @ ${DEPLOY_COMMIT:0:7}"

# Examples:
# composer install --no-dev --optimize-autoloader
# npm ci && npm run build
# sudo systemctl restart my-app
`
}

// ReadScript loads the script that would run for p.
func (m *Manager) ReadScript(p *store.Project) (*Script, error) {
	s := &Script{Source: p.ScriptSource}
	if p.ScriptSource == store.ScriptManaged {
		s.Path = m.ManagedScriptPath(p)
	} else {
		path, err := ResolveRepoScript(p.WorkDir, p.ScriptPath)
		if err != nil {
			s.Path = filepath.Join(p.WorkDir, p.ScriptPath)
			if errors.Is(err, os.ErrNotExist) {
				return s, nil
			}
			return nil, err
		}
		s.Path = path
	}

	f, err := os.Open(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxScriptSize+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxScriptSize {
		return nil, fmt.Errorf("script is larger than %d KiB", MaxScriptSize>>10)
	}
	s.Content, s.Exists = string(b), true
	return s, nil
}

// SaveScript writes a managed script and records a revision.
func (m *Manager) SaveScript(ctx context.Context, p *store.Project, content string, userID int64) (*store.ScriptRevision, error) {
	if p.ScriptSource != store.ScriptManaged {
		return nil, invalid("content", "this project runs a script from its repository; edit it there")
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if strings.TrimSpace(content) == "" {
		return nil, invalid("content", "script must not be empty")
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if len(content) > MaxScriptSize {
		return nil, invalid("content", "script must be at most %d KiB", MaxScriptSize>>10)
	}

	if err := fsutil.WriteFileAtomic(m.ManagedScriptPath(p), []byte(content), 0o700); err != nil {
		return nil, err
	}
	rev := &store.ScriptRevision{ProjectID: p.ID, Content: content}
	if userID != 0 {
		rev.CreatedBy = &userID
	}
	if err := m.store.AddScriptRevision(ctx, rev); err != nil {
		return nil, err
	}
	return rev, nil
}

// ResolveRepoScript returns the absolute path of rel inside workDir, following
// symlinks, and rejects anything that resolves outside workDir.
func ResolveRepoScript(workDir, rel string) (string, error) {
	root, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		return "", err
	}
	target, err := filepath.EvalSymlinks(filepath.Join(root, rel))
	if err != nil {
		return "", err
	}
	if !within(target, root) {
		return "", fmt.Errorf("script path %q resolves outside the working directory", rel)
	}
	return target, nil
}
