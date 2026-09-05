package main

import (
	"fmt"
	"strings"

	"github.com/FalconTechDev/synchronize/internal/syn"
)

// cmdApp sets the program syn runs out of a repository and restarts when its
// code arrives. `syn restart` calls somebody else's service manager; this is
// for when there is not one - a bot on a small box, say.
func cmdApp(cfg *syn.Config, e *syn.Engine, rest []string, watch, dir string) error {
	if len(rest) == 0 {
		return listApps(cfg)
	}
	r := cfg.Find(rest[0])
	if r == nil {
		return fmt.Errorf("unknown repository %q\n%s", rest[0], repoListHint(cfg))
	}
	if len(rest) == 1 {
		return showApp(r)
	}

	switch rest[1] {
	case "off", "clear", "disable":
		r.App = nil
		if err := cfg.Save(); err != nil {
			return err
		}
		syn.Info("%s: no longer runs anything (a running watcher will stop it)", r.Name)
		return nil

	case "start", "run":
		return e.RunAppInForeground(r)
	}

	// Everything after `--` is the command, taken verbatim. The separator
	// itself is lifted out of the flag parsing further up and arrives here
	// still attached, so drop it before it becomes part of what runs.
	words := rest[1:]
	if words[0] == "--" {
		words = words[1:]
	}
	command := strings.TrimSpace(strings.Join(words, " "))
	if command == "" {
		return fmt.Errorf("give the command to run after --\n"+
			"  syn app %s -- python bot.py", r.Name)
	}
	r.App = &syn.AppConfig{
		Command: command,
		Paths:   splitPaths(watch),
		Dir:     dir,
	}
	if err := cfg.Save(); err != nil {
		return err
	}

	syn.Info("%s: runs %s", r.Name, command)
	if len(r.App.Paths) > 0 {
		syn.Info("       restarted when these change: %s", strings.Join(r.App.Paths, ", "))
	} else {
		syn.Info("       restarted whenever an update arrives")
	}
	syn.Info("       the watcher starts it:  syn watch %s", r.Name)
	syn.Info("       or try it here first:   syn app %s start", r.Name)
	return nil
}

func showApp(r *syn.Repo) error {
	if r.App == nil || r.App.Command == "" {
		fmt.Printf("%s runs nothing.\n\n", r.Name)
		fmt.Printf("  syn app %s -- python bot.py\n", r.Name)
		fmt.Printf("  syn app %s --watch \"bot.py,src/**\" -- python bot.py\n", r.Name)
		return nil
	}
	fmt.Printf("%s\n", r.Name)
	fmt.Printf("  runs:      %s\n", r.App.Command)
	if r.App.Dir != "" {
		fmt.Printf("  in:        %s\n", r.App.Dir)
	}
	if len(r.App.Paths) > 0 {
		fmt.Printf("  restarts:  when %s change\n", strings.Join(r.App.Paths, ", "))
	} else {
		fmt.Printf("  restarts:  on any update\n")
	}
	return nil
}

func listApps(cfg *syn.Config) error {
	any := false
	for i := range cfg.Repos {
		r := &cfg.Repos[i]
		if r.App == nil || r.App.Command == "" {
			continue
		}
		any = true
		when := "any update"
		if len(r.App.Paths) > 0 {
			when = strings.Join(r.App.Paths, ", ")
		}
		fmt.Printf("%s  runs %s  (restarts on %s)\n", r.Name, r.App.Command, when)
	}
	if !any {
		fmt.Println("syn is not running anything.")
		fmt.Println()
		fmt.Println("Have it run your program and restart it when the code arrives:")
		fmt.Println()
		fmt.Println("  syn app my-bot -- python bot.py")
		fmt.Println()
		fmt.Println("The command is kept on this machine, never in git. It runs while")
		fmt.Println("the watcher runs, so start that too:  syn watch my-bot")
	}
	return nil
}
