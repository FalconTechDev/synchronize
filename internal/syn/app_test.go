package syn

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// longRunningApp is a stand-in for a bot: it appends its version to a file
// and then stays up, exactly the shape of process a hook cannot handle.
func longRunningApp(marker, version string) string {
	if runtime.GOOS == "windows" {
		return "echo " + version + ">> " + marker + " & ping -n 600 127.0.0.1 >nul"
	}
	return "echo " + version + " >> '" + marker + "'; sleep 600"
}

func waitForFile(t *testing.T, path string, want int, why string) []string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(path)
		if err == nil {
			var lines []string
			for _, l := range strings.Split(string(b), "\n") {
				if l = strings.TrimSpace(l); l != "" {
					lines = append(lines, l)
				}
			}
			if len(lines) >= want {
				return lines
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	b, _ := os.ReadFile(path)
	t.Fatalf("%s: wanted %d start(s), file holds %q", why, want, b)
	return nil
}

func TestAppStartsAndStops(t *testing.T) {
	env := newTestEnv(t)
	marker := filepath.Join(env.dir, "starts.txt")
	env.b.App = &AppConfig{Command: longRunningApp(marker, "v1")}

	env.engine.StartApps([]*Repo{env.b})
	waitForFile(t, marker, 1, "the app never started")

	env.engine.StopApps()

	// It has to be gone, not merely detached: the shell we start is not the
	// process that matters, and leaving the tree behind means two copies of
	// a bot answering the same messages.
	env.engine.appMu.Lock()
	left := len(env.engine.apps)
	env.engine.appMu.Unlock()
	if left != 0 {
		t.Errorf("%d supervisor(s) left behind", left)
	}
}

func TestAppRestartsWhenTheCodeArrives(t *testing.T) {
	env := newTestEnv(t)
	marker := filepath.Join(env.dir, "starts.txt")
	env.b.App = &AppConfig{Command: longRunningApp(marker, "run")}

	env.engine.StartApps([]*Repo{env.b})
	defer env.engine.StopApps()
	waitForFile(t, marker, 1, "the app never started")

	// New code lands.
	env.write(env.a, "bot.py", "print('v2')\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	if res := env.engine.Pull(env.b); !res.OK() || res.Pulled == 0 {
		t.Fatalf("pull: %+v (%v)", res, res.Err)
	}

	// Two starts means the old one was stopped and a new one took its place.
	lines := waitForFile(t, marker, 2, "the app was not restarted after the pull")
	if len(lines) != 2 {
		t.Errorf("started %d times, wanted exactly 2: %v", len(lines), lines)
	}
}

func TestAppIsLeftAloneWhenItsFilesDidNotChange(t *testing.T) {
	env := newTestEnv(t)
	marker := filepath.Join(env.dir, "starts.txt")
	env.b.App = &AppConfig{Command: longRunningApp(marker, "run"), Paths: []string{"bot.py"}}

	env.engine.StartApps([]*Repo{env.b})
	defer env.engine.StopApps()
	waitForFile(t, marker, 1, "the app never started")

	env.write(env.a, "README.md", "docs only\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	if res := env.engine.Pull(env.b); !res.OK() || res.Pulled == 0 {
		t.Fatalf("pull: %+v (%v)", res, res.Err)
	}

	time.Sleep(3 * time.Second)
	b, _ := os.ReadFile(marker)
	if n := len(strings.Fields(string(b))); n != 1 {
		t.Errorf("the bot was restarted for a change to the README (started %d times)", n)
	}
}

// A pull outside a watcher must not start anything: `syn pull` is a command
// that finishes, and a bot it left running would outlive it as an orphan.
func TestAPlainPullDoesNotStartTheApp(t *testing.T) {
	env := newTestEnv(t)
	marker := filepath.Join(env.dir, "starts.txt")
	env.b.App = &AppConfig{Command: longRunningApp(marker, "run")}

	env.write(env.a, "bot.py", "print('v2')\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	if res := env.engine.Pull(env.b); !res.OK() {
		t.Fatalf("pull: %v", res.Err)
	}
	time.Sleep(2 * time.Second)
	if _, err := os.Stat(marker); err == nil {
		t.Error("a one-off pull started a program it would then abandon")
	}
}

func TestAppWantsDefaultsToEverything(t *testing.T) {
	if !(AppConfig{}).Wants([]string{"anything.txt"}) {
		t.Error("an app with no paths should restart on any update")
	}
	a := AppConfig{Paths: []string{"src/**"}}
	if a.Wants([]string{"README.md"}) {
		t.Error("an app that named paths should ignore everything else")
	}
	if !a.Wants([]string{"src/main.go"}) {
		t.Error("an app that named paths should notice them")
	}
}
