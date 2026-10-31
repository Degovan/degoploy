package runas

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The Runner starts this test binary as "run-step".
func TestMain(m *testing.M) {
	if IsStep() {
		os.Exit(Main())
	}
	os.Exit(m.Run())
}

func runStep(t *testing.T, spec Spec) (string, error) {
	t.Helper()
	r := &Runner{}
	cmd, err := r.Command(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	return out.String(), err
}

func TestStep(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "app")
	out, err := runStep(t, Spec{
		Dir: dir, Mkdir: true,
		Argv: []string{"sh", "-c", `echo "$FOO $HOME"; pwd; umask`},
		Env:  []string{"PATH=" + os.Getenv("PATH"), "FOO=bar", "HOME=/nowhere"},
	})
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	home, _ := os.UserHomeDir()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 || lines[0] != "bar "+home || lines[1] != dir || lines[2] != fmt.Sprintf("%04o", currentUmask()) {
		t.Fatalf("output:\n%s", out)
	}
}

func TestScriptAndExitCode(t *testing.T) {
	out, err := runStep(t, Spec{
		Argv:   []string{"bash", "--noprofile", "--norc", ScriptPath},
		Env:    []string{"PATH=" + os.Getenv("PATH")},
		Script: "echo from the script\nexit 7\n",
	})
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 7 || out != "from the script\n" {
		t.Fatalf("err = %v, output %q", err, out)
	}

	if out, err := runStep(t, Spec{Argv: []string{"no-such-program-here"}, Env: []string{"PATH=/nonexistent"}}); !errors.As(err, &exit) || exit.ExitCode() != 127 || !strings.Contains(out, "no-such-program-here") {
		t.Fatalf("missing program: %v %q", err, out)
	}
}

// A SIGTERM to run-step stops the command's whole process group, including
// what the script started in the background.
func TestStopKillsGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	r := &Runner{}
	cmd, err := r.Command(context.Background(), Spec{
		Argv: []string{"sh", "-c", `trap "" TERM; sleep 300 & echo $! > ` + pidFile + `; wait`},
		Env:  []string{"PATH=" + os.Getenv("PATH")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var pid int
	for deadline := time.Now().Add(5 * time.Second); pid == 0 && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		b, _ := os.ReadFile(pidFile)
		pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	if pid == 0 {
		t.Fatal("the script did not start")
	}
	cmd.Process.Signal(syscall.SIGTERM)
	time.Sleep(100 * time.Millisecond)  // signals of one kind do not queue
	cmd.Process.Signal(syscall.SIGTERM) // the second one skips the grace period
	cmd.Wait()
	for deadline := time.Now().Add(5 * time.Second); syscall.Kill(pid, 0) == nil; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			syscall.Kill(pid, syscall.SIGKILL)
			t.Fatal("background process survived")
		}
	}
}

func TestCheckWorkDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.php"), nil, 0o644)
	r := &Runner{}
	if msg, err := r.CheckWorkDir(context.Background(), dir, false); err != nil || !strings.Contains(msg, "not a git repository") {
		t.Fatalf("msg = %q, %v", msg, err)
	}
	if msg, err := r.CheckWorkDir(context.Background(), filepath.Join(dir, "app"), false); err != nil || msg != "" {
		t.Fatalf("new folder: %q, %v", msg, err)
	}
}

func TestIsolated(t *testing.T) {
	if (&Runner{}).Isolated() || (&Runner{User: currentUser()}).Isolated() || !(&Runner{User: "someone-else"}).Isolated() {
		t.Fatal("Isolated")
	}
}
