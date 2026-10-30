package gitops

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// Agent lends one deploy key to git and the deploy script for the length of a
// deployment, through an SSH agent socket, so they never read the key file in
// degoploy's data directory. The agent only signs: it cannot hand out the key,
// and nobody can add or remove keys.
//
// Its folder holds the socket, the public key that picks it, and a copy of the
// shared known_hosts; Close adds the hosts ssh learned to the shared file.
type Agent struct {
	dir        string
	ln         net.Listener
	knownHosts string
	env        Env
}

// StartAgent serves the private key at keyPath. gid, unless -1, is the group of
// the user git and the script run as: the agent's files belong to it, so this
// process must be a member.
func StartAgent(keyPath, knownHosts string, gid int) (*Agent, error) {
	raw, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	key, err := ssh.ParseRawPrivateKey(raw)
	if err != nil {
		return nil, fmt.Errorf("deploy key: %w", err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		return nil, fmt.Errorf("deploy key: %w", err)
	}
	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: key}); err != nil {
		return nil, err
	}

	dir, err := os.MkdirTemp("", "degoploy-ssh-")
	if err != nil {
		return nil, err
	}
	a := &Agent{dir: dir, knownHosts: knownHosts}
	fail := func(err error) (*Agent, error) {
		a.Close()
		return nil, err
	}
	// Owner and group only; the group (the deploy user) may not list it.
	if err := share(dir, 0o710, gid); err != nil {
		return fail(err)
	}
	pub := filepath.Join(dir, "key.pub")
	if err := os.WriteFile(pub, ssh.MarshalAuthorizedKey(signer.PublicKey()), 0o640); err != nil {
		return fail(err)
	}
	hosts := filepath.Join(dir, "known_hosts")
	known, err := os.ReadFile(knownHosts)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fail(err)
	}
	if err := os.WriteFile(hosts, known, 0o660); err != nil {
		return fail(err)
	}
	sock := filepath.Join(dir, "agent.sock")
	if a.ln, err = net.Listen("unix", sock); err != nil {
		return fail(err)
	}
	for _, f := range []struct {
		path string
		mode os.FileMode
	}{{pub, 0o640}, {hosts, 0o660}, {sock, 0o660}} {
		if err := share(f.path, f.mode, gid); err != nil {
			return fail(err)
		}
	}

	go func() {
		for {
			c, err := a.ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				agent.ServeAgent(signOnly{keyring}, c)
			}()
		}
	}()
	a.env = Env{KnownHosts: hosts, AgentSocket: sock, PublicKey: pub}
	return a, nil
}

// Env is the git/ssh setup that uses the agent.
func (a *Agent) Env() Env { return a.env }

// Close stops the agent, keeps the hosts ssh learned and removes its folder.
func (a *Agent) Close() error {
	if a.ln != nil {
		a.ln.Close()
	}
	var err error
	if a.env.KnownHosts != "" {
		err = mergeKnownHosts(a.knownHosts, a.env.KnownHosts)
	}
	return errors.Join(err, os.RemoveAll(a.dir))
}

// share sets path's mode and hands it to group gid (-1 keeps the group).
func share(path string, mode os.FileMode, gid int) error {
	if gid >= 0 {
		if err := os.Chown(path, -1, gid); err != nil {
			return fmt.Errorf("give %s to the deploy user's group: %w (is degoploy a member? run: sudo degoploy setup)", path, err)
		}
	}
	return os.Chmod(path, mode)
}

// mergeKnownHosts appends the lines of from that dst does not have yet.
func mergeKnownHosts(dst, from string) error {
	learned, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	have, err := os.ReadFile(dst)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(have), "\n") {
		seen[strings.TrimSpace(line)] = true
	}
	var add bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(learned))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" && !strings.HasPrefix(line, "#") && !seen[line] {
			seen[line] = true
			add.WriteString(line + "\n")
		}
	}
	if add.Len() == 0 {
		return nil
	}
	f, err := os.OpenFile(dst, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if len(have) > 0 && !bytes.HasSuffix(have, []byte("\n")) {
		f.WriteString("\n")
	}
	_, err = f.Write(add.Bytes())
	return errors.Join(err, f.Close())
}

var errSignOnly = errors.New("degoploy's agent only signs with the project's deploy key")

// signOnly is an agent that lists and signs but changes nothing.
type signOnly struct{ agent.Agent }

func (signOnly) Add(agent.AddedKey) error   { return errSignOnly }
func (signOnly) Remove(ssh.PublicKey) error { return errSignOnly }
func (signOnly) RemoveAll() error           { return errSignOnly }
func (signOnly) Lock([]byte) error          { return errSignOnly }
func (signOnly) Unlock([]byte) error        { return errSignOnly }
