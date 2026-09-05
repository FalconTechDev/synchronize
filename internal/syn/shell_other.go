//go:build !windows

package syn

import "time"

// waitForTaskbar is a Windows concern; elsewhere the tray host is either
// there or the tray simply does not start.
func waitForTaskbar(time.Duration) bool { return true }
