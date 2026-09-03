//go:build !windows && !linux && !darwin

package syn

import (
	"fmt"
	"runtime"
)

var errUnsupported = fmt.Errorf("automatic startup is not implemented for %s - run `syn watch --all` yourself", runtime.GOOS)

// InstallAutostart is unsupported on this platform.
func InstallAutostart() error { return errUnsupported }

// UninstallAutostart is unsupported on this platform.
func UninstallAutostart() error { return errUnsupported }

// AutostartStatus reports the lack of support.
func AutostartStatus() string { return "  autostart: unsupported on " + runtime.GOOS + "\n" }

// StartWatcherNow is unsupported on this platform.
func StartWatcherNow() error { return errUnsupported }
