//go:build windows

package syn

import (
	"os/user"

	"golang.org/x/sys/windows"
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
// It is a hardening step, not a requirement: if the ACL cannot be written, the
// file is still there and the caller carries on.
func makePrivate(path string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	// The SID rather than the name, which is "DOMAIN\user" in one place and
	// localised in another.
	sid, err := windows.StringToSid(u.Uid)
	if err != nil {
		return err
	}
	// A fresh ACL holding one entry, written as protected so nothing is inherited.
	// Removing inheritance alone is not enough: an elevated process creates the file
	// with SYSTEM and the Administrators group already listed explicitly, and those
	// stay unless the whole list is replaced.
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.SET_ACCESS,
		Inheritance:       windows.NO_INHERITANCE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}}, nil)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}
