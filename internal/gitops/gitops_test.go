package gitops

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	if got := shellQuote("/data/it's here"); got != `'/data/it'\''s here'` {
		t.Fatalf("shellQuote = %s", got)
	}
}

func TestLsRemoteReportsSSHFailure(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	// Nothing listens on port 1, so ssh fails fast and git's stderr must surface.
	_, err := LsRemote(context.Background(), Env{KnownHosts: t.TempDir() + "/known_hosts"}, "ssh://git@127.0.0.1:1/acme/app.git")
	if err == nil || !strings.Contains(err.Error(), "Connection refused") {
		t.Fatalf("err = %v", err)
	}
}

func TestLsRemoteBlocksFileProtocol(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	_, err := LsRemote(context.Background(), Env{}, "file:///tmp")
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("file:// must be blocked by GIT_ALLOW_PROTOCOL, got %v", err)
	}
}
