//go:build windows

package syn

import (
	"syscall"
	"time"
	"unsafe"
)

// waitForTaskbar holds until Windows has a notification area to put an icon
// in. At login the scheduler can start us before explorer.exe is up, and a
// tray icon created then has nowhere to go: systray fails at "unable to init
// instance" and the watcher would exit with it. Waiting costs nothing when
// the desktop is already there.
func waitForTaskbar(limit time.Duration) bool {
	class, err := syscall.UTF16PtrFromString("Shell_TrayWnd")
	if err != nil {
		return true
	}
	deadline := time.Now().Add(limit)
	for {
		hwnd, _, _ := procFindWindow.Call(uintptr(unsafe.Pointer(class)), 0)
		if hwnd != 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(2 * time.Second)
	}
}
