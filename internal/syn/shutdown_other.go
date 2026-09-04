//go:build !windows

package syn

func HoldShutdown(reason string) {}
func ReleaseShutdown()           {}
func OnShutdown(fn func())       {}
