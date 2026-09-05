//go:build !windows

package syn

import (
	"context"
	"os/exec"
)

// shellCommand hands a hook to the shell. Everywhere but Windows the
// arguments reach the child as they were written, so there is nothing to
// work around.
func shellCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", command)
}
