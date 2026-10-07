package syn

import (
	"strings"
	"testing"
)

// A conflict is supposed to stop that repository until you say what to do.
// It did not: the next automatic sync swept the conflict markers into a
// commit and pushed them, so every other machine got a broken file and
// nothing in the log said why.
func TestAConflictStopsLaterSyncsInsteadOfPublishingMarkers(t *testing.T) {
	env := newTestEnv(t)

	env.write(env.a, "file.txt", "base\nfrom a\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	env.write(env.b, "file.txt", "base\nfrom b\n")

	first := env.engine.Sync(env.b)
	if first.Conflict == nil {
		t.Fatalf("expected a conflict, got %+v", first)
	}

	// The watcher comes round again ten minutes later.
	second := env.engine.Sync(env.b)
	if second.OK() {
		t.Error("the second sync carried on as if nothing were wrong")
	}
	if second.Err == nil || !strings.Contains(second.Err.Error(), "resolve") {
		t.Errorf("the error should point at `syn resolve`, got: %v", second.Err)
	}

	// Whatever happened locally, the markers must not be on the remote.
	remote := mustGit(t, env.a.Path, "show", "origin/main:file.txt")
	if strings.Contains(remote, "<<<<<<<") {
		t.Errorf("CONFLICT MARKERS WERE PUBLISHED:\n%s", remote)
	}
}

// Belt and braces: even with no conflict recorded - state file lost, or a
// merge somebody did by hand - a file with markers in it must never be
// committed on the user's behalf.
func TestAutoCommitRefusesFilesHoldingConflictMarkers(t *testing.T) {
	env := newTestEnv(t)
	env.write(env.a, "file.txt", "base\n<<<<<<< HEAD\nmine\n=======\ntheirs\n>>>>>>> other\n")

	committed, _, err := env.engine.autoCommit(env.a)
	if committed {
		t.Error("committed a file with conflict markers in it")
	}
	if err == nil {
		t.Fatal("that should have been refused with an explanation")
	}
	if !strings.Contains(err.Error(), "file.txt") {
		t.Errorf("the error should name the file: %v", err)
	}

	// And the push built on it must fail rather than publish.
	if res := env.engine.Push(env.a); res.OK() {
		t.Error("push went ahead despite the markers")
	}
}

func TestMarkersAreFoundWhereverTheySit(t *testing.T) {
	env := newTestEnv(t)
	for name, content := range map[string]string{
		"start.txt":  "<<<<<<< HEAD\na\n=======\nb\n>>>>>>> x\n",
		"middle.txt": "intro\n<<<<<<< HEAD\na\n=======\nb\n>>>>>>> x\ntail\n",
		"clean.txt":  "nothing wrong here\n",
		"prose.txt":  "I wrote <<<<<<< in the middle of a line, which is not a marker\n",
	} {
		env.write(env.a, name, content)
	}
	found := withMarkers(env.a.Path, []string{"start.txt", "middle.txt", "clean.txt", "prose.txt"})
	got := map[string]bool{}
	for _, f := range found {
		got[f] = true
	}
	if !got["start.txt"] || !got["middle.txt"] {
		t.Errorf("missed real markers: %v", found)
	}
	if got["clean.txt"] || got["prose.txt"] {
		t.Errorf("flagged a file that has no conflict in it: %v", found)
	}
}
