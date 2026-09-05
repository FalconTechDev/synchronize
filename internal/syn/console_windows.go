//go:build windows

package syn

import (
	"os"

	"golang.org/x/term"
)

// HideConsoleWindow tucks away the console the scheduler hands us. Windows
// gives every console program a window, so the watcher starting at login
// would otherwise flash one up and leave it on screen all day.
//
// The window is hidden rather than freed, because the shutdown blocker needs
// its handle to tell Windows why it is waiting.
func HideConsoleWindow() {
	if term.IsTerminal(int(os.Stdout.Fd())) {
		return // started by hand; leave the terminal alone
	}
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd == 0 {
		return
	}
	user32.NewProc("ShowWindow").Call(hwnd, 0) // SW_HIDE
}
