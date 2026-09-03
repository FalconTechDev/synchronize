package syn

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ApplySparseCheckout writes .git/info/sparse-checkout and enables or disables
// sparse mode depending on whether the repo has any Include patterns.
//
// Rules:
//   - If both Include and Exclude are empty, sparse mode is disabled: all files
//     are shown (the normal git behaviour).
//   - If Include is non-empty, only those paths/globs are checked out.
//     Exclude patterns are appended with a leading "!" and take priority.
//   - Special value "*" in Include means "everything except the excludes".
func ApplySparseCheckout(r *Repo) error {
	if len(r.Include) == 0 && len(r.Exclude) == 0 {
		// Disable sparse mode so nothing is hidden.
		if gitOK(r.Path, "sparse-checkout", "disable") {
			Debug("%s: sparse-checkout disabled (all files visible)", r.Name)
		}
		return nil
	}

	var lines []string
	if len(r.Include) == 0 {
		lines = append(lines, "/*") // start from "everything"
	} else {
		for _, p := range r.Include {
			if !strings.HasPrefix(p, "/") && !strings.Contains(p, "/") {
				p = "/" + p // anchor top-level patterns
			}
			lines = append(lines, p)
		}
	}
	for _, p := range r.Exclude {
		if !strings.HasPrefix(p, "!") {
			p = "!" + p
		}
		if !strings.HasPrefix(p, "!/") && !strings.Contains(p, "/") {
			p = "!/" + p[1:]
		}
		lines = append(lines, p)
	}

	// Write the patterns file directly so we can use cone=false (full patterns).
	sparseDir := filepath.Join(r.Path, ".git", "info")
	if err := os.MkdirAll(sparseDir, 0o755); err != nil {
		return err
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(sparseDir, "sparse-checkout"), []byte(content), 0o644); err != nil {
		return err
	}
	if _, err := git(r.Path, "sparse-checkout", "init", "--no-cone"); err != nil {
		return fmt.Errorf("could not enable sparse-checkout: %w", err)
	}
	if _, err := git(r.Path, "checkout"); err != nil {
		// A checkout error usually just means the index needs rewriting; not fatal.
		Debug("%s: sparse-checkout apply: %v", r.Name, err)
	}
	Debug("%s: sparse-checkout set (%d include, %d exclude patterns)", r.Name, len(r.Include), len(r.Exclude))
	return nil
}

// SparseStatus reports the active sparse-checkout patterns for a repo.
func SparseStatus(r *Repo) string {
	out, err := git(r.Path, "sparse-checkout", "list")
	if err != nil || out == "" {
		return "(sparse-checkout not active — all files visible)"
	}
	return out
}
