// Package runas runs git and deploy scripts as the deploy user, so they cannot
// read degoploy's data directory (deploy keys, the secret key, the database)
// or reach its admin socket.
//
// degoploy starts "degoploy run-step" as that user: through sudo, with a rule
// degoploy setup writes, or directly when it runs as root or no deploy user is
// set. The command arrives on stdin as JSON, so project variables and secrets
// never show up in argv, which every user on the server can read. run-step
// starts the command in a process group of its own and stays as its parent, so
// it can stop the whole group when degoploy cancels: degoploy itself may not
// signal another user's processes.
package runas

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// Subcommand is the degoploy command that runs one step.
const Subcommand = "run-step"

// ScriptPath is how bash names a Spec.Script, for example in its error messages.
const ScriptPath = "/dev/fd/3"

// Spec is one command for run-step.
type Spec struct {
	// Dir is the working directory; Mkdir creates it (and its parents) first.
	Dir   string `json:"dir,omitempty"`
	Mkdir bool   `json:"mkdir,omitempty"`
	// Argv is the program, looked up on Env's PATH, and its arguments.
	Argv []string `json:"argv,omitempty"`
	// Env is the whole environment, except HOME, USER and LOGNAME, which come
	// from the user the step runs as.
	Env []string `json:"env,omitempty"`
	// Script is a bash script that bash reads from ScriptPath; Argv's last
	// argument should name it. The deploy user needs no file it can read.
	Script string `json:"script,omitempty"`
	// Check, when set, runs nothing: run-step prints what keeps the user from
	// cloning into this folder, or nothing when it may.
	Check string `json:"check,omitempty"`
	Umask int    `json:"umask"`
}

// Runner starts steps as User.
type Runner struct {
	// User runs the steps; empty means degoploy's own user.
	User string
	// Exe is the degoploy binary; empty means the running one.
	Exe string
}

// Isolated reports whether steps run as another user than degoploy's own.
func (r *Runner) Isolated() bool {
	return r != nil && r.User != "" && r.User != currentUser()
}

// Validate checks that the deploy user exists.
func (r *Runner) Validate() error {
	if !r.Isolated() {
		return nil
	}
	if _, err := user.Lookup(r.User); err != nil {
		return fmt.Errorf("deploy_user %s does not exist; run: sudo degoploy setup", r.User)
	}
	return nil
}

// GID is the deploy user's primary group, which files lent to a step (the SSH
// agent socket) belong to; -1 when steps run as degoploy's own user.
func (r *Runner) GID() (int, error) {
	if !r.Isolated() {
		return -1, nil
	}
	u, err := user.Lookup(r.User)
	if err != nil {
		return -1, err
	}
	return strconv.Atoi(u.Gid)
}

// Command returns the command that runs spec. The caller sets its output and
// process group.
func (r *Runner) Command(ctx context.Context, spec Spec) (*exec.Cmd, error) {
	exe := r.Exe
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return nil, err
		}
		// After degoploy update replaced the binary, until the restart.
		exe = strings.TrimSuffix(exe, " (deleted)")
	}
	spec.Umask = currentUmask()
	data, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}

	var cmd *exec.Cmd
	switch {
	case !r.Isolated():
		cmd = exec.CommandContext(ctx, exe, Subcommand)
	case os.Geteuid() == 0:
		cred, err := credential(r.User)
		if err != nil {
			return nil, err
		}
		cmd = exec.CommandContext(ctx, exe, Subcommand)
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: cred}
	default:
		// -n: fail at once instead of asking for a password.
		cmd = exec.CommandContext(ctx, "sudo", "-n", "-u", r.User, "--", exe, Subcommand)
	}
	cmd.Stdin = bytes.NewReader(data)
	// degoploy's own working directory (its data) is closed to the deploy
	// user; run-step changes to spec.Dir itself.
	cmd.Dir = "/"
	return cmd, nil
}

// CheckWorkDir reports what keeps the deploy user from cloning into dir, or
// "" when it may. Steps run as another user also need degoploy to read dir:
// it picks runtimes from the repository's files and finds its script there.
func (r *Runner) CheckWorkDir(ctx context.Context, dir string) (string, error) {
	cmd, err := r.Command(ctx, Spec{Check: dir})
	if err != nil {
		return "", err
	}
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("check %s as %s: %w: %s", dir, r.User, err, strings.TrimSpace(stderr.String()))
	}
	if msg := strings.TrimSpace(out.String()); msg != "" || !r.Isolated() {
		return msg, nil
	}
	return readable(dir), nil
}

// readable reports what keeps this process from reading dir, or from reaching
// it when it does not exist yet.
func readable(dir string) string {
	me := currentUser()
	path, mode := dir, uint32(unix.R_OK|unix.X_OK)
	for {
		if _, err := os.Stat(path); err == nil || !errors.Is(err, fs.ErrNotExist) {
			break
		}
		parent := filepath.Dir(path)
		if parent == path {
			return ""
		}
		path, mode = parent, unix.X_OK
	}
	if unix.Access(path, mode) == nil {
		return ""
	}
	return fmt.Sprintf("%s cannot read %s, which it needs to pick runtimes and find the repository's script. Give it access with: sudo setfacl -R -m u:%s:rX -m d:u:%s:rX %s", me, path, me, me, path)
}

func credential(name string) (*syscall.Credential, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return nil, err
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	cred := &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
	ids, _ := u.GroupIds()
	for _, id := range ids {
		if g, err := strconv.Atoi(id); err == nil {
			cred.Groups = append(cred.Groups, uint32(g))
		}
	}
	return cred, nil
}

var currentUser = sync.OnceValue(func() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
})

// currentUmask reads this process's umask without changing it.
func currentUmask() int {
	if b, err := os.ReadFile("/proc/self/status"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(line, "Umask:"); ok {
				if n, err := strconv.ParseInt(strings.TrimSpace(v), 8, 32); err == nil {
					return int(n)
				}
			}
		}
	}
	return 0o022
}
