//go:build windows

package syn

import (
	"context"
	"os/exec"
	"syscall"
)

// shellCommand hands a hook to cmd.exe without letting Go quote it first.
//
// exec.Command builds the child's command line by Windows' usual quoting
// rules, which escape an embedded quote as \" - and cmd.exe does not read
// backslash-escapes. A hook as ordinary as
//
//	echo %SYN_EVENT% > "C:\Program Files\log.txt"
//
// therefore reached cmd as something it refused to parse, and the hook
// failed with a syntax error about the filename. Setting CmdLine passes the
// line through untouched; /s tells cmd to strip only the outermost pair of
// quotes and leave the rest alone, which is what makes wrapping the whole
// command in quotes safe.
func shellCommand(ctx context.Context, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `/s /c "` + command + `"`}
	return cmd
}
