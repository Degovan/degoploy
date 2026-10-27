// Package gitops runs git with a project's deploy key and an isolated SSH setup.
package gitops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Env describes the credentials git should use for one project.
type Env struct {
	KeyPath    string // private key; empty for public https remotes
	KnownHosts string // shared known_hosts file
}

// Environ returns os.Environ() plus the git/ssh settings for e.
//
// ssh ignores the user's ~/.ssh/config (-F /dev/null) so every project behaves the
// same regardless of which account runs degoploy, and git is limited to the
// transports degoploy supports (no ext::, file:// tricks).
func (e Env) Environ() []string {
	sshCmd := []string{"ssh", "-F", "/dev/null",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=15",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if e.KnownHosts != "" {
		sshCmd = append(sshCmd, "-o", "UserKnownHostsFile="+shellQuote(e.KnownHosts))
	}
	if e.KeyPath != "" {
		sshCmd = append(sshCmd, "-o", "IdentitiesOnly=yes", "-i", shellQuote(e.KeyPath))
	}
	return append(BaseEnviron(),
		"GIT_SSH_COMMAND="+strings.Join(sshCmd, " "),
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
		"GIT_ALLOW_PROTOCOL=ssh:https:http",
	)
}

// BaseEnviron is os.Environ() without degoploy's own DEGOPLOY_* settings, so
// secrets such as the OIDC client secret never reach git or deploy scripts.
func BaseEnviron() []string {
	var out []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "DEGOPLOY_") {
			out = append(out, kv)
		}
	}
	return out
}

type Ref struct {
	Name string `json:"name"`
	SHA  string `json:"sha"`
}

// LsRemote lists branch heads of url. On failure the error includes git's stderr.
func LsRemote(ctx context.Context, env Env, url string) ([]Ref, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--heads", "--", url)
	cmd.Env = env.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, errors.New("timed out after 30s contacting the remote")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s", msg)
	}

	var refs []Ref
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		sha, name, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		refs = append(refs, Ref{Name: strings.TrimPrefix(name, "refs/heads/"), SHA: sha})
	}
	return refs, nil
}

// shellQuote quotes s for the sh -c that git uses to run GIT_SSH_COMMAND.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
