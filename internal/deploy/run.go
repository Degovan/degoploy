package deploy

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/degovan/degoploy/internal/project"
	"github.com/degovan/degoploy/internal/secrets"
	"github.com/degovan/degoploy/internal/store"
)

// killGrace is how long a canceled or timed-out process group gets after SIGTERM.
const killGrace = 10 * time.Second

// run performs the deployment and returns its final status and, unless it
// succeeded, a one-line reason.
func (s *Service) run(ctx context.Context, d *store.Deployment, p *store.Project, w io.Writer) (string, *int, string) {
	start := time.Now()
	ctx, cancel := context.WithTimeoutCause(ctx, time.Duration(p.TimeoutSec)*time.Second, errTimeout)
	defer cancel()

	step(w, "Deployment #%d of %s (%s)", d.ID, p.Name, p.Slug)
	note(w, "trigger: %s   ref: %s   timeout: %ds", d.Trigger, orDash(d.Ref), p.TimeoutSec)

	var rule *store.TriggerRule
	if d.RuleID != nil {
		if r, err := s.store.RuleByID(ctx, p.ID, *d.RuleID); err == nil {
			rule = r
			note(w, "rule: %s", r.Name)
		}
	}

	vars, masked, err := s.projectVars(ctx, p)
	if err != nil {
		return s.outcome(ctx, w, start, err)
	}
	if len(vars) > 0 {
		names := make([]string, len(vars))
		for i, kv := range vars {
			names[i], _, _ = strings.Cut(kv, "=")
		}
		note(w, "variables: %s", strings.Join(names, ", "))
	}

	if p.AutoPull {
		if err := s.pull(ctx, d, p, w); err != nil {
			return s.outcome(ctx, w, start, err)
		}
	} else if st, err := os.Stat(p.WorkDir); err != nil || !st.IsDir() {
		return s.outcome(ctx, w, start, fmt.Errorf("working directory %s does not exist (auto pull is off)", p.WorkDir))
	}

	script, err := s.scriptPath(p)
	if err != nil {
		return s.outcome(ctx, w, start, err)
	}
	if err := s.store.MarkScriptStarted(context.WithoutCancel(ctx), d.ID, p.ScriptSource == store.ScriptManaged); err == nil {
		s.notify(d.ID)
	}
	step(w, "Running %s", script)

	cmd := exec.CommandContext(ctx, "bash", "--noprofile", "--norc", script)
	cmd.Dir = p.WorkDir
	cmd.Env = s.scriptEnv(d, p, rule, vars)
	setProcessGroup(cmd)
	// Secrets are masked before the last-line summary sees the output.
	out := &lastLine{w: w}
	red := newRedactor(out, masked)
	cmd.Stdout, cmd.Stderr = red, red
	err = cmd.Run()
	red.Flush()
	if ctx.Err() != nil && cmd.Process != nil {
		// Make sure nothing from a canceled/timed-out group survives.
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	if err != nil {
		// bash prefixes errors with the script path; the file name is enough.
		last := strings.ReplaceAll(out.Last(), script+":", filepath.Base(script)+":")
		err = &scriptError{err: err, last: last}
	}
	return s.outcome(ctx, w, start, err)
}

// scriptError is a failed deploy script or git command with the last line it printed.
type scriptError struct {
	err  error
	git  bool
	last string
}

func (e *scriptError) Error() string { return e.err.Error() }
func (e *scriptError) Unwrap() error { return e.err }

// outcome maps an error (or nil) to a deployment status, writes the summary
// line and returns the error summary.
func (s *Service) outcome(ctx context.Context, w io.Writer, start time.Time, err error) (string, *int, string) {
	took := time.Since(start).Round(100 * time.Millisecond)
	cause := context.Cause(ctx)
	var exitErr *exec.ExitError
	var cmdErr *scriptError
	tail := ""
	if errors.As(err, &cmdErr) {
		tail = cmdErr.last
	}
	switch {
	case err == nil:
		code := 0
		fmt.Fprintf(w, "\n%s✔ Deployment succeeded%s %sin %s%s\n", ansiGreen, ansiReset, ansiDim, took, ansiReset)
		return "success", &code, ""
	case errors.Is(cause, errTimeout):
		fmt.Fprintf(w, "\n%s✖ Timed out after %s%s\n", ansiRed, took, ansiReset)
		return "timeout", nil, fmt.Sprintf("timed out after %s", took)
	case errors.Is(cause, errCanceledByUser), errors.Is(cause, errShutdown), errors.Is(cause, context.Canceled):
		reason := errCanceledByUser.Error()
		if errors.Is(cause, errShutdown) || errors.Is(cause, context.Canceled) {
			reason = errShutdown.Error()
		}
		fmt.Fprintf(w, "\n%s■ Canceled: %s%s\n", ansiYellow, reason, ansiReset)
		return "canceled", nil, reason
	case cmdErr != nil && !cmdErr.git && errors.As(err, &exitErr):
		code := exitErr.ExitCode()
		fmt.Fprintf(w, "\n%s✖ Script exited with code %d%s %safter %s%s\n", ansiRed, code, ansiReset, ansiDim, took, ansiReset)
		return "failed", &code, cmp.Or(tail, fmt.Sprintf("script exited with code %d", code))
	default:
		fmt.Fprintf(w, "\n%s✖ %s%s\n", ansiRed, err, ansiReset)
		return "failed", nil, cmp.Or(tail, err.Error())
	}
}

// pull makes work_dir a checkout of the deployment's commit using the project's deploy key.
func (s *Service) pull(ctx context.Context, d *store.Deployment, p *store.Project, w io.Writer) error {
	if d.Ref == "" {
		return errors.New("no ref to deploy")
	}
	env := s.projects.GitEnv(p).Environ()
	git := func(args ...string) error {
		fmt.Fprintf(w, "%s$ git %s%s\n", ansiDim, strings.Join(args, " "), ansiReset)
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = p.WorkDir
		cmd.Env = env
		out := &lastLine{w: w}
		cmd.Stdout, cmd.Stderr = out, out
		setProcessGroup(cmd)
		if err := cmd.Run(); err != nil {
			return &scriptError{err: err, git: true, last: out.Last()}
		}
		return nil
	}

	step(w, "Updating repository in %s", p.WorkDir)
	if _, err := os.Stat(filepath.Join(p.WorkDir, ".git")); errors.Is(err, os.ErrNotExist) {
		if entries, err := os.ReadDir(p.WorkDir); err == nil && len(entries) > 0 {
			return fmt.Errorf("%s exists, is not empty and is not a git repository; refusing to clone into it", p.WorkDir)
		}
		if err := os.MkdirAll(p.WorkDir, 0o755); err != nil {
			return err
		}
		if err := git("init", "--quiet"); err != nil {
			return err
		}
		if err := git("remote", "add", "origin", p.RepoURL); err != nil {
			return err
		}
	} else if err := git("remote", "set-url", "origin", p.RepoURL); err != nil {
		return err
	}

	if err := git("fetch", "--force", "--prune", "--no-tags", "origin", "--", d.Ref); err != nil {
		return fmt.Errorf("git fetch failed: %w", err)
	}
	target := "FETCH_HEAD"
	if d.CommitSHA != "" && strings.Trim(d.CommitSHA, "0") != "" {
		target = d.CommitSHA
	}
	if err := git("-c", "advice.detachedHead=false", "checkout", "--force", "--detach", target, "--"); err != nil {
		return fmt.Errorf("git checkout failed: %w", err)
	}
	if _, err := os.Stat(filepath.Join(p.WorkDir, ".gitmodules")); err == nil {
		if err := git("submodule", "update", "--init", "--recursive", "--force"); err != nil {
			return fmt.Errorf("git submodule update failed: %w", err)
		}
	}

	// Record what was actually deployed (manual deploys only know the ref).
	out, err := gitOutput(ctx, p.WorkDir, env, "log", "-1", "--format=%H%x00%s%x00%an")
	if err == nil {
		parts := strings.SplitN(strings.TrimSpace(out), "\x00", 3)
		if len(parts) == 3 {
			d.CommitSHA = parts[0]
			s.store.SetDeploymentCommit(context.WithoutCancel(ctx), d.ID, parts[0], parts[1], parts[2])
			s.notify(d.ID)
		}
	}
	return nil
}

func (s *Service) scriptPath(p *store.Project) (string, error) {
	if p.ScriptSource == store.ScriptManaged {
		path := s.projects.ManagedScriptPath(p)
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("managed deploy script missing: %w", err)
		}
		return path, nil
	}
	path, err := project.ResolveRepoScript(p.WorkDir, p.ScriptPath)
	if err != nil {
		return "", fmt.Errorf("deploy script %s: %w", p.ScriptPath, err)
	}
	return path, nil
}

// projectVars returns the project's variables as KEY=value pairs, with secrets
// decrypted, and the secret values to mask in the log.
func (s *Service) projectVars(ctx context.Context, p *store.Project) (env, masked []string, err error) {
	vars, err := s.store.ListVariables(ctx, p.ID)
	if err != nil {
		return nil, nil, err
	}
	for _, v := range vars {
		value := v.Value
		if v.Secret {
			if s.secrets == nil {
				return nil, nil, fmt.Errorf("secret %s: no secret key configured", v.Name)
			}
			if value, err = s.secrets.Open(v.Value, secrets.ProjectScope(p.ID)); err != nil {
				return nil, nil, fmt.Errorf("secret %s: %w", v.Name, err)
			}
			masked = append(masked, value)
		}
		env = append(env, v.Name+"="+value)
	}
	return env, masked, nil
}

// scriptEnv builds the deploy script environment. Later entries win, so the
// order is: project variables, then rule variables, then the DEPLOY_* values
// set by degoploy.
//
// Scripts get the project's GIT_SSH_COMMAND (so `git pull` and private
// submodules use the deploy key) but not the transport restriction degoploy
// applies to its own git calls.
func (s *Service) scriptEnv(d *store.Deployment, p *store.Project, rule *store.TriggerRule, vars []string) []string {
	var env []string
	for _, kv := range s.projects.GitEnv(p).Environ() {
		if !strings.HasPrefix(kv, "GIT_ALLOW_PROTOCOL=") {
			env = append(env, kv)
		}
	}
	env = append(env, vars...)
	ruleName := ""
	if rule != nil {
		ruleName = rule.Name
		for k, v := range rule.ExtraEnv {
			env = append(env, k+"="+v)
		}
	}
	branch, tag := "", ""
	switch {
	case strings.HasPrefix(d.Ref, "refs/heads/"):
		branch = strings.TrimPrefix(d.Ref, "refs/heads/")
	case strings.HasPrefix(d.Ref, "refs/tags/"):
		tag = strings.TrimPrefix(d.Ref, "refs/tags/")
	}
	return append(env,
		"DEGOPLOY=1",
		"DEPLOY_ID="+strconv.FormatInt(d.ID, 10),
		"DEPLOY_PROJECT="+p.Slug,
		"DEPLOY_PROJECT_NAME="+p.Name,
		"DEPLOY_RULE="+ruleName,
		"DEPLOY_TRIGGER="+d.Trigger,
		"DEPLOY_REF="+d.Ref,
		"DEPLOY_BRANCH="+branch,
		"DEPLOY_TAG="+tag,
		"DEPLOY_COMMIT="+d.CommitSHA,
		"DEPLOY_AUTHOR="+d.Author,
		"DEPLOY_WORKDIR="+p.WorkDir,
	)
}

// setProcessGroup runs cmd in its own process group; cancellation sends SIGTERM
// to the whole group and Go force-kills the leader after killGrace.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	// Also bounds how long we wait for background children holding the output pipe.
	cmd.WaitDelay = killGrace
}

func gitOutput(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.String(), err
}

func step(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, "%s==>%s %s%s%s\n", ansiCyan, ansiReset, ansiBold, fmt.Sprintf(format, args...), ansiReset)
}

func note(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, "    %s%s%s\n", ansiDim, fmt.Sprintf(format, args...), ansiReset)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
