//go:build !windows

package syn

// makePrivate is a no-op where the 0600 already given to WriteFile does the
// job.
func makePrivate(string) error { return nil }
