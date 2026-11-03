//go:build windows

package syn

import (
	"fmt"
	"os/exec"
	"os/user"
	"strings"
)

// makePrivate leaves a file readable by its owner and nobody else.
//
// The 0600 passed to WriteFile means nothing here: Windows has no such mode
// bits and keeps only a read-only flag. What decides who can open the file is
// its ACL, and a new file inherits that from the folder around it - which
// gives every other account in the folder's list full control. On a machine
// with a second user that is them, and in a shared location it can be a whole
// group. The files this is used on hold a GitHub token and a Telegram token.
//
// It is a hardening step, not a requirement: if icacls is missing or refuses,
// the file is still written and the caller carries on.
func makePrivate(path string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	// The SID rather than the name, which is "DOMAIN\user" in one place and
	// localised in another, and is not always what icacls resolves.
	cmd := exec.Command("icacls", path, "/inheritance:r", "/grant:r", "*"+u.Uid+":(F)")
	hideConsole(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("icacls: %s", strings.TrimSpace(decodeConsole(out)))
	}
	return nil
}
