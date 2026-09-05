package syn

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testEnv struct {
	t      *testing.T
	dir    string
	engine *Engine
	a, b   *Repo
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := git(dir, args...)
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return out
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	if _, err := GitVersion(); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	t.Setenv("SYNCHRONIZE_HOME", filepath.Join(dir, "home"))

	origin := filepath.Join(dir, "origin.git")
	mustGit(t, dir, "init", "--bare", "--quiet", origin)

	clone := func(name string) *Repo {
		path := filepath.Join(dir, name)
		mustGit(t, dir, "clone", "--quiet", origin, path)
		mustGit(t, path, "config", "user.email", "test@example.com")
		mustGit(t, path, "config", "user.name", "test")
		mustGit(t, path, "checkout", "--quiet", "-B", "main")
		return &Repo{Name: name, Path: path, Remote: "origin", Enabled: true}
	}

	a := clone("machine-a")
	if err := os.WriteFile(filepath.Join(a.Path, "file.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, a.Path, "add", "-A")
	mustGit(t, a.Path, "commit", "--quiet", "-m", "base")
	mustGit(t, a.Path, "push", "--quiet", "-u", "origin", "main")

	b := clone("machine-b")
	mustGit(t, b.Path, "fetch", "--quiet", "origin")
	mustGit(t, b.Path, "reset", "--hard", "--quiet", "origin/main")

	cfg := DefaultConfig()
	cfg.Repos = []Repo{*a, *b}
	Quiet = true
	return &testEnv{t: t, dir: dir, engine: NewEngine(cfg), a: &cfg.Repos[0], b: &cfg.Repos[1]}
}

func (e *testEnv) write(r *Repo, name, content string) {
	e.t.Helper()
	if err := os.WriteFile(filepath.Join(r.Path, name), []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *testEnv) read(r *Repo, name string) string {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(r.Path, name))
	if err != nil {
		e.t.Fatal(err)
	}
	return string(b)
}

func TestPushThenPullMovesWorkBetweenMachines(t *testing.T) {
	env := newTestEnv(t)
	env.write(env.a, "file.txt", "base\nfrom a\n")

	if res := env.engine.Push(env.a); !res.OK() || !res.Committed || res.Pushed == 0 {
		t.Fatalf("push: %+v (%v)", res, res.Err)
	}
	res := env.engine.Pull(env.b)
	if !res.OK() || res.Pulled != 1 {
		t.Fatalf("pull: %+v (%v)", res, res.Err)
	}
	if got := env.read(env.b, "file.txt"); got != "base\nfrom a\n" {
		t.Fatalf("machine-b has %q", got)
	}
}

func TestPullKeepsUncommittedWork(t *testing.T) {
	env := newTestEnv(t)
	env.write(env.a, "file.txt", "base\nfrom a\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}

	env.write(env.b, "scratch.txt", "work in progress\n")

	if res := env.engine.Pull(env.b); !res.OK() {
		t.Fatalf("pull: %v", res.Err)
	}
	if got := env.read(env.b, "scratch.txt"); got != "work in progress\n" {
		t.Fatalf("uncommitted work was lost: %q", got)
	}
	if got := env.read(env.b, "file.txt"); got != "base\nfrom a\n" {
		t.Fatalf("remote change did not arrive: %q", got)
	}
}

func TestConflictLeavesWorkingTreeIntactAndRecordsBackup(t *testing.T) {
	env := newTestEnv(t)
	env.write(env.b, "file.txt", "base\nfrom b\n")
	if res := env.engine.Push(env.b); !res.OK() {
		t.Fatalf("push b: %v", res.Err)
	}
	env.write(env.a, "file.txt", "base\nfrom a\n")

	res := env.engine.Push(env.a)
	if res.Conflict == nil {
		t.Fatalf("expected a conflict, got %+v (%v)", res, res.Err)
	}
	if got := env.read(env.a, "file.txt"); got != "base\nfrom a\n" {
		t.Fatalf("working tree was disturbed: %q", got)
	}
	if InProgress(env.a.Path) != "" {
		t.Fatal("a rebase was left in progress")
	}
	backups, err := Backups(env.a)
	if err != nil || len(backups) == 0 {
		t.Fatalf("no backup branch was taken (%v)", err)
	}
	if _, ok := env.engine.State.Conflicts[env.a.Name]; !ok {
		t.Fatal("conflict was not recorded in state")
	}
}

func TestResolveReplaysThenCompletes(t *testing.T) {
	env := newTestEnv(t)
	env.write(env.b, "file.txt", "base\nfrom b\n")
	if res := env.engine.Push(env.b); !res.OK() {
		t.Fatalf("push b: %v", res.Err)
	}
	env.write(env.a, "file.txt", "base\nfrom a\n")
	if res := env.engine.Push(env.a); res.Conflict == nil {
		t.Fatal("expected a conflict")
	}

	if res := env.engine.Resolve(env.a, "continue"); res.Err == nil {
		t.Fatal("expected the replayed conflict to be reported")
	}
	if len(ConflictedFiles(env.a.Path)) == 0 {
		t.Fatal("expected conflict markers in the working tree")
	}

	env.write(env.a, "file.txt", "base\nmerged\n")
	if res := env.engine.Resolve(env.a, "continue"); !res.OK() {
		t.Fatalf("resolve: %+v (%v)", res, res.Err)
	}
	if res := env.engine.Pull(env.b); !res.OK() {
		t.Fatalf("pull b: %v", res.Err)
	}
	if got := env.read(env.b, "file.txt"); got != "base\nmerged\n" {
		t.Fatalf("merged result did not reach machine-b: %q", got)
	}
}

func TestResolveTheirsPrefersTheRemote(t *testing.T) {
	env := newTestEnv(t)
	env.write(env.b, "file.txt", "base\nfrom b\n")
	if res := env.engine.Push(env.b); !res.OK() {
		t.Fatalf("push b: %v", res.Err)
	}
	env.write(env.a, "file.txt", "base\nfrom a\n")
	if res := env.engine.Push(env.a); res.Conflict == nil {
		t.Fatal("expected a conflict")
	}
	if res := env.engine.Resolve(env.a, "theirs"); !res.OK() {
		t.Fatalf("resolve --theirs: %+v (%v)", res, res.Err)
	}
	if got := env.read(env.a, "file.txt"); got != "base\nfrom b\n" {
		t.Fatalf("expected the remote side to win, got %q", got)
	}
}

func TestForcePullDiscardsLocalButKeepsABackup(t *testing.T) {
	env := newTestEnv(t)
	env.write(env.b, "file.txt", "base\nfrom b\n")
	if res := env.engine.Push(env.b); !res.OK() {
		t.Fatalf("push b: %v", res.Err)
	}
	env.write(env.a, "file.txt", "base\nlocal junk\n")

	if res := env.engine.ForcePull(env.a); !res.OK() {
		t.Fatalf("force pull: %v", res.Err)
	}
	if got := env.read(env.a, "file.txt"); got != "base\nfrom b\n" {
		t.Fatalf("expected the remote content, got %q", got)
	}
	backups, _ := Backups(env.a)
	if len(backups) == 0 {
		t.Fatal("force pull discarded work without a backup branch")
	}
	if err := env.engine.RestoreBackup(env.a, backups[0]); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := env.read(env.a, "file.txt"); got != "base\nlocal junk\n" {
		t.Fatalf("restore did not bring the discarded work back: %q", got)
	}
}

func TestPruneBackupsDeletesEverythingWithZeroDays(t *testing.T) {
	env := newTestEnv(t)
	env.write(env.a, "file.txt", "base\nfrom a\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	if before, _ := Backups(env.a); len(before) == 0 {
		t.Skip("no backup branch was created, nothing to prune")
	}
	if _, err := env.engine.PruneBackups(env.a, 0); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if after, _ := Backups(env.a); len(after) != 0 {
		t.Fatalf("backups survived a zero-day prune: %v", after)
	}
}

func TestPrepareRejectsDetachedHead(t *testing.T) {
	env := newTestEnv(t)
	mustGit(t, env.a.Path, "checkout", "--quiet", "--detach")
	if _, err := env.engine.prepare(env.a); err == nil {
		t.Fatal("expected detached HEAD to be rejected")
	}
}

func TestSanitizeRefStripsRefUnsafeCharacters(t *testing.T) {
	for in, want := range map[string]string{
		"DESKTOP-PC":   "DESKTOP-PC",
		"my branch":    "my-branch",
		"feat/thing~1": "feat/thing-1",
		".hidden.":     "hidden",
	} {
		if got := sanitizeRef(in); got != want {
			t.Errorf("sanitizeRef(%q) = %q, want %q", in, got, want)
		}
	}
}
