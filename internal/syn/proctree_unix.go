//go:build !windows

package syn

import (
	"os/exec"
	"syscall"
)

// The app is started in its own process group, so a signal reaches the shell
// and everything it started - python included. Signalling only the shell
// would leave the bot running beside its replacement.
func detachFromConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// terminateTree asks the group to stop, which lets a bot close its
// connections and finish what it is writing.
func terminateTree(pid int) { signalGroup(pid, syscall.SIGTERM) }

// killTree is for when asking did not work.
func killTree(pid int) { signalGroup(pid, syscall.SIGKILL) }

func signalGroup(pid int, sig syscall.Signal) {
	// The negative pid is the process group. Fall back to the single
	// process if it was never given a group of its own.
	if err := syscall.Kill(-pid, sig); err != nil {
		_ = syscall.Kill(pid, sig)
	}
}
