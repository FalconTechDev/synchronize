//go:build windows

package syn

import (
	"os/exec"
	"syscall"
)

// Keep the console window from flashing on every git call - it is invisible
// from a terminal but very visible from the GUI.
//
// The attributes are added to whatever is already there rather than replacing
// it: a hook arrives with its command line set by hand, and overwriting that
// would put back the quoting bug it exists to avoid.
func hideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000
}
