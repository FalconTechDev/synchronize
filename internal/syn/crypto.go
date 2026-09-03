package syn

// Encryption for syn repos using git clean/smudge filters.
//
// How it works:
//   - A 32-byte key is generated randomly and stored in
//     ~/.synchronize/keys/<repo-name>.key (mode 0600, never committed).
//   - git clean filter  (pre-commit): encrypts file bytes with AES-256-GCM.
//   - git smudge filter (post-checkout): decrypts them.
//   - The filter is registered via `git config` in the repo's .git/config
//     and applied to all files via a .gitattributes rule.
//
// The encrypted format is:
//   [4 bytes magic "SYN\x01"] [12 bytes nonce] [ciphertext]

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const cryptoMagic = "SYN\x01"

// keyPath returns the path to the key file for a repo name.
func keyPath(repoName string) string {
	return filepath.Join(Home(), "keys", repoName+".key")
}

// GenerateKey creates a new random 32-byte key and saves it.
func GenerateKey(repoName string) ([]byte, error) {
	dir := filepath.Join(Home(), "keys")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	path := keyPath(repoName)
	if err := os.WriteFile(path, []byte(hex.EncodeToString(key)+"\n"), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

// LoadKey reads the stored key for repoName.
func LoadKey(repoName string) ([]byte, error) {
	path := keyPath(repoName)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no encryption key for %q (run `syn repo add --encrypt` or `syn encrypt init %s`)", repoName, repoName)
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("corrupt key file at %s", path)
	}
	return key, nil
}

// ExportKey returns the hex-encoded key so the user can share it with another machine.
func ExportKey(repoName string) (string, error) {
	key, err := LoadKey(repoName)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}

// ImportKey decodes a hex key string and saves it for repoName.
func ImportKey(repoName, hexKey string) error {
	key, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil || len(key) != 32 {
		return fmt.Errorf("invalid key: must be 64 hex characters (32 bytes)")
	}
	dir := filepath.Join(Home(), "keys")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(keyPath(repoName), []byte(hexKey+"\n"), 0o600)
}

// Encrypt reads plaintext from r, encrypts with key, writes ciphertext to w.
func Encrypt(w io.Writer, r io.Reader, key []byte) error {
	plain, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	ct := gcm.Seal(nil, nonce, plain, nil)
	_, err = w.Write(append([]byte(cryptoMagic), append(nonce, ct...)...))
	return err
}

// Decrypt reads ciphertext from r, decrypts with key, writes plaintext to w.
// If r does not start with the magic bytes it is passed through unchanged
// (allows mixing encrypted and plain files in the same repo).
func Decrypt(w io.Writer, r io.Reader, key []byte) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if len(data) < len(cryptoMagic) || string(data[:len(cryptoMagic)]) != cryptoMagic {
		_, err = w.Write(data)
		return err
	}
	data = data[len(cryptoMagic):]
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return errors.New("ciphertext too short")
	}
	plain, err := gcm.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return fmt.Errorf("decryption failed (wrong key?): %w", err)
	}
	_, err = w.Write(plain)
	return err
}

// SetupEncryption installs the git clean/smudge filters in a repo and adds
// a catch-all .gitattributes rule. Call once after cloning/creating the repo.
func SetupEncryption(repoPath, repoName string) error {
	synBin, err := os.Executable()
	if err != nil {
		return err
	}
	cmds := [][]string{
		{"config", "filter.syn-crypt.clean", synBin + " encrypt clean " + repoName},
		{"config", "filter.syn-crypt.smudge", synBin + " encrypt smudge " + repoName},
		{"config", "filter.syn-crypt.required", "true"},
	}
	for _, args := range cmds {
		if _, err := git(repoPath, args...); err != nil {
			return err
		}
	}
	// Add .gitattributes if it doesn't already have our rule.
	gaPath := filepath.Join(repoPath, ".gitattributes")
	rule := "* filter=syn-crypt\n"
	existing, _ := os.ReadFile(gaPath)
	if !strings.Contains(string(existing), "filter=syn-crypt") {
		f, err := os.OpenFile(gaPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		_, err = f.WriteString(rule)
		f.Close()
		if err != nil {
			return err
		}
		git(repoPath, "add", ".gitattributes")
	}
	return nil
}
