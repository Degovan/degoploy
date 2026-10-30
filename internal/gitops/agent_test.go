package gitops

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh/agent"

	"github.com/degovan/degoploy/internal/sshkey"
)

func TestAgent(t *testing.T) {
	dir := t.TempDir()
	if err := sshkey.Generate(dir, "degoploy-test"); err != nil {
		t.Fatal(err)
	}
	shared := filepath.Join(dir, "known_hosts")
	os.WriteFile(shared, []byte("old.example.com ssh-ed25519 AAAA1"), 0o600)

	a, err := StartAgent(filepath.Join(dir, sshkey.PrivateName), shared, -1)
	if err != nil {
		t.Fatal(err)
	}
	env := a.Env()
	if line := strings.Join(env.Environ(), "\n"); !strings.Contains(line, "IdentityAgent=") || strings.Contains(line, sshkey.PrivateName) {
		t.Fatalf("environment should point at the agent, not the key file:\n%s", line)
	}

	conn, err := net.Dial("unix", env.AgentSocket)
	if err != nil {
		t.Fatal(err)
	}
	client := agent.NewClient(conn)
	keys, err := client.List()
	if err != nil || len(keys) != 1 {
		t.Fatalf("keys = %v, %v", keys, err)
	}
	pub, _ := os.ReadFile(env.PublicKey)
	if !bytes.HasPrefix(pub, []byte(keys[0].Type())) {
		t.Fatalf("public key file %q does not match %s", pub, keys[0].Type())
	}
	if _, err := client.Sign(keys[0], []byte("data")); err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := client.RemoveAll(); err == nil {
		t.Fatal("the agent must not let anyone remove the key")
	}
	if keys, _ := client.List(); len(keys) != 1 {
		t.Fatal("key removed")
	}
	conn.Close()

	// Hosts ssh learned during the deployment end up in the shared file.
	f, _ := os.OpenFile(env.KnownHosts, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("\nnew.example.com ssh-ed25519 AAAA2\nold.example.com ssh-ed25519 AAAA1\n")
	f.Close()
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(shared); string(got) != "old.example.com ssh-ed25519 AAAA1\nnew.example.com ssh-ed25519 AAAA2\n" {
		t.Fatalf("shared known_hosts = %q", got)
	}
	if _, err := os.Stat(filepath.Dir(env.AgentSocket)); !os.IsNotExist(err) {
		t.Fatalf("agent folder left behind: %v", err)
	}
}
