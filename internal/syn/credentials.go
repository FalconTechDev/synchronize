package syn

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Credentials struct {
	Tokens map[string]string `json:"tokens"`
}

func credsPath() string { return filepath.Join(Home(), "credentials.json") }

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

func (c *Credentials) Token(host string) string {
	if c == nil {
		return ""
	}
	return c.Tokens[host]
}

func (c *Credentials) Delete(host string) {
	delete(c.Tokens, host)
}

func (c *Credentials) Set(host, token string) {
	if token == "" {
		delete(c.Tokens, host)
	} else {
		c.Tokens[host] = token
	}
}
