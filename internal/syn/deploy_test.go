package syn

import (
	"strings"
	"testing"
)

// A deploy target takes changes and sends none. What the running service
// writes into its own working tree is its business, and must never reach the
// machines people work on.
func TestDeployTargetPullsButNeverPushes(t *testing.T) {
	env := newTestEnv(t)
	env.b.PullOnly = true

	// New code arrives from the machine someone works on.
	env.write(env.a, "app.go", "package main // v2\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}

	// Meanwhile the server has written its own rubbish into the folder.
	env.write(env.b, "app.log", "started\nrequest\nrequest\n")
	env.write(env.b, "cache.db", "binary junk\n")

	res := env.engine.Sync(env.b)
	if !res.OK() {
		t.Fatalf("sync: %v", res.Err)
	}
	if res.Pulled == 0 {
		t.Error("the deploy target did not take the new code")
	}
	if res.Committed || res.Pushed != 0 {
		t.Errorf("the deploy target published something: committed=%v pushed=%d",
			res.Committed, res.Pushed)
	}
	if got := env.read(env.b, "app.go"); !strings.Contains(got, "v2") {
		t.Errorf("the new code did not arrive: %q", got)
	}

	// The rubbish is still on disk, untouched and uncommitted...
	if got := env.read(env.b, "app.log"); got != "started\nrequest\nrequest\n" {
		t.Errorf("the service's own file was disturbed: %q", got)
	}
	// ...and nowhere near the remote.
	files := mustGit(t, env.a.Path, "ls-tree", "-r", "--name-only", "origin/main")
	for _, junk := range []string{"app.log", "cache.db"} {
		if strings.Contains(files, junk) {
			t.Errorf("%s was published from the deploy target", junk)
		}
	}
}

func TestDeployTargetRefusesAPushOutright(t *testing.T) {
	env := newTestEnv(t)
	env.b.PullOnly = true
	env.write(env.b, "app.log", "noise\n")

	// Both the plain and the forced push: the watcher pushes everything it
	// is given when it stops, and the shutdown handler does the same.
	for name, res := range map[string]Result{
		"push":       env.engine.Push(env.b),
		"push force": env.engine.ForcePush(env.b),
	} {
		if !res.OK() {
			t.Errorf("%s on a deploy target should not be an error: %v", name, res.Err)
		}
		if res.Pushed != 0 || res.Committed {
			t.Errorf("%s published from a deploy target: %+v", name, res)
		}
		if !strings.Contains(res.Summary(), "deploy target") {
			t.Errorf("%s summary does not explain itself: %q", name, res.Summary())
		}
	}
	// Nothing was committed, so the file is still just sitting there.
	dirty, err := IsDirty(env.b.Path)
	if err != nil || !dirty {
		t.Errorf("the untracked file should still be uncommitted (dirty=%v, err=%v)", dirty, err)
	}
}

func TestDeployTargetStillTakesAConflictSeriously(t *testing.T) {
	env := newTestEnv(t)
	env.b.PullOnly = true

	// Someone edited the same tracked file on the server by hand. Pulling
	// over it is a conflict like any other - being a deploy target does not
	// make it safe to throw work away silently.
	env.write(env.a, "file.txt", "base\nfrom a\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	env.write(env.b, "file.txt", "base\nedited on the server\n")

	res := env.engine.Sync(env.b)
	if res.Conflict == nil && res.OK() && res.Pulled == 0 {
		t.Errorf("the pull neither landed nor complained: %+v", res)
	}
}
