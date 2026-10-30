package runas

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/degovan/degoploy/internal/project"
)

// stopGrace is how long the command's process group gets between SIGTERM and
// SIGKILL. It is shorter than degoploy's own grace for run-step, so run-step
// is still there to send the SIGKILL.
const stopGrace = 8 * time.Second

// IsStep reports whether this process was started to run a step. Test
// binaries whose tests run steps call it from TestMain.
func IsStep() bool { return len(os.Args) > 1 && os.Args[1] == Subcommand }

// Main runs the step described on stdin and returns the exit code: the
// command's own, or 127 when it could not start.
func Main() int {
	var spec Spec
	if err := json.NewDecoder(os.Stdin).Decode(&spec); err != nil {
		fmt.Fprintf(os.Stderr, "degoploy %s: read the step: %v\n", Subcommand, err)
		return 127
	}
	if spec.Check != "" {
		if err := project.CheckWorkDirAccess(spec.Check); err != nil {
			var ve *project.ValidationError
			if errors.As(err, &ve) {
				fmt.Println(ve.Message)
			} else {
				fmt.Println(err)
			}
		}
		return 0
	}
	code, err := run(spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "degoploy: %v\n", err)
		return 127
	}
	return code
}

func run(spec Spec) (int, error) {
	if len(spec.Argv) == 0 {
		return 0, errors.New("nothing to run")
	}
	viaSudo := os.Getenv("SUDO_COMMAND") != ""
	if viaSudo {
		// degoploy may signal sudo but not this user's processes. When sudo is
		// gone, stop the command at once (see stop).
		unix.Prctl(unix.PR_SET_PDEATHSIG, uintptr(syscall.SIGHUP), 0, 0, 0)
		if os.Getppid() == 1 {
			return 0, errors.New("sudo exited before the step started")
		}
	}
	unix.Umask(spec.Umask)

	env := spec.Env
	if u, err := user.Current(); err == nil {
		env = setEnv(env, "HOME", u.HomeDir)
		env = setEnv(env, "USER", u.Username)
		env = setEnv(env, "LOGNAME", u.Username)
	}
	if spec.Mkdir {
		if err := os.MkdirAll(spec.Dir, 0o775); err != nil {
			return 0, err
		}
	}
	path, err := lookPath(spec.Argv[0], env)
	if err != nil {
		return 0, err
	}

	cmd := exec.Command(path, spec.Argv[1:]...)
	cmd.Args[0] = spec.Argv[0]
	cmd.Dir, cmd.Env = spec.Dir, env
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if spec.Script != "" {
		script, err := memFile("deploy.sh", spec.Script)
		if err != nil {
			return 0, err
		}
		cmd.ExtraFiles = []*os.File{script} // fd 3, ScriptPath
	}

	signals := make(chan os.Signal, 4)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	done := make(chan struct{})
	go stop(cmd.Process.Pid, signals, done)
	err = cmd.Wait()
	close(done)

	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0, nil
	case errors.As(err, &exit):
		if ws, ok := exit.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal()), nil
		}
		return exit.ExitCode(), nil
	default:
		return 0, err
	}
}

// stop passes a cancellation on to the command's process group: SIGTERM, then
// SIGKILL after stopGrace. SIGHUP (sudo is gone) kills it at once.
func stop(pid int, signals <-chan os.Signal, done <-chan struct{}) {
	select {
	case sig := <-signals:
		if sig == syscall.SIGHUP {
			syscall.Kill(-pid, syscall.SIGKILL)
			return
		}
		syscall.Kill(-pid, syscall.SIGTERM)
	case <-done:
		return
	}
	// Any further signal means not to wait any longer.
	select {
	case <-time.After(stopGrace):
	case <-signals:
	case <-done:
		return
	}
	syscall.Kill(-pid, syscall.SIGKILL)
}

// memFile returns an in-memory file holding content.
func memFile(name, content string) (*os.File, error) {
	fd, err := unix.MemfdCreate(name, unix.MFD_CLOEXEC)
	if err != nil {
		return nil, fmt.Errorf("memfd: %w", err)
	}
	f := os.NewFile(uintptr(fd), name)
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// lookPath finds name on env's PATH.
func lookPath(name string, env []string) (string, error) {
	if strings.Contains(name, "/") {
		return name, nil
	}
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			os.Setenv("PATH", v)
		}
	}
	return exec.LookPath(name)
}

func setEnv(env []string, key, value string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}
	return append(out, key+"="+value)
}
