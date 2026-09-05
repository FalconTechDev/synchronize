package syn

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Repo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Remote  string `json:"remote"`
	Branch  string `json:"branch"`
	Enabled bool   `json:"enabled"`

	GithubSlug string `json:"github_slug,omitempty"`
	Private    bool   `json:"private,omitempty"`
	Temp       bool   `json:"temp,omitempty"`

	IntervalMinutes int `json:"interval_minutes,omitempty"`

	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`

	Hooks Hooks `json:"hooks,omitempty"`

	// PausedUntil leaves this repository out of automatic syncing for a
	// while, without the "off and forgotten" of pausing it for good.
	PausedUntil time.Time `json:"paused_until,omitempty"`

	// NoCommit are paths an automatic commit steps over. The file rules say
	// what is checked out; this says what syn will not commit for you.
	NoCommit []string `json:"no_commit,omitempty"`
}

// SyncsByItself reports whether automatic syncing should touch this one.
func (r *Repo) SyncsByItself() bool {
	if !r.Enabled {
		return false
	}
	return r.PausedUntil.IsZero() || time.Now().After(r.PausedUntil)
}

type Hooks struct {
	AfterPull      []string `json:"after_pull,omitempty"`
	AfterPush      []string `json:"after_push,omitempty"`
	OnConflict     []string `json:"on_conflict,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
}

func (h Hooks) Timeout() int {
	if h.TimeoutSeconds > 0 {
		return h.TimeoutSeconds
	}
	return 300
}

func (h Hooks) IsEmpty() bool {
	return len(h.AfterPull) == 0 && len(h.AfterPush) == 0 && len(h.OnConflict) == 0
}

type Telegram struct {
	Token  string `json:"token"`
	ChatID string `json:"chat_id"`
}

type Config struct {
	Repos           []Repo   `json:"repos"`
	Telegram        Telegram `json:"telegram"`
	Backup          bool     `json:"backup"`
	PushBackup      bool     `json:"push_backup"`
	Strategy        string   `json:"strategy"`
	IntervalMinutes int      `json:"interval_minutes"`
	Notify          string   `json:"notify"`
	CommitTemplate  string   `json:"commit_template"`
	KeepBackupDays  int      `json:"keep_backup_days"`

	GitHubClientID string `json:"github_client_id,omitempty"`

	NotifySync bool `json:"notify_sync,omitempty"`

	ServeAddr string `json:"serve_addr,omitempty"`

	Identity Identity `json:"identity,omitempty"`

	// KeepWatching decides what the login task does: keep syncing on a timer,
	// or sync once and exit.
	KeepWatching bool `json:"keep_watching,omitempty"`
}

type Identity struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

func (i Identity) IsZero() bool { return i.Name == "" || i.Email == "" }

func (c *Config) IntervalFor(r *Repo) int {
	if r.IntervalMinutes > 0 {
		return r.IntervalMinutes
	}
	return c.IntervalMinutes
}

func DefaultConfig() *Config {
	return &Config{
		Backup: true,
		// Safety branches stay on this machine. Pushing every one of them
		// turns the branch list on GitHub into a wall of syn/backup/... for
		// something only this machine will ever need to read.
		PushBackup:      false,
		Strategy:        "rebase",
		IntervalMinutes: 10,
		Notify:          "conflict",
		CommitTemplate:  "sync: auto-commit from {host} at {time}",
		KeepBackupDays:  14,
	}
}

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

func (c *Config) Save() error {
	// Hold the lock across the write so two processes cannot both replace the
	// file, and give the temporary file a name of its own - a fixed one had
	// them fighting over the same path.
	lock, err := LockConfig(10 * time.Second)
	if err != nil {
		return err
	}
	defer lock.Release()

	if err := os.MkdirAll(Home(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.tmp", configPath(), os.Getpid())
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, configPath())
}

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

// Selected turns a command-line selector into repositories. Acting on every
// repository has to be asked for by name ("all"), so a bare `syn push` can
// never quietly publish work from a repository you were not thinking about.
func (c *Config) Selected(key string) ([]*Repo, error) {
	if len(c.Repos) == 0 {
		return nil, errors.New("no repositories registered yet - run `syn init` inside a git repo")
	}

	// "all" means every repository being tracked. Pausing one only takes it
	// out of automatic syncing - asking for something by hand is never
	// refused, or work would sit here with no way out.
	if key == "all" {
		out := make([]*Repo, 0, len(c.Repos))
		for i := range c.Repos {
			out = append(out, &c.Repos[i])
		}
		return out, nil
	}

	if key == "" {
		return nil, fmt.Errorf("which repository?\n%s", c.pickHint())
	}

	r := c.Find(key)
	if r == nil {
		return nil, fmt.Errorf("unknown repository %q\n%s", key, c.pickHint())
	}
	return []*Repo{r}, nil
}

func (c *Config) pickHint() string {
	names := make([]string, 0, len(c.Repos))
	for i := range c.Repos {
		names = append(names, c.Repos[i].Name)
	}
	return fmt.Sprintf("        pick one of: %s\n        or act on all of them with: all",
		strings.Join(names, ", "))
}

// EnabledRepos are the ones automatic syncing covers. Everything else still
// answers to commands you type yourself.
func (c *Config) EnabledRepos() []*Repo {
	var out []*Repo
	for i := range c.Repos {
		if c.Repos[i].SyncsByItself() {
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
