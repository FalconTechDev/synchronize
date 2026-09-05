package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FalconTechDev/synchronize/internal/syn"
	"golang.org/x/term"
)

// cmdPath prints one path and nothing else, so `cd $(syn path my-repo)` works.
func cmdPath(cfg *syn.Config, name string) error {
	if name == "" {
		return fmt.Errorf("which repository?\n%s", repoListHint(cfg))
	}
	r := cfg.Find(name)
	if r == nil {
		return fmt.Errorf("unknown repository %q\n%s", name, repoListHint(cfg))
	}
	fmt.Println(r.Path)
	return nil
}

// cmdUndo takes back the last commit syn made on your behalf. Backups and
// restore are for conflicts; this is for "it published a draft, put it back".
func cmdUndo(e *syn.Engine, cfg *syn.Config, name string, force, yes bool) error {
	if name == "" {
		return fmt.Errorf("which repository?\n%s", repoListHint(cfg))
	}
	r := cfg.Find(name)
	if r == nil {
		return fmt.Errorf("unknown repository %q\n%s", name, repoListHint(cfg))
	}

	subject, err := syn.LastCommitSubject(r.Path)
	if err != nil {
		return err
	}
	if !syn.IsAutoCommit(subject, cfg.CommitTemplate) && !force {
		return fmt.Errorf("the last commit was not made by syn:\n"+
			"        %s\n"+
			"        undo only takes back syn's own commits; use git if you meant that one\n"+
			"        or pass --force if you are sure", subject)
	}

	pushed, err := syn.IsCommitOnRemote(r.Path, r.Remote)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Printf("  Taking back:  %s\n", subject)
	fmt.Printf("  In:           %s\n", r.Path)
	if pushed {
		fmt.Println()
		fmt.Println("  It is already on the remote, so undoing means rewriting published")
		fmt.Println("  history. Another machine that pulled it will conflict.")
	}
	fmt.Println()
	fmt.Println("  Your files are left exactly as they are - only the commit goes.")
	fmt.Println()

	// --force waves through the "this was not syn's commit" guard as well;
	// --yes only answers the question, so a script still cannot rewrite a
	// commit it was never meant to touch.
	if !force && !yes && !confirm("  Undo it? [y/N] ") {
		return fmt.Errorf("cancelled")
	}
	return e.UndoLastCommit(r, pushed)
}

// cmdPause silences automatic syncing for a while, rather than for good.
func cmdPause(cfg *syn.Config, rest []string) error {
	if len(rest) == 0 {
		return showPaused(cfg)
	}
	r := cfg.Find(rest[0])
	if r == nil {
		return fmt.Errorf("unknown repository %q\n%s", rest[0], repoListHint(cfg))
	}

	if len(rest) < 2 {
		return fmt.Errorf("how long? e.g. `syn pause %s 2h`, or `syn pause %s off`", r.Name, r.Name)
	}
	if rest[1] == "off" || rest[1] == "0" {
		r.PausedUntil = time.Time{}
		if err := cfg.Save(); err != nil {
			return err
		}
		syn.Info("%s: syncing by itself again", r.Name)
		return nil
	}

	d, err := time.ParseDuration(strings.ToLower(rest[1]))
	if err != nil || d <= 0 {
		return fmt.Errorf("say how long, like 30m, 2h or 1d - not %q", rest[1])
	}
	r.PausedUntil = time.Now().Add(d)
	if err := cfg.Save(); err != nil {
		return err
	}
	syn.Info("%s: left out of automatic syncing until %s",
		r.Name, r.PausedUntil.Format("15:04 on 2 Jan"))
	fmt.Println("  Commands you type yourself still work as normal.")
	return nil
}

func showPaused(cfg *syn.Config) error {
	any := false
	for i := range cfg.Repos {
		r := &cfg.Repos[i]
		if r.PausedUntil.IsZero() {
			continue
		}
		any = true
		if time.Now().After(r.PausedUntil) {
			fmt.Printf("  %-24s pause has expired\n", r.Name)
		} else {
			fmt.Printf("  %-24s until %s\n", r.Name, r.PausedUntil.Format("15:04 on 2 Jan"))
		}
	}
	if !any {
		fmt.Println("  Nothing is paused.")
		fmt.Println()
		fmt.Println("  syn pause <repo> 2h     leave it out of automatic syncing for a while")
		fmt.Println("  syn pause <repo> off    bring it back now")
	}
	return nil
}

// cmdIgnore keeps files out of an automatic commit while leaving them on disk.
// The file rules decide what is checked out; this decides what gets committed.
func cmdIgnore(cfg *syn.Config, rest []string) error {
	if len(rest) == 0 {
		return fmt.Errorf("usage: syn ignore <repo> <glob>, or syn ignore <repo> to see the list")
	}
	r := cfg.Find(rest[0])
	if r == nil {
		return fmt.Errorf("unknown repository %q\n%s", rest[0], repoListHint(cfg))
	}

	if len(rest) == 1 {
		if len(r.NoCommit) == 0 {
			fmt.Printf("  %s: nothing is held back from automatic commits\n\n", r.Name)
			fmt.Println("  syn ignore <repo> \"*.env\"      hold matching files back")
			fmt.Println("  syn ignore <repo> clear         commit everything again")
			return nil
		}
		fmt.Printf("  %s: held back from automatic commits\n", r.Name)
		for _, p := range r.NoCommit {
			fmt.Printf("    %s\n", p)
		}
		fmt.Println()
		fmt.Println("  They stay on disk and never leave this machine by themselves.")
		return nil
	}

	if rest[1] == "clear" {
		r.NoCommit = nil
		if err := cfg.Save(); err != nil {
			return err
		}
		syn.Info("%s: automatic commits take everything again", r.Name)
		return nil
	}

	for _, p := range rest[1:] {
		r.NoCommit = appendUniq(r.NoCommit, p)
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	syn.Info("%s: %s held back from automatic commits", r.Name, strings.Join(rest[1:], ", "))
	return nil
}

func repoListHint(cfg *syn.Config) string {
	if len(cfg.Repos) == 0 {
		return "        nothing is being tracked yet - try: syn repo add owner/name"
	}
	names := make([]string, 0, len(cfg.Repos))
	for i := range cfg.Repos {
		names = append(names, cfg.Repos[i].Name)
	}
	return "        pick one of: " + strings.Join(names, ", ")
}

// dryRun says what a sync would commit and publish, without doing any of it.
// For a command that commits on your behalf, being able to look first is
// cheap reassurance.
func dryRun(e *syn.Engine, repos []*syn.Repo) error {
	for _, r := range repos {
		st := e.Status(r, true)
		fmt.Println(r.Name)

		if st.Err != nil {
			fmt.Printf("  would fail: %v\n\n", st.Err)
			continue
		}
		if st.Conflict != nil {
			fmt.Print("  would do nothing: there is a conflict to settle first\n\n")
			continue
		}

		did := false
		if files := syn.WouldCommit(r); len(files) > 0 {
			did = true
			fmt.Printf("  would commit %s:\n", plural(len(files), "file"))
			for _, f := range files {
				fmt.Printf("      %-9s %s\n", f.Status, f.Path)
			}
		}
		if held := syn.WouldHoldBack(r); len(held) > 0 {
			fmt.Println("  would leave alone (syn ignore):")
			for _, f := range held {
				fmt.Printf("      %-9s %s\n", f.Status, f.Path)
			}
		}
		if st.Behind > 0 {
			did = true
			fmt.Printf("  would pull %s from %s/%s\n", plural(st.Behind, "commit"), r.Remote, st.Branch)
		}
		if st.Ahead > 0 {
			did = true
			fmt.Printf("  would push %s to %s/%s\n", plural(st.Ahead, "commit"), r.Remote, st.Branch)
		}
		if !did {
			fmt.Println("  nothing to do")
		}
		fmt.Println()
	}
	return nil
}

// confirm asks, and takes silence for a no. With nobody at the keyboard
// there is no one to ask, so it refuses rather than guessing - the commands
// that need to work unattended take --yes instead.
func confirm(prompt string) bool {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Println("  (nothing to answer with - pass --yes to go ahead unattended)")
		return false
	}
	fmt.Print(prompt)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	fmt.Println()
	return err == nil && strings.HasPrefix(strings.ToLower(strings.TrimSpace(answer)), "y")
}

// looksLikeURL tells a GitHub slug from somewhere else to clone from.
func looksLikeURL(s string) bool {
	for _, p := range []string{"http://", "https://", "ssh://", "git://", "file://"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return strings.Contains(s, "@") && strings.Contains(s, ":") // git@host:path
}

// cmdRepoAddURL clones from any git remote. There is no API to ask about the
// default branch, so git is asked instead.
func cmdRepoAddURL(cfg *syn.Config, url string, values map[string]string) error {
	name := syn.RepoNameFromURL(url)
	if name == "" {
		return fmt.Errorf("could not work out a name from %s - pass --path to choose the folder", url)
	}

	dest := values["--path"]
	if dest == "" {
		dest = "./" + name
	}
	abs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if existing := cfg.Find(abs); existing != nil && syn.IsRepo(abs) {
		return fmt.Errorf("already registered as %q", existing.Name)
	}

	if !syn.IsRepo(abs) {
		if chosen, err := confirmDestination(url, abs); err != nil {
			return err
		} else if chosen != "" {
			abs = chosen
		}
		syn.Info("cloning %s...", url)
		if err := syn.CloneWithToken(url, abs, "", "--quiet"); err != nil {
			return fmt.Errorf("clone failed: %w", err)
		}
		syn.Info("cloned to %s", abs)
	}

	branch := values["--branch"]
	if branch == "" {
		branch, _ = syn.CurrentBranch(abs)
	}
	interval := 0
	if v := values["--interval"]; v != "" {
		if n, err := parseMinutes(v); err == nil && n > 0 {
			interval = n
		}
	}

	// Work the name out once: asking again after appending would see the
	// entry we just added and hand back a "-2".
	name = uniqueName(cfg, name)

	cfg.Repos = append(cfg.Repos, syn.Repo{
		Name:            name,
		Path:            abs,
		Remote:          "origin",
		Branch:          branch,
		Enabled:         true,
		IntervalMinutes: interval,
	})
	if err := cfg.Save(); err != nil {
		return err
	}
	syn.Info("registered %q (%s, branch %s)", name, url, branch)
	syn.Info("git handles the credentials for this remote, as it would for any clone")
	return nil
}
