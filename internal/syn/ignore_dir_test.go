package syn

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `syn ignore my-repo "*.env"` promises that no automatic commit takes a
// .env. git reports a brand-new folder as one entry, "dir/", without looking
// inside it, so a pattern for .env never matched the file in there and the
// commit swept it up with everything else.
func TestIgnoreAppliesInsideANewFolder(t *testing.T) {
	env := newTestEnv(t)
	env.a.NoCommit = []string{"*.env"}

	if err := os.MkdirAll(filepath.Join(env.a.Path, "service"), 0o755); err != nil {
		t.Fatal(err)
	}
	env.write(env.a, filepath.Join("service", "app.go"), "package main\n")
	env.write(env.a, filepath.Join("service", "prod.env"), "TOKEN=hunter2\n")

	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}

	tree := mustGit(t, env.a.Path, "ls-tree", "-r", "--name-only", "origin/main")
	if !strings.Contains(tree, "service/app.go") {
		t.Errorf("the ordinary file was not committed: %q", tree)
	}
	if strings.Contains(tree, "prod.env") {
		t.Errorf("prod.env was published even though *.env is held back:\n%s", tree)
	}
	if _, err := os.Stat(filepath.Join(env.a.Path, "service", "prod.env")); err != nil {
		t.Errorf("the held-back file should still be on disk: %v", err)
	}
}
