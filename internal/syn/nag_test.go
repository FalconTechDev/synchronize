package syn

import (
	"os"
	"testing"
	"time"
)

// Stopping on a conflict has a failure mode of its own: the notice is seen
// once, and then the repository quietly does not sync for a week. The
// reminder has to come back - but once a day, not every ten minutes, or it
// becomes the thing you learn to ignore.
func TestAStaleConflictIsRaisedAgainOnceADay(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SYNCHRONIZE_HOME", dir)
	Quiet = true

	cfg := DefaultConfig()
	e := &Engine{Cfg: cfg, State: LoadState(), Notifier: NewNotifier(cfg)}
	r := &Repo{Name: "bot", Enabled: true}

	fresh := Conflict{Repo: "bot", Files: []string{"a.txt"}, At: time.Now().Add(-time.Hour)}
	e.nagAboutConflict(r, fresh)
	if _, nagged := e.State.LastNag["bot"]; nagged {
		t.Error("complained about a conflict that is an hour old; the first notice covered that")
	}

	old := Conflict{Repo: "bot", Files: []string{"a.txt"}, At: time.Now().Add(-48 * time.Hour)}
	e.nagAboutConflict(r, old)
	first, nagged := e.State.LastNag["bot"]
	if !nagged {
		t.Fatal("a conflict two days old went unmentioned")
	}

	// The next tick, ten minutes later, must stay quiet.
	e.nagAboutConflict(r, old)
	if again := e.State.LastNag["bot"]; !again.Equal(first) {
		t.Error("complained twice in a row; it would be tuned out within the hour")
	}

	// A day later it is worth saying again.
	e.State.LastNag["bot"] = time.Now().Add(-25 * time.Hour)
	e.nagAboutConflict(r, old)
	if again := e.State.LastNag["bot"]; !again.After(first) {
		t.Error("went silent for good after one reminder")
	}
}

// The reminder is kept per repository and must go when the repository does.
func TestForgettingARepositoryForgetsItsReminder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SYNCHRONIZE_HOME", dir)

	st := LoadState()
	st.LastNag["bot"] = time.Now()
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	ForgetRepoState("bot")
	if _, left := LoadState().LastNag["bot"]; left {
		t.Error("the reminder outlived the repository")
	}
}

// Every map a caller is handed must be writable, on every path - including
// the one where there is no state file at all, which is a fresh install.
func TestLoadStateAlwaysReturnsWritableMaps(t *testing.T) {
	for _, tc := range []struct{ name, file string }{
		{"no state file at all", ""},
		{"an empty object", "{}"},
		{"a state file from before these fields existed", `{"conflicts":{},"last_sync":{}}`},
		{"rubbish", "not json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("SYNCHRONIZE_HOME", dir)
			if tc.file != "" {
				if err := os.WriteFile(statePath(), []byte(tc.file), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			st := LoadState()
			// Any of these would panic on a nil map.
			st.Conflicts["x"] = Conflict{}
			st.LastSync["x"] = time.Now()
			st.LastPrune["x"] = time.Now()
			st.LastNag["x"] = time.Now()
		})
	}
}
