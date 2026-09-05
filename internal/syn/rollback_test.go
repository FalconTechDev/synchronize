package syn

import (
	"runtime"
	"testing"
)

func failingHook() string {
	if runtime.GOOS == "windows" {
		return "exit /b 1"
	}
	return "exit 1"
}

func TestFailingHookRollsThePullBack(t *testing.T) {
	env := newTestEnv(t)
	env.b.Hooks.AfterPull = HookList{{Run: failingHook()}}
	env.b.Hooks.Rollback = true

	before, err := CurrentCommit(env.b.Path)
	if err != nil {
		t.Fatal(err)
	}

	env.write(env.a, "file.txt", "base\nfrom a\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}

	res := env.engine.Pull(env.b)
	if res.Err == nil {
		t.Fatal("a rolled-back pull must report a failure, not look like a success")
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
	if got := env.read(env.b, "file.txt"); got != "base\n" {
		t.Errorf("the file kept the pulled content: %q", got)
	}
}

func TestFailingHookLeavesThePullAloneByDefault(t *testing.T) {
	env := newTestEnv(t)
	env.b.Hooks.AfterPull = HookList{{Run: failingHook()}}
	// Rollback not switched on: undoing a pull is a real loss, so it has to
	// be asked for.

	env.write(env.a, "file.txt", "base\nfrom a\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	res := env.engine.Pull(env.b)
	if res.RolledBack {
		t.Error("rolled back without being asked to")
	}
	if got := env.read(env.b, "file.txt"); got != "base\nfrom a\n" {
		t.Errorf("the pull did not stick: %q", got)
	}
}

func TestRollbackIsSkippedWhenItWouldDestroyWork(t *testing.T) {
	env := newTestEnv(t)
	env.b.Hooks.AfterPull = HookList{{Run: failingHook()}}
	env.b.Hooks.Rollback = true

	env.write(env.a, "file.txt", "base\nfrom a\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	// Uncommitted work in a file the pull does not touch. A hard reset would
	// take it with it, so the rollback has to stand down.
	env.write(env.b, "notes.txt", "something I was writing\n")

	res := env.engine.Pull(env.b)
	if res.RolledBack {
		t.Error("rolled back over uncommitted work")
	}
	if got := env.read(env.b, "notes.txt"); got != "something I was writing\n" {
		t.Errorf("uncommitted work was lost: %q", got)
	}
}
