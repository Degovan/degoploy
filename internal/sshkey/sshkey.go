// Package sshkey generates and reads the per-project ed25519 deploy keys.
package sshkey

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/degovan/degoploy/internal/fsutil"
)

const (
	PrivateName = "id_ed25519"
	PublicName  = "id_ed25519.pub"
)

// Generate creates a fresh key pair in dir, replacing any existing one atomically.
func Generate(dir, comment string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	block, err := ssh.MarshalPrivateKey(priv, comment)
	if err != nil {
		return err
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return err
	}
	authorized := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))) + " " + comment + "\n"

	if err := fsutil.WriteFileAtomic(filepath.Join(dir, PrivateName), pem.EncodeToMemory(block), 0o600); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(filepath.Join(dir, PublicName), []byte(authorized), 0o644)
}

// Public returns the authorized_keys line and its SHA256 fingerprint.
func Public(dir string) (authorized, fingerprint string, err error) {
	raw, err := os.ReadFile(filepath.Join(dir, PublicName))
	if err != nil {
		return "", "", err
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey(raw)
	if err != nil {
		return "", "", fmt.Errorf("parse public key: %w", err)
	}
	return strings.TrimSpace(string(raw)), ssh.FingerprintSHA256(key), nil
}
