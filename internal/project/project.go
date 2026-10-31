// Package project owns project lifecycle: database rows plus each project's
// on-disk directory (deploy key, managed script, logs).
package project

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/degovan/degoploy/internal/gitops"
	"github.com/degovan/degoploy/internal/sshkey"
	"github.com/degovan/degoploy/internal/store"
	"github.com/degovan/degoploy/internal/trigger"
)

// ErrSlugTaken is returned when another project already uses the slug.
var ErrSlugTaken = &ValidationError{Field: "slug", Message: "is already used by another project"}

type Manager struct {
	store       *store.Store
	dataDir     string
	projectsDir string

	// CheckAccess checks that deployments may clone into a work_dir, as the
	// user they run as; nil checks as this process's user.
	CheckAccess func(ctx context.Context, dir string, releases bool) error
}

// NewManager manages projects with their data in dataDir. Projects without a
// working directory of their own get <projectsDir>/<slug>; an empty
// projectsDir makes the working directory required.
func NewManager(st *store.Store, dataDir, projectsDir string) *Manager {
	return &Manager{store: st, dataDir: dataDir, projectsDir: projectsDir}
}

// ProjectsDir is where projects without a working directory of their own go.
func (m *Manager) ProjectsDir() string { return m.projectsDir }

func (m *Manager) Dir(p *store.Project) string    { return filepath.Join(m.dataDir, "projects", p.Slug) }
func (m *Manager) SSHDir(p *store.Project) string { return filepath.Join(m.Dir(p), "ssh") }
func (m *Manager) KeyPath(p *store.Project) string {
	return filepath.Join(m.SSHDir(p), sshkey.PrivateName)
}
func (m *Manager) ManagedScriptPath(p *store.Project) string {
	return filepath.Join(m.Dir(p), "deploy.sh")
}
func (m *Manager) LogDir(p *store.Project) string { return filepath.Join(m.Dir(p), "logs") }
func (m *Manager) KnownHostsPath() string         { return filepath.Join(m.dataDir, "known_hosts") }

// GitEnv returns the credentials git should use for p.
func (m *Manager) GitEnv(p *store.Project) gitops.Env {
	return gitops.Env{KeyPath: m.KeyPath(p), KnownHosts: m.KnownHostsPath()}
}

// InputFrom returns the editable fields of p, for merging partial updates.
func InputFrom(p *store.Project) Input {
	return Input{
		Name: p.Name, Slug: p.Slug, Provider: p.Provider, RepoURL: p.RepoURL, WorkDir: p.WorkDir,
		ScriptSource: p.ScriptSource, ScriptPath: p.ScriptPath, AutoPull: p.AutoPull,
		TimeoutSec: p.TimeoutSec, Enabled: p.Enabled,
		Releases: p.Releases, KeepReleases: p.KeepReleases, SharedPaths: p.SharedPaths, AfterActivate: p.AfterActivate,
	}
}

func (m *Manager) Create(ctx context.Context, in Input, userID int64) (*store.Project, error) {
	in.normalize()
	m.defaultWorkDir(&in, in.Slug)
	if err := in.validate(m.dataDir, m.projectsDir, true); err != nil {
		return nil, err
	}
	if err := m.checkWorkDirFree(ctx, in.WorkDir, 0); err != nil {
		return nil, err
	}
	if in.AutoPull {
		if err := m.checkAccess(ctx, in.WorkDir, in.Releases); err != nil {
			return nil, err
		}
	}
	var firstRule *store.TriggerRule
	if b := strings.TrimSpace(in.DefaultBranch); b != "" {
		firstRule = &store.TriggerRule{
			Name: "Push to " + b, Enabled: true, Event: trigger.KindPush, RefPattern: b,
			SkipKeyword: "[skip deploy]",
		}
		if field, msg := trigger.ValidateRule(firstRule); field != "" {
			return nil, invalid("default_branch", "%s", msg)
		}
	}

	p := &store.Project{
		PublicID:      newPublicID(),
		Slug:          in.Slug,
		WebhookSecret: newSecret(),
	}
	apply(p, in)

	if err := m.store.CreateProject(ctx, p); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			return nil, ErrSlugTaken
		}
		return nil, err
	}

	err := m.initDir(ctx, p, userID)
	if err == nil {
		err = m.saveRuntimes(ctx, p, in)
	}
	if err == nil && firstRule != nil {
		firstRule.ProjectID = p.ID
		err = m.store.CreateRule(ctx, firstRule)
	}
	if err != nil {
		// Roll back so a half-initialised project never lingers.
		m.store.DeleteProject(context.WithoutCancel(ctx), p.ID)
		os.RemoveAll(m.Dir(p))
		return nil, fmt.Errorf("initialise project directory: %w", err)
	}
	return p, nil
}

func (m *Manager) initDir(ctx context.Context, p *store.Project, userID int64) error {
	dir := m.Dir(p)
	// No row owned this directory (the slug was free), so anything here is stale.
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(m.LogDir(p), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	if err := sshkey.Generate(m.SSHDir(p), keyComment(p)); err != nil {
		return err
	}
	if p.ScriptSource == store.ScriptManaged {
		if _, err := m.SaveScript(ctx, p, DefaultScript(p), userID); err != nil {
			return err
		}
	}
	return nil
}

// Update applies in to p. The slug is immutable and ignored.
func (m *Manager) Update(ctx context.Context, p *store.Project, in Input, userID int64) error {
	in.normalize()
	m.defaultWorkDir(&in, p.Slug)
	if err := in.validate(m.dataDir, m.projectsDir, false); err != nil {
		return err
	}
	if in.WorkDir != p.WorkDir {
		if err := m.checkWorkDirFree(ctx, in.WorkDir, p.ID); err != nil {
			return err
		}
	}
	// Only a new folder, pulling newly turned on or another layout: a project
	// whose folder broke later can still be edited, and its deployments say
	// what is wrong.
	if in.AutoPull && (in.WorkDir != p.WorkDir || !p.AutoPull || in.Releases != p.Releases) {
		if err := m.checkAccess(ctx, in.WorkDir, in.Releases); err != nil {
			return err
		}
	}
	switchedToManaged := p.ScriptSource != store.ScriptManaged && in.ScriptSource == store.ScriptManaged

	next := *p
	apply(&next, in)
	if err := m.store.UpdateProject(ctx, &next); err != nil {
		return err
	}
	*p = next
	if err := m.saveRuntimes(ctx, p, in); err != nil {
		return err
	}

	if switchedToManaged {
		if _, err := os.Stat(m.ManagedScriptPath(p)); errors.Is(err, os.ErrNotExist) {
			if _, err := m.SaveScript(ctx, p, DefaultScript(p), userID); err != nil {
				return err
			}
		}
	}
	return nil
}

// Delete removes the project row and its degoploy directory. work_dir is never touched.
func (m *Manager) Delete(ctx context.Context, p *store.Project) error {
	if err := m.store.DeleteProject(ctx, p.ID); err != nil {
		return err
	}
	return os.RemoveAll(m.Dir(p))
}

func (m *Manager) RegenerateKey(p *store.Project) error {
	return sshkey.Generate(m.SSHDir(p), keyComment(p))
}

func (m *Manager) RegenerateSecret(ctx context.Context, p *store.Project) error {
	next := *p
	next.WebhookSecret = newSecret()
	if err := m.store.UpdateProject(ctx, &next); err != nil {
		return err
	}
	*p = next
	return nil
}

// PublicKey returns the deploy key line and its SHA256 fingerprint.
func (m *Manager) PublicKey(p *store.Project) (string, string, error) {
	return sshkey.Public(m.SSHDir(p))
}

// TestConnection lists the remote's branches using the project's deploy key.
func (m *Manager) TestConnection(ctx context.Context, p *store.Project) ([]gitops.Ref, error) {
	if err := os.MkdirAll(m.dataDir, 0o700); err != nil {
		return nil, err
	}
	return gitops.LsRemote(ctx, m.GitEnv(p), p.RepoURL)
}

func (m *Manager) checkAccess(ctx context.Context, dir string, releases bool) error {
	if m.CheckAccess == nil {
		return CheckWorkDirAccess(dir, releases)
	}
	return m.CheckAccess(ctx, dir, releases)
}

// defaultWorkDir puts a project without a working directory in projects_dir.
func (m *Manager) defaultWorkDir(in *Input, slug string) {
	if in.WorkDir == "" && m.projectsDir != "" && slugRe.MatchString(slug) {
		in.WorkDir = filepath.Join(m.projectsDir, slug)
	}
}

// checkWorkDirFree rejects a work_dir equal to, inside, or containing another project's.
func (m *Manager) checkWorkDirFree(ctx context.Context, dir string, selfID int64) error {
	projects, err := m.store.ListProjects(ctx)
	if err != nil {
		return err
	}
	for _, other := range projects {
		if other.ID == selfID {
			continue
		}
		if within(dir, other.WorkDir) || within(other.WorkDir, dir) {
			return invalid("work_dir", "overlaps the working directory of project %q (%s)", other.Name, other.WorkDir)
		}
	}
	return nil
}

func apply(p *store.Project, in Input) {
	p.Name = in.Name
	p.Provider = in.Provider
	p.RepoURL = in.RepoURL
	p.WorkDir = in.WorkDir
	p.ScriptSource = in.ScriptSource
	p.ScriptPath = in.ScriptPath
	p.AutoPull = in.AutoPull
	p.TimeoutSec = in.TimeoutSec
	p.Enabled = in.Enabled
	p.Releases = in.Releases
	p.KeepReleases = in.KeepReleases
	p.SharedPaths = in.SharedPaths
	p.AfterActivate = in.AfterActivate
}

// saveRuntimes stores the runtime settings in carries.
func (m *Manager) saveRuntimes(ctx context.Context, p *store.Project, in Input) error {
	for name, spec := range in.Runtimes {
		if err := m.store.SetProjectRuntime(ctx, p.ID, name, spec); err != nil {
			return err
		}
	}
	return nil
}

func keyComment(p *store.Project) string { return "degoploy-" + p.Slug }

func newPublicID() string {
	b := make([]byte, 15)
	rand.Read(b)
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
}

func newSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
