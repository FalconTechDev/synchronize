package syn

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

// A watcher that was killed rather than asked to stop leaves its note
// behind. Reporting that as "running" would be worse than useless: it is
// exactly the situation where nothing is syncing and everything looks fine.
func TestAKilledWatcherIsNotReportedAsRunning(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SYNCHRONIZE_HOME", dir)

	stale := Heartbeat{PID: 4242, Started: time.Now().Add(-time.Hour), Updated: time.Now().Add(-2 * time.Minute)}
	b, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(heartbeatPath(), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if hb := RunningWatcher(); hb != nil {
		t.Errorf("a note last touched 2 minutes ago was reported as a running watcher: %+v", hb)
	}
}

func TestARunningWatcherIsReported(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SYNCHRONIZE_HOME", dir)

	w := startHeartbeat(3)
	defer w.stop()

	hb := RunningWatcher()
	if hb == nil {
		t.Fatal("the watcher's own note was not read back")
	}
	if hb.PID != os.Getpid() {
		t.Errorf("pid = %d, want %d", hb.PID, os.Getpid())
	}
	if hb.Repos != 3 {
		t.Errorf("repos = %d, want 3", hb.Repos)
	}

	// Stopping tidily removes it, so status is right straight away rather
	// than after the staleness window.
	w.stop()
	if hb := RunningWatcher(); hb != nil {
		t.Error("the note outlived a tidy shutdown")
	}
}

func TestNoNoteMeansNotRunning(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SYNCHRONIZE_HOME", dir)
	if hb := RunningWatcher(); hb != nil {
		t.Errorf("found a watcher where there is no note at all: %+v", hb)
	}
}

// Rubbish in the file must read as "not running" rather than crash the
// command that asked.
func TestAnUnreadableNoteIsNotAWatcher(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SYNCHRONIZE_HOME", dir)
	if err := os.WriteFile(heartbeatPath(), []byte("{half written"), 0o600); err != nil {
		t.Fatal(err)
	}
	if hb := RunningWatcher(); hb != nil {
		t.Errorf("a corrupt note was read as a running watcher: %+v", hb)
	}
}

func TestNextCheckFollowsTheInterval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.IntervalMinutes = 10
	e := &Engine{Cfg: cfg, State: &State{LastSync: map[string]time.Time{}, Conflicts: map[string]Conflict{}}}
	r := &Repo{Name: "a", Enabled: true}

	if got := e.NextCheck(r); !got.IsZero() {
		t.Errorf("a repository never synced should have no due time, got %v", got)
	}

	last := time.Now().Add(-3 * time.Minute)
	e.State.LastSync["a"] = last
	want := last.Add(10 * time.Minute)
	if got := e.NextCheck(r); !got.Equal(want) {
		t.Errorf("NextCheck = %v, want %v", got, want)
	}

	// A per-repository interval wins over the global one.
	r.IntervalMinutes = 1
	if got := e.NextCheck(r); !got.Equal(last.Add(time.Minute)) {
		t.Errorf("the repository's own interval was ignored: %v", got)
	}
}
