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

	"github.com/degovan/degoploy/internal/gitops"
	"github.com/degovan/degoploy/internal/project"
	"github.com/degovan/degoploy/internal/runas"
	"github.com/degovan/degoploy/internal/secrets"
	"github.com/degovan/degoploy/internal/store"
	"github.com/degovan/degoploy/internal/toolchain"
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

	// git and the script use the deploy key through an agent; the key file
	// stays where only degoploy can read it.
	gid, err := s.runner().GID()
	if err != nil {
		return s.outcome(ctx, w, start, err)
	}
	agent, err := gitops.StartAgent(s.projects.KeyPath(p), s.projects.KnownHostsPath(), gid)
	if err != nil {
		return s.outcome(ctx, w, start, fmt.Errorf("deploy key: %w", err))
	}
	defer agent.Close()
	gitEnv := agent.Env().Environ()

	if p.AutoPull {
		if err := s.pull(ctx, d, p, w, gitEnv); err != nil {
			return s.outcome(ctx, w, start, err)
		}
	} else if st, err := os.Stat(p.WorkDir); err != nil || !st.IsDir() {
		return s.outcome(ctx, w, start, fmt.Errorf("working directory %s does not exist (auto pull is off)", p.WorkDir))
	}

	script, content, err := s.script(p)
	if err != nil {
		return s.outcome(ctx, w, start, err)
	}
	if err := s.store.MarkScriptStarted(context.WithoutCancel(ctx), d.ID, p.ScriptSource == store.ScriptManaged); err == nil {
		s.notify(d.ID)
	}
	bins, runtimeEnv, err := s.setupRuntimes(ctx, p, w)
	if err != nil {
		return s.outcome(ctx, w, start, err)
	}
	step(w, "Running %s", script)

	env := s.scriptEnv(d, p, rule, vars, gitEnv)
	if len(bins) > 0 {
		env = withRuntimes(env, bins, runtimeEnv)
	}
	spec := runas.Spec{Dir: p.WorkDir, Env: env, Argv: []string{"bash", "--noprofile", "--norc", script}}
	if content != "" {
		// The deploy user cannot read degoploy's copy, so bash gets the text.
		spec.Script, spec.Argv[3] = content, runas.ScriptPath
	}
	cmd, err := s.runner().Command(ctx, spec)
	if err != nil {
		return s.outcome(ctx, w, start, err)
	}
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
		last := strings.ReplaceAll(out.Last(), spec.Argv[3]+":", filepath.Base(script)+":")
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
func (s *Service) pull(ctx context.Context, d *store.Deployment, p *store.Project, w io.Writer, env []string) error {
	if d.Ref == "" {
		return errors.New("no ref to deploy")
	}
	mkdir := false
	git := func(args ...string) error {
		fmt.Fprintf(w, "%s$ git %s%s\n", ansiDim, strings.Join(args, " "), ansiReset)
		cmd, err := s.runner().Command(ctx, runas.Spec{Dir: p.WorkDir, Mkdir: mkdir, Env: env, Argv: append([]string{"git"}, args...)})
		if err != nil {
			return err
		}
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
		// Made by the deploy user, group-writable under the unit's umask
		// (0002), so the web server's group can write to the app's storage.
		mkdir = true
		if err := git("init", "--quiet", "--shared=group"); err != nil {
			return err
		}
		mkdir = false
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
	out, err := s.gitOutput(ctx, p.WorkDir, env, "log", "-1", "--format=%H%x00%s%x00%an")
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

// script returns the path of the deploy script and, for a managed script,
// its content.
func (s *Service) script(p *store.Project) (path, content string, err error) {
	if p.ScriptSource == store.ScriptManaged {
		path := s.projects.ManagedScriptPath(p)
		b, err := os.ReadFile(path)
		if err != nil {
			return "", "", fmt.Errorf("managed deploy script missing: %w", err)
		}
		return path, string(b), nil
	}
	path, err = project.ResolveRepoScript(p.WorkDir, p.ScriptPath)
	if err != nil {
		return "", "", fmt.Errorf("deploy script %s: %w", p.ScriptPath, err)
	}
	return path, "", nil
}

// setupRuntimes picks the version of each runtime (Node.js, PHP) the project
// uses, installs it when needed, and returns the bin directories to put first
// on PATH and the environment the runtimes add.
func (s *Service) setupRuntimes(ctx context.Context, p *store.Project, w io.Writer) (bins, env []string, err error) {
	if s.Runtimes == nil {
		return nil, nil, nil
	}
	settings, err := s.store.ProjectRuntimes(ctx, p.ID)
	if err != nil {
		return nil, nil, err
	}
	for _, m := range s.Runtimes.All() {
		name := m.Name()
		if on, err := s.store.Setting(ctx, store.SettingRuntimeEnabled(name)); err != nil {
			return nil, nil, err
		} else if on == "false" {
			continue
		}
		spec, source := settings[name].Spec, "project settings"
		switch spec {
		case toolchain.Off:
			continue
		case toolchain.Auto, "":
			var uses bool
			spec, source, uses = m.Kind().Detect(p.WorkDir)
			if !uses {
				continue
			}
			if spec == "" {
				def, err := s.store.Setting(ctx, store.SettingRuntimeDefault(name))
				if err != nil {
					return nil, nil, err
				}
				if def == "" {
					note(w, "%s: the server's own (the repository names no version and there is no default)", name)
					continue
				}
				spec, source = def, "the default version"
			}
		}
		v, installed, err := m.Resolve(ctx, spec)
		if err != nil {
			return nil, nil, fmt.Errorf("%s %s (from %s): %w", m.Title(), spec, source, err)
		}
		if !installed {
			step(w, "Installing %s %s", m.Title(), m.Label(v))
		}
		v, bin, err := m.Use(ctx, spec, noteWriter{w})
		if err != nil {
			return nil, nil, fmt.Errorf("install %s %s: %w", m.Title(), m.Label(v), err)
		}
		label := m.Label(v)
		where := ""
		if in, _ := m.Lookup(v); in.System {
			where = "system, "
		}
		note(w, "%s: %s (%s%s from %s)", name, label, where, spec, source)
		if settings[name].Used != label {
			s.store.SetProjectRuntimeUsed(context.WithoutCancel(ctx), p.ID, name, label)
		}
		bins = append(bins, bin)
		env = append(env, m.Env()...)
	}
	return bins, env, nil
}

// withRuntimes puts bins first on PATH, in order, and adds env.
func withRuntimes(base, bins, env []string) []string {
	out := make([]string, 0, len(base)+len(env)+1)
	path := "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	for _, kv := range base {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			path = v
			continue
		}
		out = append(out, kv)
	}
	out = append(out, env...)
	return append(out, "PATH="+strings.Join(append(bins, path), string(os.PathListSeparator)))
}

// noteWriter prints each line written to it as a note.
type noteWriter struct{ w io.Writer }

func (n noteWriter) Write(b []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		note(n.w, "%s", line)
	}
	return len(b), nil
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
// Scripts get gitEnv's GIT_SSH_COMMAND (so `git pull` and private submodules
// use the deploy key) but not the transport restriction degoploy applies to
// its own git calls.
func (s *Service) scriptEnv(d *store.Deployment, p *store.Project, rule *store.TriggerRule, vars, gitEnv []string) []string {
	var env []string
	for _, kv := range gitEnv {
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
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	// Also bounds how long we wait for background children holding the output pipe.
	cmd.WaitDelay = killGrace
}

func (s *Service) gitOutput(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	cmd, err := s.runner().Command(ctx, runas.Spec{Dir: dir, Env: env, Argv: append([]string{"git"}, args...)})
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
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
