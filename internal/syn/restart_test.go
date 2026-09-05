package syn

import (
	"os"
	"path/filepath"
	"testing"
)

func restartTouching(path string) *RestartConfig {
	return &RestartConfig{Paths: []string{"bot.py"}, Command: writeVar("SYN_EVENT", path)}
}

// The whole point: a restart fires when the file it watches arrives, and
// stays quiet otherwise. It is a deploy server's only trigger, so both
// halves matter.
func TestRestartFiresOnlyForItsFiles(t *testing.T) {
	env := newTestEnv(t)
	fired := filepath.Join(env.dir, "restarted.txt")
	env.b.Restart = restartTouching(fired)

	// Something unrelated changes.
	env.write(env.a, "README.md", "docs\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	if res := env.engine.Pull(env.b); !res.OK() || res.Pulled == 0 {
		t.Fatalf("pull: %+v (%v)", res, res.Err)
	}
	if _, err := os.Stat(fired); err == nil {
		t.Fatal("the service was restarted for a change to the README")
	}

	// Now the file it watches.
	env.write(env.a, "bot.py", "print('v2')\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	if res := env.engine.Pull(env.b); !res.OK() {
		t.Fatalf("pull: %+v (%v)", res, res.Err)
	}
	if _, err := os.Stat(fired); err != nil {
		t.Fatalf("bot.py changed and nothing was restarted: %v", err)
	}
}

func TestRestartDoesNotFireWhenNothingArrived(t *testing.T) {
	env := newTestEnv(t)
	fired := filepath.Join(env.dir, "restarted.txt")
	env.b.Restart = restartTouching(fired)

	// A pull with nothing to bring down must not restart anything - this is
	// the watcher's normal case, once a minute, all day.
	if res := env.engine.Pull(env.b); !res.OK() {
		t.Fatalf("pull: %v", res.Err)
	}
	if _, err := os.Stat(fired); err == nil {
		t.Error("an empty pull restarted the service")
	}
}

func TestFailingRestartRollsThePullBack(t *testing.T) {
	env := newTestEnv(t)
	env.b.Restart = &RestartConfig{
		Paths:    []string{"bot.py"},
		Command:  failingHook(),
		Rollback: true,
	}
	before, err := CurrentCommit(env.b.Path)
	if err != nil {
		t.Fatal(err)
	}

	env.write(env.a, "bot.py", "print('broken')\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}

	res := env.engine.Pull(env.b)
	if res.Err == nil {
		t.Fatal("a rolled-back pull must report a failure")
	}
	if !res.RolledBack {
		t.Error("Result.RolledBack should say what happened")
	}
	after, err := CurrentCommit(env.b.Path)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("still on %s, wanted to be back on %s", short(after), short(before))
	}
	// The old code has to still be there - that is the reason to roll back.
	if _, err := os.Stat(filepath.Join(env.b.Path, "bot.py")); err == nil {
		t.Error("the new bot.py survived the rollback")
	}
}

func TestFailingRestartLeavesThePullAloneByDefault(t *testing.T) {
	env := newTestEnv(t)
	env.b.Restart = &RestartConfig{Paths: []string{"bot.py"}, Command: failingHook()}

	env.write(env.a, "bot.py", "print('v2')\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	res := env.engine.Pull(env.b)
	if res.RolledBack {
		t.Error("rolled back without being asked to")
	}
	if _, err := os.Stat(filepath.Join(env.b.Path, "bot.py")); err != nil {
		t.Errorf("the pull did not stick: %v", err)
	}
}

// A restart on a deploy target is the whole use case, so check the two
// features work together rather than only apart.
func TestRestartWorksOnADeployTarget(t *testing.T) {
	env := newTestEnv(t)
	fired := filepath.Join(env.dir, "restarted.txt")
	env.b.PullOnly = true
	env.b.Restart = restartTouching(fired)

	env.write(env.a, "bot.py", "print('v2')\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	// The server has its own scratch file, as servers do.
	env.write(env.b, "bot.log", "running\n")

	res := env.engine.Sync(env.b)
	if !res.OK() {
		t.Fatalf("sync: %v", res.Err)
	}
	if _, err := os.Stat(fired); err != nil {
		t.Fatalf("the service was not restarted: %v", err)
	}
	if res.Pushed != 0 || res.Committed {
		t.Errorf("the deploy target published something: %+v", res)
	}
}

func TestRestartWantsMatchesTheSamePatternsAsHooks(t *testing.T) {
	rc := RestartConfig{Paths: []string{"src/**", "bot.py"}}
	for _, tc := range []struct {
		changed []string
		want    bool
	}{
		{[]string{"bot.py"}, true},
		{[]string{"src/deep/thing.go"}, true},
		{[]string{"README.md"}, false},
		{[]string{"bot.pyc"}, false},
	} {
		if got := rc.Wants(tc.changed); got != tc.want {
			t.Errorf("Wants(%v) = %v, want %v", tc.changed, got, tc.want)
		}
	}
}
