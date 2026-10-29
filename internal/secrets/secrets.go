// Package secrets encrypts values at rest with AES-256-GCM.
//
// The key is 32 random bytes, given base64-encoded in the config
// (secret_key / DEGOPLOY_SECRET_KEY) or kept in a key file that is created on
// first use. Losing the key makes every stored secret unreadable.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const keySize = 32

// sealedPrefix versions the ciphertext format.
const sealedPrefix = "v1:"

// Box seals and opens values with one key.
type Box struct {
	aead   cipher.AEAD
	macKey []byte
}

func New(key []byte) (*Box, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("secret key must be %d bytes, got %d", keySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("degoploy mac key"))
	return &Box{aead: aead, macKey: mac.Sum(nil)}, nil
}

// MAC returns a keyed fingerprint of data, for telling whether settings that
// include a secret have changed without storing anything derived from the
// secret alone.
func (b *Box) MAC(scope string, data ...string) string {
	mac := hmac.New(sha256.New, b.macKey)
	mac.Write([]byte(scope))
	for _, d := range data {
		mac.Write([]byte{0})
		mac.Write([]byte(d))
	}
	return hex.EncodeToString(mac.Sum(nil))
}

// LoadKey decodes encoded when it is set; otherwise it reads the key file at
// path, creating it with a fresh random key if it does not exist yet.
func LoadKey(encoded, path string) ([]byte, error) {
	if encoded != "" {
		key, err := decodeKey(encoded)
		if err != nil {
			return nil, fmt.Errorf("secret_key: %w", err)
		}
		return key, nil
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return createKeyFile(path)
	}
	if err != nil {
		return nil, err
	}
	key, err := decodeKey(string(raw))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return key, nil
}

func decodeKey(s string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("not valid base64: %w", err)
	}
	if len(key) != keySize {
		return nil, fmt.Errorf("must decode to %d bytes, got %d", keySize, len(key))
	}
	return key, nil
}

func createKeyFile(path string) ([]byte, error) {
	key := make([]byte, keySize)
	rand.Read(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	// O_EXCL: if another process created it meanwhile, use theirs.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return LoadKey("", path)
	}
	if err != nil {
		return nil, err
	}
	_, werr := f.WriteString(base64.StdEncoding.EncodeToString(key) + "\n")
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(path)
		return nil, werr
	}
	return key, nil
}

// Seal encrypts plaintext. scope is authenticated but not stored: the same
// scope must be passed to Open, which stops a sealed value from being moved
// to another owner (e.g. another project).
func (b *Box) Seal(plaintext, scope string) string {
	nonce := make([]byte, b.aead.NonceSize())
	rand.Read(nonce)
	sealed := b.aead.Seal(nonce, nonce, []byte(plaintext), []byte(scope))
	return sealedPrefix + base64.StdEncoding.EncodeToString(sealed)
}

// Open decrypts a value from Seal.
func (b *Box) Open(sealed, scope string) (string, error) {
	enc, ok := strings.CutPrefix(sealed, sealedPrefix)
	if !ok {
		return "", errors.New("unknown secret format")
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", fmt.Errorf("corrupt secret: %w", err)
	}
	n := b.aead.NonceSize()
	if len(raw) < n {
		return "", errors.New("corrupt secret: too short")
	}
	plain, err := b.aead.Open(nil, raw[:n], raw[n:], []byte(scope))
	if err != nil {
		return "", errors.New("cannot decrypt secret: wrong key or corrupt value")
	}
	return string(plain), nil
}

// ProjectScope is the Seal scope of a project's secret variables.
func ProjectScope(projectID int64) string { return "project/" + strconv.FormatInt(projectID, 10) }
