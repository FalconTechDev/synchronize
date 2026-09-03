package syn

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Conflict records a sync that stopped because git could not merge cleanly.
// It survives restarts so `syn status` and `syn resolve` can pick it up later.
type Conflict struct {
	Repo         string    `json:"repo"`
	Branch       string    `json:"branch"`
	BackupBranch string    `json:"backup_branch"`
	Files        []string  `json:"files"`
	Reason       string    `json:"reason"`
	At           time.Time `json:"at"`
}

// State is the mutable runtime record kept next to the config.
type State struct {
	Conflicts map[string]Conflict  `json:"conflicts"` // keyed by repo name
	LastSync  map[string]time.Time `json:"last_sync"`
}

func statePath() string { return filepath.Join(Home(), "state.json") }

// LoadState reads state.json, returning an empty state if it does not exist.
func LoadState() *State {
	st := &State{Conflicts: map[string]Conflict{}, LastSync: map[string]time.Time{}}
	b, err := os.ReadFile(statePath())
	if errors.Is(err, os.ErrNotExist) || err != nil {
		return st
	}
	_ = json.Unmarshal(b, st)
	if st.Conflicts == nil {
		st.Conflicts = map[string]Conflict{}
	}
	if st.LastSync == nil {
		st.LastSync = map[string]time.Time{}
	}
	return st
}

// Save writes the state back to disk atomically.
func (s *State) Save() error {
	if err := os.MkdirAll(Home(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := statePath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, statePath())
}
