//go:build windows

package syn

import (
	"syscall"
	"unicode/utf8"
	"unsafe"
)

var (
	procGetConsoleOutputCP  = kernel32.NewProc("GetConsoleOutputCP")
	procGetOEMCP            = kernel32.NewProc("GetOEMCP")
	procMultiByteToWideChar = kernel32.NewProc("MultiByteToWideChar")
)

// decodeConsole turns what a child process printed into text we can put in
// the log.
//
// cmd.exe and the tools it runs write in the console's OEM code page - CP866
// on a Russian Windows, CP437 on an American one - while the log file is
// UTF-8. Copying the bytes across unchanged turned every non-ASCII error
// message into mojibake, which for a tool that only ever talks to you through
// its log is worse than the failure it was trying to report: you could see
// that a hook broke, but not why.
//
// Output that is already valid UTF-8 is left alone. A hook may well be a
// program of yours that writes UTF-8 regardless of the console setting, and
// decoding that as OEM would break what currently works. ASCII is valid UTF-8,
// so the common case takes this path and never touches Windows at all.
func decodeConsole(b []byte) string {
	if len(b) == 0 || utf8.Valid(b) {
		return string(b)
	}
	cp, _, _ := procGetConsoleOutputCP.Call()
	if cp == 0 {
		// No console is attached (a hidden autostart, a service, CI): the child still
		// writes in the system OEM code page, so use that rather than giving up.
		cp, _, _ = procGetOEMCP.Call()
	}
	if cp == 0 {
		return string(b)
	}
	s, ok := fromCodePage(uint32(cp), b)
	if !ok {
		return string(b)
	}
	return s
}

func fromCodePage(cp uint32, b []byte) (string, bool) {
	const mbErrInvalidChars = 0x00000008

	// Ask how much room the wide string needs, then convert into it.
	n, _, _ := procMultiByteToWideChar.Call(
		uintptr(cp), mbErrInvalidChars,
		uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)),
		0, 0)
	if n == 0 {
		return "", false
	}
	buf := make([]uint16, n)
	n, _, _ = procMultiByteToWideChar.Call(
		uintptr(cp), mbErrInvalidChars,
		uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)),
		uintptr(unsafe.Pointer(&buf[0])), n)
	if n == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf[:n]), true
}
