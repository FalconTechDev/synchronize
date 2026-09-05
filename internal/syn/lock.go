package syn

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// A Lock keeps two syn processes out of each other's way. The design invites
// them: a watcher on a timer, a task at login, another at shutdown, and
// whatever you type yourself - all driving git in the same folder and writing
// the same config.
//
// The lock is held by the operating system, not by a file we write, so a
// process that is killed does not leave one behind.
type Lock struct {
	f *os.File
}

func lockDir() string { return filepath.Join(Home(), "locks") }

// LockRepo waits for exclusive use of one repository. Two rebases in the same
// working tree leave a state neither process can finish.
func LockRepo(name string, wait time.Duration) (*Lock, error) {
	return acquire("repo-"+safeLockName(name), wait,
		fmt.Sprintf("another syn is working on %s", name))
}

// LockConfig covers a read-modify-write of config.json. Without it, a watcher
// saving while you add a repository drops whichever finished first.
func LockConfig(wait time.Duration) (*Lock, error) {
	return acquire("config", wait, "another syn is changing the settings")
}

func acquire(name string, wait time.Duration, busy string) (*Lock, error) {
	if err := os.MkdirAll(lockDir(), 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(lockDir(), name+".lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(wait)
	for {
		if ok, err := tryLock(f); err != nil {
			f.Close()
			return nil, err
		} else if ok {
			return &Lock{f: f}, nil
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("%s - waited %s", busy, wait)
		}
		time.Sleep(120 * time.Millisecond)
	}
}

func (l *Lock) Release() {
	if l == nil || l.f == nil {
		return
	}
	unlock(l.f)
	l.f.Close()
	l.f = nil
}

// safeLockName keeps a repository name usable as a filename; two repositories
// whose names differ only by case would share a lock, which is harmless.
func safeLockName(s string) string {
	repl := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "*", "-",
		"?", "-", "\"", "-", "<", "-", ">", "-", "|", "-", " ", "-")
	s = repl.Replace(strings.ToLower(s))
	if len(s) > 80 {
		s = s[:80]
	}
	if s == "" {
		s = "unnamed"
	}
	return s
}
