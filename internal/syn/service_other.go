//go:build !linux

package syn

import (
	"fmt"
	"runtime"
)

// ServiceOptions is the same shape everywhere so the command that fills it
// in does not have to be built per platform.
type ServiceOptions struct {
	User string
	Home string
}

// ServiceSupported is false off Linux. A Windows server already has the
// scheduled task, which runs from boot and survives a reboot; macOS is a
// workstation as far as this tool is concerned.
func ServiceSupported() bool { return false }

func unsupported() error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("there is no separate service to install on Windows\n" +
			"        the scheduled task already runs from boot:  syn auto on\n" +
			"        for a machine nobody logs into, set the task to run whether\n" +
			"        the user is signed in or not, in Task Scheduler")
	}
	return fmt.Errorf("a system service is a Linux thing here; on %s use:  syn auto on", runtime.GOOS)
}

func InstallService(ServiceOptions) error { return unsupported() }
func UninstallService() error             { return unsupported() }
func ServiceStatus() string               { return "not supported on " + runtime.GOOS }
