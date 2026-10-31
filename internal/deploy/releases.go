package deploy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/degovan/degoploy/internal/release"
	"github.com/degovan/degoploy/internal/runas"
	"github.com/degovan/degoploy/internal/store"
)

// cleanupTimeout bounds removing a failed release after its deployment was
// canceled or timed out.
const cleanupTimeout = time.Minute

// build checks the deployment's commit out into a release of its own, runs
// the script there and, when it succeeds, makes the release current.
func (s *Service) build(ctx context.Context, j *job) error {
	d, p, w := j.d, j.p, j.w
	if d.Ref == "" {
		return errors.New("no ref to deploy")
	}
	base, repo := p.WorkDir, release.Repo(p.WorkDir)
	dir := release.Dir(base, d.ID)
	git := func(dir string, mkdir bool, args ...string) error {
		return s.git(ctx, w, dir, mkdir, j.gitEnv, args...)
	}

	step(w, "Checking out release %d in %s", d.ID, dir)
	if _, err := os.Stat(filepath.Join(base, ".git")); err == nil {
		return fmt.Errorf("%s holds a checkout deployed in place; releases need a folder of their own", base)
	}
	if err := s.releaseOp(ctx, j, release.Op{Action: release.OpPrepare, Base: base, ID: d.ID}); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(repo, "HEAD")); errors.Is(err, os.ErrNotExist) {
		if err := git(repo, true, "init", "--quiet", "--bare", "--shared=group"); err != nil {
			return err
		}
		if err := git(repo, false, "remote", "add", "origin", p.RepoURL); err != nil {
			return err
		}
	} else if err := git(repo, false, "remote", "set-url", "origin", p.RepoURL); err != nil {
		return err
	}
	if err := git(repo, false, "fetch", "--force", "--prune", "--no-tags", "origin", "--", d.Ref); err != nil {
		return fmt.Errorf("git fetch failed: %w", err)
	}
	target := "FETCH_HEAD"
	if d.CommitSHA != "" && strings.Trim(d.CommitSHA, "0") != "" {
		target = d.CommitSHA
	}
	// Forget releases removed since the last deployment.
	if err := git(repo, false, "worktree", "prune"); err != nil {
		return err
	}
	if err := git(repo, false, "worktree", "add", "--quiet", "--detach", dir, target); err != nil {
		return fmt.Errorf("git worktree add failed: %w", err)
	}
	j.release, j.dir = d.ID, dir
	s.store.SetDeploymentRelease(context.WithoutCancel(ctx), d.ID, d.ID)
	if _, err := os.Stat(filepath.Join(dir, ".gitmodules")); err == nil {
		if err := git(dir, false, "submodule", "update", "--init", "--recursive", "--force"); err != nil {
			return fmt.Errorf("git submodule update failed: %w", err)
		}
	}
	s.recordCommit(ctx, d, dir, j.gitEnv)

	if len(p.SharedPaths) > 0 {
		step(w, "Linking shared files")
		if err := s.releaseOp(ctx, j, release.Op{Action: release.OpLink, Base: base, ID: d.ID, Shared: p.SharedPaths}); err != nil {
			return fmt.Errorf("link shared files: %w", err)
		}
	}
	if err := s.runScript(ctx, j); err != nil {
		return err
	}
	return s.activate(ctx, j)
}

// rollback makes a release that is still on the server current again.
func (s *Service) rollback(ctx context.Context, j *job) error {
	d, p := j.d, j.p
	if !p.Releases {
		return errors.New("this project deploys in place; it has no releases to switch to")
	}
	if d.ReleaseID == nil {
		return errors.New("no release to switch to")
	}
	j.release = *d.ReleaseID
	j.dir = release.Dir(p.WorkDir, j.release)
	if !release.Exists(p.WorkDir, j.release) {
		return fmt.Errorf("release %d is no longer on the server", j.release)
	}
	if err := s.store.MarkScriptStarted(context.WithoutCancel(ctx), d.ID, false); err == nil {
		s.notify(d.ID)
	}
	if p.AfterActivate != "" {
		var err error
		if j.bins, j.runtimeEnv, err = s.setupRuntimes(ctx, p, j.w, j.dir); err != nil {
			return err
		}
	}
	return s.activate(ctx, j)
}

// activate points current at j.release, runs the after-activate commands and
// the health check, and removes old releases. A failed health check switches
// back to the release that was live (see restore).
func (s *Service) activate(ctx context.Context, j *job) error {
	p, w := j.p, j.w
	base := p.WorkDir
	prev := release.Live(base)
	step(w, "Switching current to release %d", j.release)
	if err := s.releaseOp(ctx, j, release.Op{Action: release.OpActivate, Base: base, ID: j.release}); err != nil {
		return err
	}
	note(w, "%s → %s", release.Current(base), j.dir)

	var err error
	if p.AfterActivate != "" {
		step(w, "Running the after-activate commands")
		if err = s.bash(ctx, j, release.Current(base), "after-activate", p.AfterActivate); err != nil {
			note(w, "release %d is live, but the after-activate commands failed", j.release)
		}
	}
	if err == nil && p.HealthURL != "" {
		if err = s.checkHealth(ctx, j); err != nil && ctx.Err() == nil {
			if rollsBack(j, prev) {
				err = s.restore(ctx, j, prev, err)
			} else {
				note(w, "release %d stays live: there is no earlier release to switch back to", j.release)
			}
		}
	}

	// Old releases go even when the steps above failed: the switch is done.
	// They go only now, so the release a failed check switches back to is still there.
	pruneCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if perr := s.releaseOp(pruneCtx, j, release.Op{Action: release.OpPrune, Base: base, ID: release.Live(base), Keep: p.KeepReleases}); perr != nil {
		note(w, "removing old releases: %v", perr)
	}
	if _, serr := os.Stat(filepath.Join(release.Repo(base), "HEAD")); serr == nil {
		s.gitOutput(pruneCtx, release.Repo(base), j.gitEnv, "worktree", "prune")
	}
	return err
}

// discard removes a release whose deployment failed, so only releases that
// went live stay on the server.
func (s *Service) discard(ctx context.Context, j *job) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if release.Live(j.p.WorkDir) == j.release {
		return
	}
	if err := s.releaseOp(ctx, j, release.Op{Action: release.OpRemove, Base: j.p.WorkDir, ID: j.release}); err != nil {
		note(j.w, "could not remove release %d: %v", j.release, err)
		return
	}
	s.gitOutput(ctx, release.Repo(j.p.WorkDir), j.gitEnv, "worktree", "prune")
	note(j.w, "removed release %d; current still points at the release that was live", j.release)
}

// releaseOp changes the project's releases as the deploy user, which owns them.
func (s *Service) releaseOp(ctx context.Context, j *job, op release.Op) error {
	cmd, err := s.runner().Command(ctx, runas.Spec{Release: &op})
	if err != nil {
		return err
	}
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	setProcessGroup(cmd)
	err = cmd.Run()
	if out.Len() > 0 {
		noteWriter{j.w}.Write(out.Bytes())
	}
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return errors.New(msg)
		}
		return err
	}
	return nil
}

// recordCommit records what was actually deployed (manual deploys only know the ref).
func (s *Service) recordCommit(ctx context.Context, d *store.Deployment, dir string, env []string) {
	out, err := s.gitOutput(ctx, dir, env, "log", "-1", "--format=%H%x00%s%x00%an")
	if err != nil {
		return
	}
	parts := strings.SplitN(strings.TrimSpace(out), "\x00", 3)
	if len(parts) == 3 {
		d.CommitSHA = parts[0]
		s.store.SetDeploymentCommit(context.WithoutCancel(ctx), d.ID, parts[0], parts[1], parts[2])
		s.notify(d.ID)
	}
}
