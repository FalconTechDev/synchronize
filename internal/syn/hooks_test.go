package syn

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func envValue(env []string, key string) (string, bool) {
	for i := len(env) - 1; i >= 0; i-- {
		if k, v, ok := strings.Cut(env[i], "="); ok && k == key {
			return v, true
		}
	}
	return "", false
}

func TestHookEnvDescribesTheSync(t *testing.T) {
	r := &Repo{Name: "bot", Path: t.TempDir(), Remote: "origin"}
	info := hookInfo{
		Event:   "after-pull",
		Commit:  "1111111111111111111111111111111111111111",
		Prev:    "2222222222222222222222222222222222222222",
		Changed: []string{"src/main.go", "README.md"},
	}
	env := hookEnv(r, info)

	for key, want := range map[string]string{
		"SYN_REPO":          "bot",
		"SYN_PATH":          r.Path,
		"SYN_EVENT":         "after-pull",
		"SYN_COMMIT":        info.Commit,
		"SYN_PREV_COMMIT":   info.Prev,
		"SYN_CHANGED_COUNT": "2",
		// One path per line: a comma is legal in a filename, a newline is
		// not, so a deploy script can split on lines and be right.
		"SYN_CHANGED_FILES": "src/main.go\nREADME.md",
	} {
		got, ok := envValue(env, key)
		if !ok {
			t.Errorf("%s was not passed to the hook at all", key)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestHookEnvWithNothingChanged(t *testing.T) {
	env := hookEnv(&Repo{Name: "bot", Path: t.TempDir()}, hookInfo{Event: "after-push"})
	if got, _ := envValue(env, "SYN_CHANGED_COUNT"); got != "0" {
		t.Errorf("SYN_CHANGED_COUNT = %q, want 0", got)
	}
	// Empty, not missing: a script reading $SYN_CHANGED_FILES should get an
	// empty list rather than fall through to whatever the parent had set.
	if got, ok := envValue(env, "SYN_CHANGED_FILES"); !ok || got != "" {
		t.Errorf("SYN_CHANGED_FILES = %q (present: %v), want empty and present", got, ok)
	}
}

// writeVar is a hook that records one variable, so the test reads what the
// hook actually received rather than what we meant to send it.
func writeVar(name, path string) string {
	if runtime.GOOS == "windows" {
		// Unquoted on purpose: cmd /c already receives the whole command as
		// one quoted argument, and a second pair of quotes inside it is what
		// makes it complain about the filename. The temp path has no spaces.
		return "echo %" + name + "% > " + path
	}
	return "printf '%s' \"$" + name + "\" > '" + path + "'"
}

func TestAfterPullHookSeesTheEvent(t *testing.T) {
	env := newTestEnv(t)
	out := filepath.Join(env.dir, "event.txt")
	env.b.Hooks.AfterPull = []string{writeVar("SYN_EVENT", out)}

	env.write(env.a, "file.txt", "base\nfrom a\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	if res := env.engine.Pull(env.b); !res.OK() || res.Pulled == 0 {
		t.Fatalf("pull: %+v (%v)", res, res.Err)
	}

	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("the after-pull hook did not run: %v", err)
	}
	if got := strings.TrimSpace(string(b)); got != "after-pull" {
		t.Errorf("hook saw SYN_EVENT=%q, want after-pull", got)
	}
}

func TestAfterPushHookCountsTheCommittedFiles(t *testing.T) {
	env := newTestEnv(t)
	out := filepath.Join(env.dir, "count.txt")
	env.a.Hooks.AfterPush = []string{writeVar("SYN_CHANGED_COUNT", out)}

	env.write(env.a, "file.txt", "base\nedited\n")
	env.write(env.a, "extra.txt", "new\n")
	if res := env.engine.Push(env.a); !res.OK() || res.Pushed == 0 {
		t.Fatalf("push: %+v (%v)", res, res.Err)
	}

	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("the after-push hook did not run: %v", err)
	}
	if got := strings.TrimSpace(string(b)); got != "2" {
		t.Errorf("hook saw SYN_CHANGED_COUNT=%q, want 2 (file.txt and extra.txt)", got)
	}
}
