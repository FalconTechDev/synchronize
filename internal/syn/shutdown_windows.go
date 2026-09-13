//go:build windows

package syn

import (
	"sync"
	"syscall"
	"unsafe"
)

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procShutdownBlockCreate      = user32.NewProc("ShutdownBlockReasonCreate")
	procShutdownBlockDestroy     = user32.NewProc("ShutdownBlockReasonDestroy")
	procGetConsoleWindow         = kernel32.NewProc("GetConsoleWindow")
	procSetConsoleCtrlHandler    = kernel32.NewProc("SetConsoleCtrlHandler")
	procSetProcessShutdownParams = kernel32.NewProc("SetProcessShutdownParameters")
	procGetConsoleProcessList    = kernel32.NewProc("GetConsoleProcessList")
	procFindWindow               = user32.NewProc("FindWindowW")
)

const (
	ctrlCEvent        = 0
	ctrlBreakEvent    = 1
	ctrlCloseEvent    = 2
	ctrlLogoffEvent   = 5
	ctrlShutdownEvent = 6
)

var (
	shutdownOnce sync.Once
	onShutdown   func()
)

// HoldShutdown asks Windows to keep the session alive while we publish. The
// reason appears on the "these apps are preventing shutdown" screen, so the
// wait is explained rather than mysterious.
func HoldShutdown(reason string) {
	hwnd := blockWindow()
	if hwnd == 0 {
		return
	}
	p, err := syscall.UTF16PtrFromString(reason)
	if err != nil {
		return
	}
	procShutdownBlockCreate.Call(hwnd, uintptr(unsafe.Pointer(p)))
}

func ReleaseShutdown() {
	hwnd := blockWindow()
	if hwnd == 0 {
		return
	}
	procShutdownBlockDestroy.Call(hwnd)
}

// OnShutdown runs fn when Windows is logging off, shutting down or closing
// our console, and blocks the sequence until fn returns. Windows still
// enforces its own timeout, so fn must be the short path: commit and push.
func OnShutdown(fn func()) {
	shutdownOnce.Do(func() {
		onShutdown = fn

		// Be among the last to go, so the repositories are still reachable.
		procSetProcessShutdownParams.Call(0x100, 0)

		// Two ways of hearing the same news. The console handler is the
		// only one available to a program someone ran in a terminal; the
		// hidden window is the only one available to the autostart build,
		// which has no console at all. Whichever arrives first does the
		// work, and the other finds nothing left to do.
		startShutdownWindow()

		cb := syscall.NewCallback(func(event uint32) uintptr {
			switch event {
			case ctrlCloseEvent, ctrlLogoffEvent, ctrlShutdownEvent:
				runShutdownHandler()
				return 1
			}
			return 0
		})
		procSetConsoleCtrlHandler.Call(cb, 1)
	})
}
