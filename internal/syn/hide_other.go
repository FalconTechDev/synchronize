//go:build !windows

package syn

import "os/exec"

func hideConsole(cmd *exec.Cmd) {}
