package syn

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Credentials holds GitHub auth tokens, keyed by hostname (normally "github.com").
type Credentials struct {
	Tokens map[string]string `json:"tokens"` // hostname -> PAT / OAuth token
}

func credsPath() string { return filepath.Join(Home(), "credentials.json") }

// LoadCredentials reads the credentials file, returning empty creds if absent.
func LoadCredentials() *Credentials {
	c := &Credentials{Tokens: map[string]string{}}
	b, err := os.ReadFile(credsPath())
	if errors.Is(err, os.ErrNotExist) || err != nil {
		return c
	}
	_ = json.Unmarshal(b, c)
	if c.Tokens == nil {
		c.Tokens = map[string]string{}
	}
	return c
}

// Save writes credentials to disk with restricted permissions.
func (c *Credentials) Save() error {
	if err := os.MkdirAll(Home(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := credsPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, credsPath())
}

// Token returns the stored token for host, or "".
func (c *Credentials) Token(host string) string {
	if c == nil {
		return ""
	}
	return c.Tokens[host]
}

// Set stores (or clears, if token is "") the token for host.
func (c *Credentials) Set(host, token string) {
	if token == "" {
		delete(c.Tokens, host)
	} else {
		c.Tokens[host] = token
	}
}
