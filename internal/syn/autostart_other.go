//go:build !windows && !linux && !darwin

package syn

import (
	"fmt"
	"runtime"
)

var errUnsupported = fmt.Errorf("automatic startup is not implemented for %s - run `syn watch --all` yourself", runtime.GOOS)

func InstallAutostart() error { return errUnsupported }

func UninstallAutostart() error { return errUnsupported }

func AutostartStatus() string { return "  autostart: unsupported on " + runtime.GOOS + "\n" }

func StartWatcherNow() error { return errUnsupported }
