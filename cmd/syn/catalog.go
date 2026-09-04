package main

import "sort"

// cmdInfo is the single description of a command: what it takes, what it does,
// and where it belongs in the help. The hints under the prompt and the help
// listing are both built from this, so they cannot drift apart.
type cmdInfo struct {
	Args  string
	Desc  string
	Group string
}

var catalog = map[string]cmdInfo{
	// Syncing
	"pull":   {"<repo|all> [--force]", "bring remote changes in (--force resets to the remote)", "Syncing"},
	"push":   {"<repo|all> [--force]", "commit local work and publish it", "Syncing"},
	"sync":   {"<repo|all>", "pull, then push", "Syncing"},
	"status": {"<repo|all> [--fetch]", "show what a sync would do", "Syncing"},
	"diff":   {"<repo|all>", "show uncommitted changes", "Syncing"},

	// Repositories
	"ls":      {"", "list the repositories syn tracks", "Repositories"},
	"list":    {"", "list the repositories syn tracks", "Repositories"},
	"init":    {"[path]", "track the git repo at path (default: here)", "Repositories"},
	"repo":    {"<add|ls>", "clone from GitHub, or list what is there", "Repositories"},
	"enable":  {"<repo>", "include it when you say all", "Repositories"},
	"disable": {"<repo>", "leave it out when you say all", "Repositories"},
	"remove":  {"<repo>", "stop tracking it; the files stay put", "Repositories"},
	"set":     {"<repo> <name|remote|branch|interval> <value>", "change a repository setting", "Repositories"},
	"files":   {"<include|exclude|clear> <repo> <glob>", "choose which paths get synced", "Repositories"},

	// Hooks
	"hook": {"<ls|add|run|clear|timeout>", "run a command after a sync changes something", "Hooks"},

	// Conflicts
	"resolve": {"<repo> [--ours|--theirs|--abort]", "deal with a conflict", "Conflicts and safety"},
	"backups": {"<repo>", "list the safety branches taken before rewrites", "Conflicts and safety"},
	"restore": {"<repo> <backup-branch>", "move the branch back onto a safety branch", "Conflicts and safety"},
	"prune":   {"<repo|all> [--days N]", "delete safety branches older than N days", "Conflicts and safety"},

	// Account and setup
	"auth":   {"<login|logout|status|setup>", "sign in to GitHub, or see who is signed in", "Account and setup"},
	"config": {"[set <key> <value>]", "show or change the settings", "Account and setup"},
	"doctor": {"", "check that git, auth and the repos are wired up", "Account and setup"},

	// Long running - the shell sends these to a real terminal
	"watch": {"<repo|all>", "sync on a timer and publish before shutdown", "Running in the background"},
	"serve": {"[--addr :7391]", "web dashboard with a live log", "Running in the background"},
	"tray":  {"", "sit in the system tray", "Running in the background"},

	// This session
	"help":    {"", "show every command", "This session"},
	"log":     {"[n] [--live]", "recent log lines, or follow them live", "This session"},
	"clear":   {"", "clear the screen", "This session"},
	"exit":    {"", "leave the shell", "This session"},
	"version": {"", "print the version", "This session"},
}

// subHints describe the second word, so a hint never dries up mid-command.
var subHints = map[string]cmdInfo{
	"repo add":     {"<owner/name> [--path ./dir] [--branch main] [--interval 10]", "clone it and start tracking", ""},
	"repo ls":      {"", "list your repositories on GitHub", ""},
	"auth login":   {"[--pat <token>]", "sign in with a personal access token", ""},
	"auth logout":  {"", "forget the stored token", ""},
	"auth status":  {"", "show who is signed in", ""},
	"auth setup":   {"<github oauth client id>", "use your own OAuth app", ""},
	"hook ls":      {"", "show the hooks on every repository", ""},
	"hook add":     {"<repo> <after-pull|after-push|on-conflict> <command>", "run a command after a sync", ""},
	"hook run":     {"<repo> <after-pull|after-push|on-conflict>", "try a hook without syncing", ""},
	"hook clear":   {"<repo> [event]", "remove hooks", ""},
	"hook timeout": {"<repo> <seconds>", "how long a hook may run", ""},
	"files include": {"<repo> <glob>", "only sync paths that match", ""},
	"files exclude": {"<repo> <glob>", "never sync paths that match", ""},
	"files clear":   {"<repo>", "go back to syncing everything", ""},
	"config set":    {"<key> <value>", "keys: strategy, interval_minutes, backup, push_backup, notify, commit_template, keep_backup_days", ""},
}

// groupOrder keeps the help in a sensible reading order.
var groupOrder = []string{
	"Syncing",
	"Repositories",
	"Hooks",
	"Conflicts and safety",
	"Running in the background",
	"Account and setup",
	"This session",
}

func (c cmdInfo) hint() string {
	switch {
	case c.Args != "" && c.Desc != "":
		return c.Args + "   " + c.Desc
	case c.Args != "":
		return c.Args
	default:
		return c.Desc
	}
}

// hintFor is what the prompt shows once a command is recognised.
func hintFor(key string) string {
	if info, ok := subHints[key]; ok {
		return fit(info.hint())
	}
	if info, ok := catalog[key]; ok {
		return fit(info.hint())
	}
	return ""
}

func knownCommands() []string {
	out := make([]string, 0, len(catalog))
	for name := range catalog {
		if name == "list" {
			continue // ls is the one people type
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
