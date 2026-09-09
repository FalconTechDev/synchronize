//go:build windows

package syn

import (
	"os"
	"os/exec"
	"syscall"
)

// createNoWindow gives the child a console of its own with no window drawn
// for it. The console still exists, which matters: the shutdown handler is a
// console control handler, and without a console there is nothing to hand it.
const createNoWindow = 0x08000000

// detachedMarker stops the relaunch below from happening twice.
const detachedMarker = "SYN_DETACHED"

// DetachIfWindowed re-runs this program without a console window and reports
// that the caller should now exit.
//
// Hiding the window is not enough on Windows 11. When Windows Terminal is the
// default console host - it is, out of the box - the window you see belongs to
// WindowsTerminal.exe, and GetConsoleWindow hands us only the pseudo-console
// that ConPTY made for us. Hiding that hides nothing: the terminal window
// stays on screen all day with an empty black pane.
//
// So the console is never drawn in the first place. The watcher relaunches
// itself with CREATE_NO_WINDOW and the first process exits, taking the
// terminal window with it.
//
// A watcher started from a shell is left alone: you asked to see it.
func DetachIfWindowed() bool {
	if os.Getenv(detachedMarker) == "1" {
		return false // this is the relaunched copy
	}
	if !ownsConsole() {
		return false // a shell is attached too, so somebody is watching
	}
	exe, err := ExePath()
	if err != nil {
		Debug("could not find our own path to relaunch without a window: %v", err)
		return false
	}

	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = append(os.Environ(), detachedMarker+"=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	if wd, err := os.Getwd(); err == nil {
		cmd.Dir = wd
	}
	if err := cmd.Start(); err != nil {
		// Better a visible window than no watcher at all.
		Warn("could not restart without a console window: %v", err)
		return false
	}
	Debug("restarted without a console window as pid %d", cmd.Process.Pid)
	return true
}
