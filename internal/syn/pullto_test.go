package syn

import (
	"strings"
	"testing"
)

func TestPullToPinsAndRollsBack(t *testing.T) {
	env := newTestEnv(t)

	// Two releases on the remote, the second of which we will want to undo.
	env.write(env.a, "file.txt", "v1\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push v1: %v", res.Err)
	}
	v1, err := CurrentCommit(env.a.Path)
	if err != nil {
		t.Fatal(err)
	}
	mustGit(t, env.a.Path, "tag", "v1")
	mustGit(t, env.a.Path, "push", "--quiet", "origin", "v1")

	env.write(env.a, "file.txt", "v2 broken\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push v2: %v", res.Err)
	}

	// The other machine takes v2, as an ordinary pull would.
	if res := env.engine.Pull(env.b); !res.OK() {
		t.Fatalf("pull: %v", res.Err)
	}
	if got := env.read(env.b, "file.txt"); got != "v2 broken\n" {
		t.Fatalf("machine-b has %q", got)
	}

	// v2 is bad; go back to the tag and stay there.
	res := env.engine.PullTo("v1")(env.b)
	if !res.OK() {
		t.Fatalf("pull --to v1: %v", res.Err)
	}
	if got := env.read(env.b, "file.txt"); got != "v1\n" {
		t.Errorf("after rolling back to v1 the file is %q", got)
	}
	at, err := CurrentCommit(env.b.Path)
	if err != nil {
		t.Fatal(err)
	}
	if at != v1 {
		t.Errorf("on %s, wanted %s", short(at), short(v1))
	}
	if res.Backup == "" {
		t.Error("moving the branch by force should leave a backup behind")
	}

	// Asking for the same ref twice changes nothing and says so.
	if again := env.engine.PullTo("v1")(env.b); !again.OK() || again.Pulled != 0 {
		t.Errorf("a second pull --to v1 should be a no-op, got %+v", again)
	}
}

func TestPullToRejectsARefThatDoesNotExist(t *testing.T) {
	env := newTestEnv(t)
	res := env.engine.PullTo("v9.9.9-nope")(env.b)
	if res.Err == nil {
		t.Fatal("a ref that does not exist must be an error, not a silent no-op")
	}
	if !strings.Contains(res.Err.Error(), "v9.9.9-nope") {
		t.Errorf("the error should name what was asked for: %v", res.Err)
	}
	// And nothing should have moved.
	if got := env.read(env.b, "file.txt"); got != "base\n" {
		t.Errorf("the repository moved anyway: %q", got)
	}
}

func TestPullToPrefersTheRemoteBranch(t *testing.T) {
	env := newTestEnv(t)
	env.write(env.a, "file.txt", "ahead\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	// machine-b's own main is behind; "--to main" has to mean the remote's,
	// otherwise pinning a deploy to a branch would pin it to a stale copy.
	if res := env.engine.PullTo("main")(env.b); !res.OK() {
		t.Fatalf("pull --to main: %v", res.Err)
	}
	if got := env.read(env.b, "file.txt"); got != "ahead\n" {
		t.Errorf("--to main used the local branch, file is %q", got)
	}
}
