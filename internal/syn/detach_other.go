//go:build !windows

package syn

// DetachIfWindowed is a Windows concern. Elsewhere a background process is
// started by the service manager without a terminal in the first place.
func DetachIfWindowed() bool { return false }
