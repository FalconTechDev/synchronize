package syn

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ApplySparseCheckout(r *Repo) error {
	if len(r.Include) == 0 && len(r.Exclude) == 0 {
		
		if gitOK(r.Path, "sparse-checkout", "disable") {
			Debug("%s: sparse-checkout disabled (all files visible)", r.Name)
		}
		return nil
	}

	var lines []string
	if len(r.Include) == 0 {
		lines = append(lines, "/*") 
	} else {
		for _, p := range r.Include {
			if !strings.HasPrefix(p, "/") && !strings.Contains(p, "/") {
				p = "/" + p 
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
		
		Debug("%s: sparse-checkout apply: %v", r.Name, err)
	}
	Debug("%s: sparse-checkout set (%d include, %d exclude patterns)", r.Name, len(r.Include), len(r.Exclude))
	return nil
}

func SparseStatus(r *Repo) string {
	out, err := git(r.Path, "sparse-checkout", "list")
	if err != nil || out == "" {
		return "(sparse-checkout not active — all files visible)"
	}
	return out
}
