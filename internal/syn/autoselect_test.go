package syn

import (
	"testing"
	"time"
)

// Automatic syncing is supposed to leave alone what you switched off or
// paused. These are the two halves of that promise: the watcher must not
// pick such a repository up when it starts, and must notice when you pause
// one while it is already running.
func TestAutomaticSyncingSkipsDisabledAndPaused(t *testing.T) {
	for _, tc := range []struct {
		name string
		repo Repo
		want bool
	}{
		{"enabled", Repo{Name: "a", Enabled: true}, true},
		{"switched off", Repo{Name: "a", Enabled: false}, false},
		{"paused for an hour", Repo{Name: "a", Enabled: true, PausedUntil: time.Now().Add(time.Hour)}, false},
		{"pause expired", Repo{Name: "a", Enabled: true, PausedUntil: time.Now().Add(-time.Hour)}, true},
		{"off and paused", Repo{Name: "a", Enabled: false, PausedUntil: time.Now().Add(time.Hour)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.repo.SyncsByItself(); got != tc.want {
				t.Errorf("SyncsByItself() = %v, want %v", got, tc.want)
			}
			kept := autoTargets([]*Repo{&tc.repo})
			if (len(kept) == 1) != tc.want {
				t.Errorf("autoTargets kept %d, want %d", len(kept), map[bool]int{true: 1, false: 0}[tc.want])
			}
		})
	}
}

// Pausing is useless if it only takes effect after a reboot: the watcher
// runs for days, and `syn pause` writes to the config file of a process
// that has already read it.
func TestWatcherNoticesAPauseSetWhileItRuns(t *testing.T) {
	env := newTestEnv(t)
	repos := []*Repo{env.a}

	// The watcher reads the config from disk, so put it there first.
	if err := env.engine.Cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if len(autoTargets(repos)) != 1 {
		t.Fatal("the repository should start out syncing by itself")
	}

	// Someone types `syn pause machine-a 2h` in another window: that writes
	// the config, it does not reach into this process.
	fresh, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	r := fresh.Find(env.a.Name)
	if r == nil {
		t.Fatalf("%s is not in the config on disk", env.a.Name)
	}
	r.PausedUntil = time.Now().Add(2 * time.Hour)
	if err := fresh.Save(); err != nil {
		t.Fatal(err)
	}

	env.engine.reloadAutoState(repos)
	if n := len(autoTargets(repos)); n != 0 {
		t.Errorf("the watcher still has %d repository(ies) to sync after one was paused", n)
	}
}
