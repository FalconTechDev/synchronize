package syn

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLooksLikeSecret(t *testing.T) {
	for file, want := range map[string]bool{
		".env":             true,
		"service/.env":     true,
		".env.production":  true,
		"deploy/ID_RSA":    true,
		"certs/server.pem": true,
		"api.key":          true,
		"credentials.json": true,
		".npmrc":           true,
		// templates are meant to be committed
		".env.example":         false,
		".env.sample":          false,
		"config/.env.template": false,
		"id_rsa.pub":           false,
		// ordinary files that merely sit near the words
		"environment.md":    false,
		"src/keys.go":       false,
		"docs/env-vars.txt": false,
		"README.md":         false,
	} {
		if got := looksLikeSecret(file); got != want {
			t.Errorf("looksLikeSecret(%q) = %v, want %v", file, got, want)
		}
	}
}

// The case the guard exists for: a new .env turns up and the next tick would
// have published it.
func TestANewSecretIsNotCommittedAutomatically(t *testing.T) {
	env := newTestEnv(t)
	env.write(env.a, "app.go", "package main\n")
	env.write(env.a, ".env", "TOKEN=hunter2\n")

	res := env.engine.Push(env.a)
	if !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	tree := mustGit(t, env.a.Path, "ls-tree", "-r", "--name-only", "origin/main")
	if !strings.Contains(tree, "app.go") {
		t.Errorf("the ordinary file should still go: %q", tree)
	}
	if strings.Contains(tree, ".env") {
		t.Errorf(".env was published:\n%s", tree)
	}
	if got := env.read(env.a, ".env"); got != "TOKEN=hunter2\n" {
		t.Errorf("the held-back file must be left exactly as it was: %q", got)
	}
}

// Deciding it yourself is the way out, and syn must respect it from then on.
func TestASecretYouCommitYourselfIsCarried(t *testing.T) {
	env := newTestEnv(t)
	env.write(env.a, ".env", "TOKEN=one\n")
	mustGit(t, env.a.Path, "add", "-f", ".env")
	mustGit(t, env.a.Path, "commit", "--quiet", "-m", "on purpose")

	// Later edits to a file that is already tracked are ordinary changes.
	env.write(env.a, ".env", "TOKEN=two\n")
	if res := env.engine.Push(env.a); !res.OK() || !res.Committed {
		t.Fatalf("a tracked .env was held back as though it were new: %+v (%v)", res, res.Err)
	}
	if got := mustGit(t, env.a.Path, "show", "origin/main:.env"); strings.TrimSpace(got) != "TOKEN=two" {
		t.Errorf("the edit to the tracked file did not publish: %q", got)
	}
}

func TestStagingASecretYourselfCountsAsDecidingToo(t *testing.T) {
	env := newTestEnv(t)
	env.write(env.a, "prod.pem", "-----BEGIN-----\n")
	mustGit(t, env.a.Path, "add", "-f", "prod.pem")

	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	if tree := mustGit(t, env.a.Path, "ls-tree", "-r", "--name-only", "origin/main"); !strings.Contains(tree, "prod.pem") {
		t.Errorf("a file you staged on purpose was held back:\n%s", tree)
	}
}

func TestTheSecretGuardCanBeSwitchedOff(t *testing.T) {
	env := newTestEnv(t)
	env.a.SecretGuardOff = true
	env.write(env.a, ".env", "TOKEN=x\n")

	if res := env.engine.Push(env.a); !res.OK() || !res.Committed {
		t.Fatalf("push: %+v (%v)", res, res.Err)
	}
	if tree := mustGit(t, env.a.Path, "ls-tree", "-r", "--name-only", "origin/main"); !strings.Contains(tree, ".env") {
		t.Errorf("secret_guard off should let it through:\n%s", tree)
	}
}

// A file the remote would reject has to be left out, or every later sync
// fails on the same commit.
func TestAFileTooBigForTheRemoteIsLeftOut(t *testing.T) {
	t.Setenv("SYN_MAX_FILE_MB", "1")
	env := newTestEnv(t)
	big := make([]byte, 2<<20)
	if err := os.WriteFile(filepath.Join(env.a.Path, "video.bin"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	env.write(env.a, "notes.txt", "small\n")

	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	tree := mustGit(t, env.a.Path, "ls-tree", "-r", "--name-only", "origin/main")
	if strings.Contains(tree, "video.bin") {
		t.Errorf("a file over the limit was committed:\n%s", tree)
	}
	if !strings.Contains(tree, "notes.txt") {
		t.Errorf("the small file should not be held up by the big one:\n%s", tree)
	}
}

// With Git LFS the file on disk is large and what is committed is a pointer,
// so the size rule must not fire.
func TestLargeFilesAreFineWhenTheRepoUsesLFS(t *testing.T) {
	t.Setenv("SYN_MAX_FILE_MB", "1")
	env := newTestEnv(t)
	env.write(env.a, ".gitattributes", "*.bin filter=lfs diff=lfs merge=lfs -text\n")
	if err := os.WriteFile(filepath.Join(env.a.Path, "model.bin"), make([]byte, 2<<20), 0o644); err != nil {
		t.Fatal(err)
	}

	take, held := guard(env.a, []FileChange{
		{Status: "new", Path: ".gitattributes"},
		{Status: "new", Path: "model.bin"},
	})
	if len(held) != 0 || len(take) != 2 {
		t.Errorf("a big file in an LFS repository was held back: take %v, held %v", take, held)
	}
}

// What --dry-run promises has to be what the real run does.
func TestDryRunAgreesWithTheGuard(t *testing.T) {
	env := newTestEnv(t)
	env.write(env.a, "app.go", "package main\n")
	env.write(env.a, ".env", "TOKEN=x\n")

	for _, f := range WouldCommit(env.a) {
		if f.Path == ".env" {
			t.Error("the dry run says it would commit a .env the real run leaves alone")
		}
	}
	guarded := WouldGuard(env.a)
	if len(guarded) != 1 || guarded[0].File.Path != ".env" {
		t.Errorf("WouldGuard = %v, want just .env", guarded)
	}
}

func TestMaxFileBytes(t *testing.T) {
	t.Setenv("SYN_MAX_FILE_MB", "")
	if got := maxFileBytes(); got != int64(defaultMaxFileMB)<<20 {
		t.Errorf("default = %d", got)
	}
	t.Setenv("SYN_MAX_FILE_MB", "10")
	if got := maxFileBytes(); got != 10<<20 {
		t.Errorf("override = %d", got)
	}
	for _, bad := range []string{"abc", "-3", "0"} {
		t.Setenv("SYN_MAX_FILE_MB", bad)
		if got := maxFileBytes(); got != int64(defaultMaxFileMB)<<20 {
			t.Errorf("a nonsense override %q should fall back to the default, got %d", bad, got)
		}
	}
}
