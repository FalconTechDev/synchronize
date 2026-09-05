package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/FalconTechDev/synchronize/internal/syn"
	"golang.org/x/term"
)

// confirmSecondCopy allows a second working copy of the same repository -
// two branches side by side is a real way to work - but says plainly what
// happens when both copies sit on the same branch and sync themselves.
func confirmSecondCopy(cfg *syn.Config, slug, branch string, force bool) error {
	var clashes []*syn.Repo
	for i := range cfg.Repos {
		r := &cfg.Repos[i]
		if strings.EqualFold(r.GithubSlug, slug) && syn.IsRepo(r.Path) {
			clashes = append(clashes, r)
		}
	}
	if len(clashes) == 0 {
		return nil
	}

	sameBranch := false
	for _, r := range clashes {
		if r.Branch == "" || r.Branch == branch {
			sameBranch = true
		}
	}

	fmt.Println()
	fmt.Printf("  %s is already tracked here:\n", slug)
	for _, r := range clashes {
		on := r.Branch
		if on == "" {
			on = "(current branch)"
		}
		fmt.Printf("      %-24s %s   on %s\n", r.Name, r.Path, on)
	}
	fmt.Println()

	if !sameBranch {
		fmt.Printf("  The new copy is on %s, so the two will not tread on each other.\n\n", branch)
		return nil
	}

	fmt.Printf("  The new copy would also be on %s. Both copies commit and push on\n", branch)
	fmt.Println("  their own, so each will keep pulling in what the other just wrote.")
	fmt.Println("  That works, but it makes for noisy history - a branch per copy keeps")
	fmt.Println("  them apart:  syn repo add ... --branch <name>")
	fmt.Println()

	if force {
		return nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("refusing to add a second copy on the same branch without --force")
	}

	fmt.Print("  Add it anyway? [y/N] ")
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	fmt.Println()
	if err != nil || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(answer)), "y") {
		return fmt.Errorf("cancelled")
	}
	return nil
}
