//go:build !windows

package syn

// decodeConsole is a Windows concern: everywhere else a child process writes
// UTF-8 and the bytes go straight through.
func decodeConsole(b []byte) string { return string(b) }
