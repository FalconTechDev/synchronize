package syn

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Conflict struct {
	Repo         string    `json:"repo"`
	Branch       string    `json:"branch"`
	BackupBranch string    `json:"backup_branch"`
	Files        []string  `json:"files"`
	Reason       string    `json:"reason"`
	At           time.Time `json:"at"`
}

type State struct {
	Conflicts map[string]Conflict  `json:"conflicts"` 
	LastSync  map[string]time.Time `json:"last_sync"`
}

func statePath() string { return filepath.Join(Home(), "state.json") }

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
