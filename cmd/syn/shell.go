package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/FalconTechDev/synchronize/internal/syn"
)

const (
	cReset = "\033[0m"
	cDim   = "\033[38;5;244m"
	cBold  = "\033[1m"
	cAccent = "\033[38;5;141m"
	cOK    = "\033[38;5;78m"
	cWarn  = "\033[38;5;179m"
	cErr   = "\033[38;5;203m"
	cInfo  = "\033[38;5;75m"
)

var shellTopics = []struct {
	Title    string
	Commands [][2]string
}{
	{"Syncing", [][2]string{
		{"pull [repo]", "bring remote changes in"},
		{"push [repo]", "commit and publish local work"},
		{"sync [repo]", "pull, then push"},
		{"status [repo]", "show what would happen"},
		{"diff [repo]", "show uncommitted changes"},
	}},
	{"Repositories", [][2]string{
		{"ls", "list registered repositories"},
		{"init [path]", "register the repo at path"},
		{"repo add <owner/repo>", "clone from GitHub and register"},
		{"enable / disable <repo>", "include or exclude from --all"},
		{"remove <repo>", "stop tracking (files untouched)"},
		{"set <repo> <k> <v>", "change name, remote, branch, interval"},
	}},
	{"Hooks", [][2]string{
		{"hook ls", "show configured hooks"},
		{"hook add <repo> <event> <cmd>", "run a command after a sync"},
		{"hook run <repo> <event>", "try a hook right now"},
		{"hook clear <repo> [event]", "remove hooks"},
	}},
	{"Conflicts", [][2]string{
		{"resolve <repo>", "replay the conflict into your tree"},
		{"resolve <repo> --ours", "keep your side everywhere"},
		{"resolve <repo> --theirs", "keep the remote side"},
		{"backups <repo>", "list safety branches"},
	}},
	{"This session", [][2]string{
		{"help", "show this list"},
		{"log [n]", "recent log lines"},
		{"clear", "clear the screen"},
		{"exit", "leave the shell"},
	}},
}

func cmdShell() error {
	fmt.Print("\033[2J\033[H")
	printBanner()
	printOverview()
	fmt.Printf("\n%sType a command, %shelp%s%s for the list, %sexit%s%s to leave.%s\n\n",
		cDim, cBold, cReset, cDim, cBold, cReset, cDim, cReset)

	ed := newEditor()
	commands := shellCommands()

	for {
		complete := func(partial string) ([]string, string) {
			return completeLine(partial, commands, repoNames())
		}
		line, ok := ed.Read(prompt(), complete)
		if !ok {
			fmt.Printf("%sbye%s\n", cDim, cReset)
			return nil
		}
		if line == "" {
			continue
		}

		args, err := splitArgs(line)
		if err != nil {
			fmt.Printf("%s%v%s\n\n", cErr, err, cReset)
			continue
		}

		switch args[0] {
		case "exit", "quit", "q", ":q":
			fmt.Printf("%sbye%s\n", cDim, cReset)
			return nil
		case "clear", "cls":
			fmt.Print("\033[2J\033[H")
			printBanner()
			continue
		case "help", "?", "h":
			printHelp()
			continue
		case "ls":
			args = []string{"list"}
		case "shell":
			fmt.Printf("%salready in the shell%s\n\n", cDim, cReset)
			continue
		case "watch", "serve", "install", "uninstall", "start", "tray":
			fmt.Printf("%s%s runs until stopped - start it from a normal terminal instead:%s\n",
				cWarn, args[0], cReset)
			fmt.Printf("  syn %s\n\n", strings.Join(args, " "))
			continue
		}

		if err := run(args); err != nil {
			syn.Errorf("%v", err)
		}
		fmt.Println()
	}
}

func prompt() string {
	cfg, err := syn.LoadConfig()
	if err != nil {
		return fmt.Sprintf("%ssyn%s > ", cAccent, cReset)
	}

	state := syn.LoadState()
	attention := 0
	for i := range cfg.Repos {
		if _, bad := state.Conflicts[cfg.Repos[i].Name]; bad {
			attention++
		}
	}
	if attention > 0 {
		return fmt.Sprintf("%ssyn%s %s(%d needs attention)%s > ",
			cAccent, cReset, cErr, attention, cReset)
	}
	return fmt.Sprintf("%ssyn%s > ", cAccent, cReset)
}

func printBanner() {
	fmt.Printf("%s%s  synchronize%s %s%s%s\n",
		cBold, cAccent, cReset, cDim, Version, cReset)
	fmt.Printf("%s  automatic git sync across your machines%s\n", cDim, cReset)
}

func printOverview() {
	cfg, err := syn.LoadConfig()
	if err != nil {
		fmt.Printf("\n%s  could not read the config: %v%s\n", cErr, err, cReset)
		return
	}
	if len(cfg.Repos) == 0 {
		fmt.Printf("\n%s  No repositories yet.%s\n", cDim, cReset)
		fmt.Printf("%s  Run %sinit%s%s inside a git repo, or %srepo add owner/name%s%s.%s\n",
			cDim, cBold, cReset, cDim, cBold, cReset, cDim, cReset)
		return
	}

	engine := syn.NewEngine(cfg)
	fmt.Println()
	for i := range cfg.Repos {
		r := &cfg.Repos[i]
		st := engine.Status(r, false)

		mark, colour := "ok", cOK
		switch {
		case st.Err != nil:
			mark, colour = "error", cErr
		case st.Conflict != nil:
			mark, colour = "conflict", cErr
		case st.Behind > 0:
			mark, colour = fmt.Sprintf("%d to pull", st.Behind), cInfo
		case st.Ahead > 0:
			mark, colour = fmt.Sprintf("%d to push", st.Ahead), cAccent
		case st.Dirty:
			mark, colour = fmt.Sprintf("%d uncommitted", len(st.Changed)), cWarn
		case !r.Enabled:
			mark, colour = "paused", cDim
		}

		when := "never synced"
		if !st.LastSync.IsZero() {
			when = "synced " + since(st.LastSync)
		}
		fmt.Printf("  %s%-26s%s %s%-16s%s %s%s%s\n",
			cBold, truncate(r.Name, 26), cReset,
			colour, mark, cReset,
			cDim, when, cReset)
	}
}

func printHelp() {
	fmt.Println()
	for _, topic := range shellTopics {
		fmt.Printf("%s%s%s\n", cBold, topic.Title, cReset)
		for _, c := range topic.Commands {
			fmt.Printf("  %s%-30s%s %s%s%s\n",
				cAccent, c[0], cReset, cDim, c[1], cReset)
		}
		fmt.Println()
	}
	fmt.Printf("%sEverything the syn command accepts works here too.%s\n\n", cDim, cReset)
}

func since(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func splitArgs(line string) ([]string, error) {
	var args []string
	var cur strings.Builder
	quote := rune(0)
	escaped := false

	for _, r := range line {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\' && quote != '\'':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
		case r == ' ' || r == '\t':
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unbalanced %c quote", quote)
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return args, nil
}

func repoNames() []string {
	cfg, err := syn.LoadConfig()
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(cfg.Repos))
	for i := range cfg.Repos {
		out = append(out, cfg.Repos[i].Name)
	}
	return out
}

func shellCommands() []string {
	return []string{
		"pull", "push", "sync", "status", "diff", "ls", "list", "init", "repo",
		"enable", "disable", "remove", "set", "files", "hook", "resolve",
		"backups", "restore", "prune", "log", "notify", "config", "doctor",
		"version", "auth", "help", "clear", "exit",
	}
}
