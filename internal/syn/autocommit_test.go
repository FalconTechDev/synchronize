package syn

import (
	"strings"
	"testing"
)

// For a repository you actually develop in, committing every ten minutes
// fills the history with noise and sends half-finished work to the other
// machine. With autocommit off, syn publishes the commits you made and
// leaves the rest where it is.
func TestAutocommitOffPublishesOnlyYourOwnCommits(t *testing.T) {
	env := newTestEnv(t)
	env.a.NoAutoCommit = true

	// A commit made deliberately, and some work in progress beside it.
	env.write(env.a, "done.txt", "finished\n")
	mustGit(t, env.a.Path, "add", "done.txt")
	mustGit(t, env.a.Path, "commit", "--quiet", "-m", "a real commit")
	env.write(env.a, "wip.txt", "half written\n")

	res := env.engine.Push(env.a)
	if !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	if res.Committed {
		t.Error("committed on our behalf despite autocommit being off")
	}
	if res.Pushed == 0 {
		t.Error("the deliberate commit was not published")
	}

	// The real commit is on the remote; the work in progress is not.
	files := mustGit(t, env.a.Path, "ls-tree", "-r", "--name-only", "origin/main")
	if !strings.Contains(files, "done.txt") {
		t.Errorf("the deliberate commit did not reach the remote: %q", files)
	}
	if strings.Contains(files, "wip.txt") {
		t.Error("work in progress was published anyway")
	}

	// And it is still sitting in the working tree, untouched.
	if got := env.read(env.a, "wip.txt"); got != "half written\n" {
		t.Errorf("the uncommitted file was disturbed: %q", got)
	}
	if dirty, err := IsDirty(env.a.Path); err != nil || !dirty {
		t.Error("the working tree should still have the uncommitted work in it")
	}
}

// With it on - the default - nothing changes.
func TestAutocommitOnIsStillTheDefault(t *testing.T) {
	env := newTestEnv(t)
	env.write(env.a, "file.txt", "base\nedited\n")

	res := env.engine.Push(env.a)
	if !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	if !res.Committed {
		t.Error("the default should still commit for you")
	}
}

// Pulling is unaffected: it is sending that is held back, not receiving.
func TestAutocommitOffStillPulls(t *testing.T) {
	env := newTestEnv(t)
	env.b.NoAutoCommit = true

	env.write(env.a, "file.txt", "base\nfrom a\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	res := env.engine.Pull(env.b)
	if !res.OK() || res.Pulled == 0 {
		t.Fatalf("pull: %+v (%v)", res, res.Err)
	}
	if got := env.read(env.b, "file.txt"); got != "base\nfrom a\n" {
		t.Errorf("the change did not arrive: %q", got)
	}
}
