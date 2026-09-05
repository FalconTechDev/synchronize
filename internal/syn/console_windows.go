//go:build windows

package syn

import "unsafe"

// HideConsoleWindow tucks away the console the scheduler hands us. Windows
// gives every console program a window, so the watcher starting at login
// would otherwise flash one up and leave it on screen all day.
//
// The window is hidden rather than freed, because the shutdown blocker needs
// its handle to tell Windows why it is waiting.
func HideConsoleWindow() {
	if !ownsConsole() {
		return // started from a shell that was already there; leave it alone
	}
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd == 0 {
		return
	}
	user32.NewProc("ShowWindow").Call(hwnd, 0) // SW_HIDE
}

// ownsConsole says whether this console exists only for us. Asking whether
// stdout is a terminal is not enough: the scheduler gives its task a real
// console too, and then the window stays on screen all day. What tells the
// two apart is who else is attached - a console opened for us has exactly
// one process in it, ours; a shell's console has at least the shell as well.
func ownsConsole() bool {
	var pids [8]uint32
	n, _, _ := procGetConsoleProcessList.Call(
		uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	return n == 1
}
