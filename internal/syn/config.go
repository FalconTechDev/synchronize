package syn

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Repo is a single repository tracked by synchronize.
type Repo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Remote  string `json:"remote"`
	Branch  string `json:"branch"` // empty = whatever branch is checked out
	Enabled bool   `json:"enabled"`

	// GitHub-specific (optional).
	GithubSlug string `json:"github_slug,omitempty"` // "owner/repo"
	Private    bool   `json:"private,omitempty"`
	Temp       bool   `json:"temp,omitempty"` // created by syn tmp; deleted on drop

	// Per-repo overrides (0/empty = use global).
	IntervalMinutes int `json:"interval_minutes,omitempty"`

	// File-level rules. Both support standard glob patterns.
	// Empty Include + empty Exclude = sync everything (normal git behaviour).
	Include []string `json:"include,omitempty"` // only these paths
	Exclude []string `json:"exclude,omitempty"` // never these paths
}

// Telegram holds optional notification credentials.
type Telegram struct {
	Token  string `json:"token"`
	ChatID string `json:"chat_id"`
}

// Config is the on-disk configuration, shared by the CLI and the daemon.
type Config struct {
	Repos           []Repo   `json:"repos"`
	Telegram        Telegram `json:"telegram"`
	Backup          bool     `json:"backup"`           // create syn/backup/... branches before rewriting history
	PushBackup      bool     `json:"push_backup"`      // also push those branches to the remote
	Strategy        string   `json:"strategy"`         // "rebase" or "merge"
	IntervalMinutes int      `json:"interval_minutes"` // global watch loop period
	Notify          string   `json:"notify"`           // "never" | "conflict" | "always"
	CommitTemplate  string   `json:"commit_template"`  // {host} and {time} are substituted
	KeepBackupDays  int      `json:"keep_backup_days"` // prune local backup branches older than this

	// GitHub OAuth App client_id. Set via `syn auth setup <client_id>`.
	// Leave empty to use PAT-only login.
	GitHubClientID string `json:"github_client_id,omitempty"`

	// NotifySync sends OS toast on every successful sync (not just conflicts).
	NotifySync bool `json:"notify_sync,omitempty"`

	// ServeAddr is the listen address for `syn serve` (default "127.0.0.1:7391").
	ServeAddr string `json:"serve_addr,omitempty"`
}

// IntervalFor returns the sync interval for r, falling back to the global one.
func (c *Config) IntervalFor(r *Repo) int {
	if r.IntervalMinutes > 0 {
		return r.IntervalMinutes
	}
	return c.IntervalMinutes
}

// DefaultConfig returns the configuration used before the user touches anything.
func DefaultConfig() *Config {
	return &Config{
		Backup:          true,
		PushBackup:      true,
		Strategy:        "rebase",
		IntervalMinutes: 10,
		Notify:          "conflict",
		CommitTemplate:  "sync: auto-commit from {host} at {time}",
		KeepBackupDays:  14,
	}
}

// Home is the directory holding config, state and logs.
func Home() string {
	if v := os.Getenv("SYNCHRONIZE_HOME"); v != "" {
		return v
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ".synchronize"
	}
	return filepath.Join(h, ".synchronize")
}

func configPath() string { return filepath.Join(Home(), "config.json") }

// LoadConfig reads the config, filling in defaults for anything missing.
func LoadConfig() (*Config, error) {
	cfg := DefaultConfig()
	b, err := os.ReadFile(configPath())
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("config.json is not valid JSON: %w", err)
	}
	if cfg.Strategy == "" {
		cfg.Strategy = "rebase"
	}
	if cfg.IntervalMinutes <= 0 {
		cfg.IntervalMinutes = 10
	}
	if cfg.Notify == "" {
		cfg.Notify = "conflict"
	}
	if cfg.CommitTemplate == "" {
		cfg.CommitTemplate = DefaultConfig().CommitTemplate
	}
	return cfg, nil
}

// Save writes the config back to disk atomically.
func (c *Config) Save() error {
	if err := os.MkdirAll(Home(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := configPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, configPath())
}

// Find returns the repo matching name or path, or nil.
func (c *Config) Find(key string) *Repo {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	abs, _ := filepath.Abs(key)
	for i := range c.Repos {
		r := &c.Repos[i]
		if strings.EqualFold(r.Name, key) {
			return r
		}
		if abs != "" && samePath(r.Path, abs) {
			return r
		}
	}
	return nil
}

// Selected resolves the repos a command should act on. An empty key means
// "every enabled repo".
func (c *Config) Selected(key string) ([]*Repo, error) {
	if key == "" {
		var out []*Repo
		for i := range c.Repos {
			if c.Repos[i].Enabled {
				out = append(out, &c.Repos[i])
			}
		}
		if len(out) == 0 {
			return nil, errors.New("no repositories registered yet - run `syn init` inside a git repo")
		}
		return out, nil
	}
	r := c.Find(key)
	if r == nil {
		return nil, fmt.Errorf("unknown repository %q", key)
	}
	return []*Repo{r}, nil
}

// EnabledRepos returns pointers to all enabled repos (used by the tray).
func (c *Config) EnabledRepos() []*Repo {
	var out []*Repo
	for i := range c.Repos {
		if c.Repos[i].Enabled {
			out = append(out, &c.Repos[i])
		}
	}
	return out
}

func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
