package syn

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// heartbeatApp writes to a file forever. It is a shell loop, so the process
// doing the writing is a *child* of the shell syn starts - which is the whole
// difficulty: killing the shell alone leaves this running.
func heartbeatApp(marker string) string {
	if runtime.GOOS == "windows" {
		// No wrapping cmd /c: shellCommand already runs this through
		// `cmd /s /c`, and a second one re-parses the parentheses wrongly.
		return "@for /l %i in (1,1,100000) do (echo tick>> " + marker + " & ping -n 2 127.0.0.1 >nul)"
	}
	return "while true; do echo tick >> '" + marker + "'; sleep 1; done"
}

// Stopping an app has to stop everything it started. A bot left running
// beside its replacement answers the same messages twice, and nothing in the
// log would say so.
func TestStoppingAnAppKillsTheWholeTree(t *testing.T) {
	env := newTestEnv(t)
	marker := filepath.Join(env.dir, "heartbeat.txt")
	env.b.App = &AppConfig{Command: heartbeatApp(marker)}

	env.engine.StartApps([]*Repo{env.b})
	waitForFile(t, marker, 2, "the heartbeat never started")

	env.engine.StopApps()

	// Let anything still alive have time to write, then check it did not.
	time.Sleep(2 * time.Second)
	first, err := os.Stat(marker)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Second)
	second, err := os.Stat(marker)
	if err != nil {
		t.Fatal(err)
	}
	if second.Size() != first.Size() {
		t.Errorf("something is still running after the app was stopped: "+
			"the file grew from %d to %d bytes", first.Size(), second.Size())
	}
}

// The same thing at the moment it matters most: a restart must not leave the
// old version running.
func TestRestartingAnAppDoesNotLeaveTheOldOneRunning(t *testing.T) {
	env := newTestEnv(t)
	marker := filepath.Join(env.dir, "heartbeat.txt")
	env.b.App = &AppConfig{Command: heartbeatApp(marker)}

	env.engine.StartApps([]*Repo{env.b})
	defer env.engine.StopApps()
	waitForFile(t, marker, 2, "the heartbeat never started")

	env.engine.appMu.Lock()
	s := env.engine.apps[env.b.Name]
	env.engine.appMu.Unlock()

	s.Restart()
	if !s.Running() {
		t.Fatal("the app is not running after a restart")
	}

	// One writer means one process. Count how fast the file grows and
	// compare it with the rate before - two copies would write twice as
	// often.
	time.Sleep(1 * time.Second)
	a, _ := os.Stat(marker)
	time.Sleep(4 * time.Second)
	b, _ := os.Stat(marker)
	grew := b.Size() - a.Size()

	s.Stop()
	time.Sleep(2 * time.Second)
	c, _ := os.Stat(marker)
	time.Sleep(3 * time.Second)
	d, _ := os.Stat(marker)
	if d.Size() != c.Size() {
		t.Errorf("the old copy outlived the restart: the file is still growing "+
			"(%d -> %d) after everything was stopped; it grew %d bytes while supposedly one process ran",
			c.Size(), d.Size(), grew)
	}
}
