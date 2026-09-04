//go:build windows

package syn

import (
	"os/exec"
	"syscall"
)

// Keep the console window from flashing on every git call - it is invisible
// from a terminal but very visible from the GUI.
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
