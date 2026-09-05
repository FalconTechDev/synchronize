//go:build windows

package syn

import (
	"os/exec"
	"strconv"
)

// Windows has no signal to ask a process tree to wind up, and killing the
// shell we started leaves python running underneath it. taskkill /T walks
// the tree; /F is the only way it goes at all, so "terminate" and "kill"
// are the same call here.
func terminateTree(pid int) { taskkill(pid) }
func killTree(pid int)      { taskkill(pid) }

func taskkill(pid int) {
	cmd := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	hideConsole(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		Debug("taskkill %d: %v: %s", pid, err, decodeConsole(out))
	}
}

// detachFromConsole keeps a supervised app from sharing our console: a
// Ctrl+C meant for syn would otherwise go to the bot as well, and it would
// die before we could stop it tidily.
func detachFromConsole(cmd *exec.Cmd) {
	hideConsole(cmd)
	// CREATE_NEW_PROCESS_GROUP, on top of the flags hideConsole sets.
	cmd.SysProcAttr.CreationFlags |= 0x00000200
}
