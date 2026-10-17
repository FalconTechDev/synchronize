package syn

import (
	"encoding/json"
	"fmt"
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

	// LastPrune keeps the automatic clear-out to once a day per repository.
	LastPrune map[string]time.Time `json:"last_prune,omitempty"`

	// LastNag keeps the reminder about an unresolved conflict to once a
	// day, so it is noticed rather than tuned out.
	LastNag map[string]time.Time `json:"last_nag,omitempty"`
}

func statePath() string { return filepath.Join(Home(), "state.json") }

func LoadState() *State {
	st := &State{}
	if b, err := os.ReadFile(statePath()); err == nil {
		_ = json.Unmarshal(b, st)
	}
	st.fillMaps()
	return st
}

// fillMaps leaves every map ready to be written to.
//
// Each of these is omitted from the file when empty, so it comes back nil
// from any state file written before that field existed - and writing to a
// nil map panics. Doing it in one place, on every path out of LoadState,
// means adding a field later cannot reintroduce that: twice now a new map
// has been added and the initialisation missed on one path.
func (s *State) fillMaps() {
	if s.Conflicts == nil {
		s.Conflicts = map[string]Conflict{}
	}
	if s.LastSync == nil {
		s.LastSync = map[string]time.Time{}
	}
	if s.LastPrune == nil {
		s.LastPrune = map[string]time.Time{}
	}
	if s.LastNag == nil {
		s.LastNag = map[string]time.Time{}
	}
}

func (s *State) Save() error {
	if err := os.MkdirAll(Home(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.tmp", statePath(), os.Getpid())
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, statePath())
}

// ForgetRepoState drops everything remembered about a repository. Called when
// one stops being tracked: a conflict left on record would otherwise attach
// itself to whatever is registered under that name next, and syncing refuses
// to run while a conflict is on record.
func ForgetRepoState(name string) {
	st := LoadState()
	delete(st.Conflicts, name)
	delete(st.LastSync, name)
	delete(st.LastPrune, name)
	delete(st.LastNag, name)
	if err := st.Save(); err != nil {
		Warn("could not clear the saved state for %q: %v", name, err)
	}
}

// RenameRepoState carries the record across a rename, so a conflict is not
// quietly forgotten by giving the repository another name.
func RenameRepoState(from, to string) {
	if from == to {
		return
	}
	st := LoadState()
	if c, ok := st.Conflicts[from]; ok {
		c.Repo = to
		st.Conflicts[to] = c
		delete(st.Conflicts, from)
	}
	if v, ok := st.LastSync[from]; ok {
		st.LastSync[to] = v
		delete(st.LastSync, from)
	}
	if v, ok := st.LastPrune[from]; ok {
		st.LastPrune[to] = v
		delete(st.LastPrune, from)
	}
	if err := st.Save(); err != nil {
		Warn("could not move the saved state from %q to %q: %v", from, to, err)
	}
}
