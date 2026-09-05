package main

import (
	"fmt"
	"strings"

	"github.com/FalconTechDev/synchronize/internal/syn"
)

func cmdRestart(cfg *syn.Config, e *syn.Engine, rest []string, systemd string, noRollback bool) error {
	if len(rest) == 0 {
		return listRestarts(cfg)
	}

	r := cfg.Find(rest[0])
	if r == nil {
		return fmt.Errorf("unknown repository %q\n%s", rest[0], repoListHint(cfg))
	}

	if len(rest) == 1 {
		return showRestart(r)
	}

	switch rest[1] {
	case "off", "clear", "disable":
		r.Restart = nil
		if err := cfg.Save(); err != nil {
			return err
		}
		syn.Info("%s: restart disabled", r.Name)
		return nil

	case "run", "test":
		if r.Restart == nil || r.Restart.Command == "" {
			return fmt.Errorf("%s has no restart configured", r.Name)
		}
		syn.Info("%s: running restart command now", r.Name)
		for _, run := range e.RunRestartNow(r) {
			if run.Err != nil {
				return fmt.Errorf("restart failed: %s: %w", run.Command, run.Err)
			}
		}
		return nil
	}

	paths, cmd := parseRestartArgs(rest[1:])
	if systemd != "" {
		cmd = "systemctl restart " + systemd
	}
	rollback := !noRollback
	if len(paths) == 0 {
		return fmt.Errorf("give at least one file pattern (e.g. bot.py, src/**)")
	}
	if cmd == "" {
		return fmt.Errorf("give a restart command after --, or use --systemd <name>\n"+
			"  syn restart %s bot.py -- systemctl restart my-bot\n"+
			"  syn restart %s bot.py --systemd my-bot", r.Name, r.Name)
	}

	r.Restart = &syn.RestartConfig{
		Paths:    paths,
		Command:  cmd,
		Rollback: rollback,
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	syn.Info("%s: after pull, when %s change: %s", r.Name, strings.Join(paths, ", "), cmd)
	if rollback {
		syn.Info("       if the command fails, the pull will be rolled back")
	}
	return nil
}

// parseRestartArgs splits the file patterns from the command that follows a
// `--`. The command arrives verbatim, so `systemctl restart x --now` keeps
// its own flags instead of syn reading them as its own.
func parseRestartArgs(args []string) (paths []string, command string) {
	for i, a := range args {
		if a == "--" {
			return paths, strings.TrimSpace(strings.Join(args[i+1:], " "))
		}
		paths = append(paths, a)
	}
	return paths, ""
}

func showRestart(r *syn.Repo) error {
	if r.Restart == nil || r.Restart.Command == "" {
		fmt.Printf("%s: no restart configured\n", r.Name)
		fmt.Println()
		fmt.Println("  syn restart", r.Name, "bot.py -- systemctl restart my-bot")
		fmt.Println("  syn restart", r.Name, "bot.py --systemd my-bot")
		return nil
	}
	rc := r.Restart
	fmt.Printf("%s: when %s change\n", r.Name, strings.Join(rc.Paths, ", "))
	fmt.Printf("  run:  %s\n", rc.Command)
	if rc.Rollback {
		fmt.Println("  rollback on failure: yes")
	} else {
		fmt.Println("  rollback on failure: no")
	}
	return nil
}

func listRestarts(cfg *syn.Config) error {
	any := false
	for i := range cfg.Repos {
		r := &cfg.Repos[i]
		if r.Restart == nil || r.Restart.Command == "" {
			continue
		}
		any = true
		rc := r.Restart
		fmt.Printf("%s  when %s change → %s\n", r.Name, strings.Join(rc.Paths, ", "), rc.Command)
	}
	if !any {
		fmt.Println("No restart configured.")
		fmt.Println()
		fmt.Println("On a server, restart a service when specific files arrive:")
		fmt.Println()
		fmt.Println(`  syn restart my-bot bot.py -- systemctl restart my-bot`)
		fmt.Println(`  syn restart my-bot bot.py --systemd my-bot`)
		fmt.Println()
		fmt.Println("The command is stored on this machine only - never in git.")
	}
	return nil
}
