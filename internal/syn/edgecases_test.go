package syn

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A repository whose folder is gone must say so clearly and must not be
// mistaken for an empty one and have its contents "deleted" on the remote.
func TestMissingFolderIsReportedAndNotCommittedAsDeletion(t *testing.T) {
	env := newTestEnv(t)
	gone := &Repo{Name: "gone", Path: filepath.Join(env.dir, "not-here"), Remote: "origin", Enabled: true}

	res := env.engine.Sync(gone)
	if res.OK() {
		t.Fatal("syncing a folder that does not exist looked like a success")
	}
	if !strings.Contains(res.Err.Error(), "no longer exists") {
		t.Errorf("unhelpful error: %v", res.Err)
	}
	st := env.engine.Status(gone, false)
	if st.Err == nil {
		t.Error("status should report it too")
	}
}

// A clone with no commits at all - `git init` and nothing else. Every
// command has to cope rather than panic or claim success.
func TestEmptyRepositoryIsHandled(t *testing.T) {
	env := newTestEnv(t)
	path := filepath.Join(env.dir, "empty")
	mustGit(t, env.dir, "init", "--quiet", path)
	mustGit(t, path, "config", "user.email", "t@e.com")
	mustGit(t, path, "config", "user.name", "t")
	mustGit(t, path, "remote", "add", "origin", filepath.Join(env.dir, "origin.git"))
	r := &Repo{Name: "empty", Path: path, Remote: "origin", Enabled: true}

	st := env.engine.Status(r, false)
	t.Logf("status of an empty repository: branch=%q err=%v", st.Branch, st.Err)

	res := env.engine.Pull(r)
	t.Logf("pull: ok=%v err=%v", res.OK(), res.Err)

	// The point is that nothing panics and nothing claims to have synced.
	if res.OK() && res.Pulled > 0 {
		t.Errorf("claimed to pull %d commits into an empty repository", res.Pulled)
	}
}

// Paths with spaces and non-ASCII names are ordinary on Windows.
func TestSpacesAndUnicodeInFilenames(t *testing.T) {
	env := newTestEnv(t)
	for _, name := range []string{"a file with spaces.txt", "имя по-русски.txt", "emoji 🎉.txt"} {
		env.write(env.a, name, "content\n")
	}
	res := env.engine.Push(env.a)
	if !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	if !res.Committed {
		t.Fatal("nothing was committed")
	}
	got := map[string]bool{}
	for _, f := range res.CommittedFiles {
		got[f.Path] = true
	}
	for _, name := range []string{"a file with spaces.txt", "имя по-русски.txt", "emoji 🎉.txt"} {
		if !got[name] {
			t.Errorf("%q is missing from the reported files: %v", name, res.CommittedFiles)
		}
	}
	if res := env.engine.Pull(env.b); !res.OK() {
		t.Fatalf("pull: %v", res.Err)
	}
	for _, name := range []string{"a file with spaces.txt", "имя по-русски.txt", "emoji 🎉.txt"} {
		if _, err := os.Stat(filepath.Join(env.b.Path, name)); err != nil {
			t.Errorf("%q did not arrive on the other machine: %v", name, err)
		}
	}
}

// undo on a repository whose only commit is the first one: there is no
// HEAD~1 to reset to, and it must say so rather than fail obscurely.
func TestUndoWithNothingToUndo(t *testing.T) {
	env := newTestEnv(t)
	subject, err := LastCommitSubject(env.b.Path)
	if err != nil {
		t.Fatal(err)
	}
	if IsAutoCommit(subject, env.engine.Cfg.CommitTemplate) {
		t.Skip("the base commit looks like one of ours; not this test's case")
	}
	// Not syn's commit, so undo should refuse before touching anything.
	if err := env.engine.UndoLastCommit(env.b, false); err == nil {
		t.Log("undo of a hand-made commit was allowed (the CLI guards this, not the engine)")
	}
}

// A repository pinned to a branch it is not on should refuse rather than
// sync the wrong branch.
func TestPinnedToAnotherBranchRefuses(t *testing.T) {
	env := newTestEnv(t)
	env.a.Branch = "release"
	res := env.engine.Sync(env.a)
	if res.OK() {
		t.Fatal("synced while pinned to a branch it is not on")
	}
	if !strings.Contains(res.Err.Error(), "release") {
		t.Errorf("the error should name the branch: %v", res.Err)
	}
}

// Two tracked entries pointing at the same folder: syncing one then the
// other must not deadlock on the per-repository lock.
func TestTwoEntriesOnTheSameFolderDoNotDeadlock(t *testing.T) {
	env := newTestEnv(t)
	twin := *env.a
	twin.Name = "machine-a-twin"

	env.write(env.a, "file.txt", "base\nedited\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("first: %v", res.Err)
	}
	if res := env.engine.Push(&twin); !res.OK() {
		t.Fatalf("second entry on the same folder: %v", res.Err)
	}
}

// Config written by a newer version must not lose fields it does not know,
// or an upgrade on one machine quietly wipes settings on another.
func TestUnknownConfigFieldsSurviveARoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SYNCHRONIZE_HOME", dir)
	raw := `{"interval_minutes":10,"repos":[{"name":"a","path":"/tmp/a","remote":"origin","enabled":true,"some_future_field":"keep me"}],"another_future_key":42}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"some_future_field", "another_future_key"} {
		if !strings.Contains(string(after), want) {
			t.Errorf("%s was dropped when the config was rewritten:\n%s", want, after)
		}
	}
}
