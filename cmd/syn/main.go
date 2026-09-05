package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/FalconTechDev/synchronize/internal/syn"
	"golang.org/x/term"
)

var Version = "dev"

const usage = `synchronize (syn) — automatic git sync across your machines

USAGE
  syn <command> [repo] [flags]

  Commands that touch a repository need it named - see ` + "`syn list`" + `.
  Pass "all" to act on every repository you track. Pausing one only
  keeps it out of automatic syncing - you can still reach it by hand.

GITHUB AUTH
  auth login             authenticate with GitHub (PAT or Device Flow)
  auth login --pat <token>  use a specific Personal Access Token
  auth setup <client_id>    use a registered GitHub OAuth App (enables Device Flow)
  auth logout            remove stored credentials
  auth status            show who is logged in

REPOSITORIES
  init [path]            register the repo at path (default: current directory)
  repo add <owner/repo> [--path ./dir] [--branch main] [--interval 5]
                         clone from GitHub and register (works with private repos)
  repo add <url>         any other git remote - GitLab, a company server -
                         where git handles the credentials as it always does
  repo ls                list your GitHub repos (requires auth)
  ls  (or list)          show registered repositories
  path <repo>            print its folder and nothing else, for cd $(syn path x)
  ignore <repo> <glob>   keep files out of automatic commits (they stay on disk)
  remove <repo>          stop tracking a repository (files untouched)
  enable <repo>          same as auto <repo> on
  disable <repo>         same as auto <repo> off
  set <repo> <k> <v>     change name, remote, branch, interval, require_checks
                         interval takes minutes or a duration: 30m, 12h, 1d
                         require_checks on: never pull a commit whose GitHub
                         checks are red or still running (--no-checks overrides)

PART OF A REPOSITORY  (by default the whole thing is synced)
  files <repo>                  show which paths are being synced
  files include <repo> <glob>   keep only what matches: "src/**", "notes.md"
  files exclude <repo> <glob>   keep everything except: "logs/**", "*.mp4"
  files clear <repo>            go back to the whole repository

  This uses git sparse-checkout: the files you leave out are removed from
  this folder but stay on the remote, and syn will not commit them as
  deleted. Useful for pulling one folder out of a large repository.

WAYS TO LOOK AT IT
  shell                  a session that stays open until you type exit
  tray                   sit by the clock with pull and push to hand
  serve [--addr :7391]   dashboard in the browser, with the log as it happens

SYNCING
  pull  <repo|all> [--force]  bring remote changes in  (--force: reset to remote)
  pull  <repo> --to <ref>     go to a particular commit, tag or branch, and
                              nowhere else - how a deploy is pinned or rolled back
  push  <repo|all> [--force]  commit + publish local work (--force: force-with-lease)
  sync  <repo|all>            pull, then push
  status <repo|all> [--fetch] show what would happen
  status ... --json           the same, for a script to read
  status ... --porcelain      one tab-separated line per repository
  undo <repo>                 take back syn's last commit, keeping the changes

  Add --dry-run to pull, push or sync to see what they would do first.

  watch <repo|all>            sync on a timer; publishes before the machine
                              shuts down and holds shutdown until it has

HOOKS  (run a command on THIS machine after a sync changed something)
  hook ls                          show configured hooks
  hook add <repo> <event> <cmd>    event: after-pull, after-push, on-conflict
  hook add <repo> <event> --paths "src/**,go.mod" <cmd>
                                   only when one of those files changed
  hook run <repo> <event>          try it now without syncing
  hook clear <repo> [event]        remove hooks
  hook timeout <repo> <seconds>    how long a hook may run (default 300)
  hook rollback <repo> on|off      put a pull back if its after-pull hook fails

  A hook is told what happened: SYN_EVENT, SYN_REPO, SYN_PATH, SYN_BRANCH,
  SYN_COMMIT, SYN_PREV_COMMIT, SYN_CHANGED_FILES (one path per line) and
  SYN_CHANGED_COUNT.

  Hooks live in ~/.synchronize/config.json and never travel through git,
  so a command can only come from this machine.
  Example:  syn hook add my-bot after-pull "systemctl restart my-bot"

CONFLICTS
  resolve <repo>           replay the conflict into your working tree
  resolve <repo> --ours    keep your side everywhere
  resolve <repo> --theirs  keep the remote side everywhere
  resolve <repo> --abort   give up on the in-progress rebase/merge
  backups <repo>           list safety branches taken before rewrites
  restore <repo> <ref>     move the branch back onto a safety branch
  prune <repo|all> [--days N]  delete safety branches older than N days
                           (default: keep_backup_days; --days 0 deletes all)

AUTOMATIC SYNCING  (three separate switches)
  auto                   show which of the three are on
  auto login on|off      sync once when you log in
  auto watch on|off      after that, keep syncing while you work
  auto watch 30m         same, and set how often (30m, 12h, 1d)
  auto shutdown on|off   publish before the machine goes down, and hold
                         the shutdown until that has finished
  auto on|off            all three at once
  auto <repo> on|off     leave one repository out, or bring it back
  pause <repo> 2h        the same, but only for a while (off to end it early)
  start                  run the watcher now without waiting for login

OTHER
  config                 print the current configuration
  config set <k> <v>     backup, push_backup, strategy, interval_minutes,
                         notify, commit_template, keep_backup_days
  log [repo] [n]         last n log lines, all of them or one repository's
  log [repo] --live      stream the log in real time
  diff <repo|all>        what is not committed yet, untracked files included
  diff <repo> --pushed   what this branch has that the remote does not
  doctor                 check that everything is wired up
  version                print the version

GLOBAL FLAGS
  --all       same as naming "all"
  --quiet     no stdout (the log file is still written)
  -v          verbose

COMING SOON
  syn send <file> @<repo>   send files to another machine via a shared repo
  syn receive <repo>        watch for incoming files and copy them locally
  syn encrypt <repo>        per-repo AES-256 encryption via git filters
  syn tmp <files...>        one-shot file transfer via a throwaway repo
  config telegram ...       conflict alerts via Telegram bot
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Print(usage)
		return
	}
	if err := run(args); err != nil {
		syn.Errorf("%v", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return nil
	}

	valueFlags := map[string]bool{
		"--days":     true,
		"--pat":      true,
		"--path":     true,
		"--branch":   true,
		"--interval": true,
		"--lines":    true,
		"--desc":     true,
		"--dest":     true,
		"--addr":     true,
		"--to":       true,
		"--paths":    true,
	}

	var flags []string
	var pos []string
	values := map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case valueFlags[a] && i+1 < len(args):
			values[a] = args[i+1]
			i++
		case strings.Contains(a, "=") && valueFlags[strings.SplitN(a, "=", 2)[0]]:
			parts := strings.SplitN(a, "=", 2)
			values[parts[0]] = parts[1]
		case strings.HasPrefix(a, "-"):
			flags = append(flags, a)
		default:
			pos = append(pos, a)
		}
	}
	has := func(name string) bool {
		for _, f := range flags {
			if f == name {
				return true
			}
		}
		return false
	}
	syn.Quiet = has("--quiet") || has("-q")
	syn.Verbose = has("-v") || has("--verbose")

	cmd := pos[0]
	rest := pos[1:]
	arg := func(i int) string {
		if i < len(rest) {
			return rest[i]
		}
		return ""
	}

	selector := arg(0)
	if has("--all") {
		selector = "all"
	}

	if needsGit(cmd) {
		if err := requireGit(); err != nil {
			return err
		}
	}

	return dispatch(cmd, rest, arg, selector, has, values)
}

// parseMinutes accepts a bare number of minutes or a duration such as
// 30m, 12h or 1d, because typing 720 for twelve hours invites mistakes.
func parseMinutes(v string) (int, error) {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return 0, fmt.Errorf("empty value")
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n, nil
	}
	if strings.HasSuffix(v, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(v, "d"))
		if err != nil {
			return 0, err
		}
		return n * 24 * 60, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, err
	}
	if d < time.Minute {
		return 0, fmt.Errorf("%s is shorter than a minute", v)
	}
	return int(d.Minutes()), nil
}

// uniqueName finds a free name near the one asked for. The loop has to move
// on each turn, otherwise a taken "-2" would spin forever.
func uniqueName(cfg *syn.Config, base string) string {
	if cfg.Find(base) == nil {
		return base
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%d", base, n)
		if cfg.Find(candidate) == nil {
			return candidate
		}
	}
}

func needsGit(cmd string) bool {
	switch cmd {
	case "help", "--help", "-h", "version", "--version", "config", "log", "auth":
		return false
	}
	return true
}

func requireGit() error {
	if _, err := exec.LookPath("git"); err == nil {
		return nil
	}
	hint := "install it from https://git-scm.com/downloads"
	if runtime.GOOS == "windows" {
		hint = "install it with:  winget install --id Git.Git -e"
	}
	return fmt.Errorf("git is not installed (or not on your PATH)\n"+
		"        synchronize drives the real git, so it needs git available.\n"+
		"        %s\n"+
		"        Then open a NEW terminal and try again.", hint)
}

func dispatch(cmd string, rest []string, arg func(int) string, selector string, has func(string) bool, values map[string]string) error {
	cfg, err := syn.LoadConfig()
	if err != nil {
		return err
	}
	engine := syn.NewEngine(cfg)
	engine.SkipChecks = has("--no-checks")
	syn.FallbackIdentity = cfg.Identity

	creds := syn.LoadCredentials()

	switch cmd {
	case "help", "--help", "-h":
		fmt.Print(usage)
		return nil
	case "version", "--version":
		fmt.Printf("synchronize %s (%s/%s)\n", Version, runtime.GOOS, runtime.GOARCH)
		return nil

	case "auth":
		return cmdAuth(cfg, creds, rest, has, values)

	case "init":
		return cmdInit(cfg, arg(0))
	case "repo":
		return cmdRepo(cfg, creds, engine, rest, has, values)
	case "log":
		// The arguments are "how many" and "about what", in either order,
		// both optional: syn log, syn log 100, syn log my-repo, syn log
		// my-repo 100.
		n, repo := 40, ""
		if v := values["--lines"]; v != "" {
			fmt.Sscanf(v, "%d", &n)
		}
		for _, a := range rest {
			if strings.HasPrefix(a, "-") {
				continue
			}
			if parsed, err := strconv.Atoi(a); err == nil && parsed > 0 {
				n = parsed
				continue
			}
			if cfg.Find(a) == nil {
				return fmt.Errorf("log takes a number of lines or a repository, and %q is neither\n%s",
					a, repoListHint(cfg))
			}
			repo = cfg.Find(a).Name
		}
		if has("--live") || has("-f") {
			stop := make(chan struct{})
			what := "log"
			if repo != "" {
				what = repo + " log"
			}
			fmt.Printf("streaming %s (Ctrl+C to stop)...\n", what)
			return syn.LiveTailFor(os.Stdout, repo, stop)
		}
		fmt.Println(syn.TailLogFor(repo, n))
		return nil

	case "diff":
		repos, err := cfg.Selected(selector)
		if err != nil {
			return err
		}
		for _, r := range repos {
			var out string
			var err error
			if has("--pushed") {
				branch := r.Branch
				if branch == "" {
					branch, _ = syn.CurrentBranch(r.Path)
				}
				out, err = syn.DiffAgainstRemote(r.Path, r.Remote, branch)
			} else {
				out, err = syn.Diff(r.Path)
			}
			if err != nil {
				return err
			}
			if len(repos) > 1 {
				fmt.Printf("=== %s ===\n", r.Name)
			}
			if strings.TrimSpace(out) == "" {
				fmt.Println("(nothing uncommitted)")
			} else {
				fmt.Println(out)
			}
		}
		return nil

	case "tray":
		syn.CaptureLibraryLogs()
		syn.RunTray(cfg, engine)
		return nil

	case "send", "receive", "encrypt", "tmp":
		// A script has to be able to tell this from success.
		return fmt.Errorf("%s is not finished yet - see the readme for what works today", cmd)

	case "shell", "sh", "i":
		return cmdShell()

	case "hook", "hooks":
		return cmdHook(cfg, engine, rest, values["--paths"])

	case "serve":
		addr := values["--addr"]
		return engine.Serve(cfg, addr)
	case "list", "ls":
		return cmdList(cfg)
	case "remove", "rm":
		return cmdRemove(cfg, arg(0))
	case "enable", "disable":
		return cmdToggle(cfg, arg(0), cmd == "enable")
	case "set":
		return cmdSet(cfg, arg(0), arg(1), arg(2))

	case "files":
		return cmdFiles(cfg, rest)

	case "pull":
		if to := values["--to"]; to != "" {
			if has("--force") {
				return fmt.Errorf("--to already resets hard; --force adds nothing")
			}
			return runOp(engine, cfg, selector, has, engine.PullTo(to))
		}
		return runOp(engine, cfg, selector, has, pick(has("--force"), engine.ForcePull, engine.Pull))
	case "push":
		return runOp(engine, cfg, selector, has, pick(has("--force"), engine.ForcePush, engine.Push))
	case "sync":
		return runOp(engine, cfg, selector, has, engine.Sync)

	case "status":
		return cmdStatus(engine, cfg, selector, has)
	case "watch":
		repos, err := cfg.Selected(selector)
		if err != nil {
			return err
		}
		// Started by the scheduler there is a console window nobody asked
		// for; started from a terminal this does nothing.
		syn.CaptureLibraryLogs()
		syn.HideConsoleWindow()
		if has("--no-tray") {
			engine.Watch(repos)
		} else {
			engine.WatchWithTray(repos, cfg)
		}
		return nil

	case "resolve":
		if arg(0) == "" {
			return fmt.Errorf("resolve needs a repository name")
		}
		repos, err := cfg.Selected(arg(0))
		if err != nil {
			return err
		}
		mode := "continue"
		switch {
		case has("--abort"):
			mode = "abort"
		case has("--ours"):
			mode = "ours"
		case has("--theirs"):
			mode = "theirs"
		}
		res := engine.Resolve(repos[0], mode)
		if res.Err != nil {
			return res.Err
		}
		syn.Info("%s", res.Detail())
		return nil

	case "backups":
		return cmdBackups(cfg, selector)
	case "restore":
		if arg(0) == "" || arg(1) == "" {
			return fmt.Errorf("usage: syn restore <repo> <backup-branch>")
		}
		repos, err := cfg.Selected(arg(0))
		if err != nil {
			return err
		}
		return engine.RestoreBackup(repos[0], arg(1))
	case "prune":
		days := -1
		if v := values["--days"]; v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return fmt.Errorf("--days needs a number of days (0 deletes every backup)")
			}
			days = n
		}
		return cmdPrune(engine, cfg, selector, days)

	case "path":
		return cmdPath(cfg, arg(0))
	case "undo":
		return cmdUndo(engine, cfg, arg(0), has("--force"), has("--yes"))
	case "pause":
		return cmdPause(cfg, rest)
	case "ignore":
		return cmdIgnore(cfg, rest)

	case "auto", "autostart", "install", "uninstall":
		return cmdAuto(cfg, cmd, rest)
	case "start":
		return syn.StartWatcherNow()

	case "config":
		return cmdConfig(cfg, rest)
	case "doctor":
		return cmdDoctor(engine, cfg)
	}
	return fmt.Errorf("unknown command %q - run `syn help`", cmd)
}

func pick(force bool, a, b func(*syn.Repo) syn.Result) func(*syn.Repo) syn.Result {
	if force {
		return a
	}
	return b
}

func runOp(e *syn.Engine, cfg *syn.Config, selector string, has func(string) bool, op func(*syn.Repo) syn.Result) error {
	repos, err := cfg.Selected(selector)
	if err != nil {
		return err
	}

	if has("--dry-run") {
		return dryRun(e, repos)
	}

	results := e.RunAll(repos, op)
	if has("--json") {
		if err := printJSON(opReports(results)); err != nil {
			return err
		}
	}

	failed := 0
	for _, r := range results {
		if !r.OK() {
			failed++
		}
	}
	if failed == 0 {
		return nil
	}
	if failed == 1 {
		return fmt.Errorf("1 repository needs attention - see `syn status`")
	}
	return fmt.Errorf("%d repositories need attention - see `syn status`", failed)
}

func cmdInit(cfg *syn.Config, path string) error {
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if !syn.IsRepo(abs) {
		return fmt.Errorf("%s is not a git repository - clone it first", abs)
	}
	root, err := syn.RepoRoot(abs)
	if err != nil {
		return err
	}
	root = filepath.Clean(root)
	if existing := cfg.Find(root); existing != nil {
		return fmt.Errorf("already registered as %q", existing.Name)
	}
	remotes, err := syn.Remotes(root)
	if err != nil {
		return err
	}
	if len(remotes) == 0 {
		return fmt.Errorf("%s has no remote - add one with `git remote add origin <url>`", root)
	}
	remote := remotes[0]
	for _, r := range remotes {
		if r == "origin" {
			remote = "origin"
		}
	}
	name := uniqueName(cfg, filepath.Base(root))
	cfg.Repos = append(cfg.Repos, syn.Repo{
		Name:    name,
		Path:    root,
		Remote:  remote,
		Enabled: true,
	})
	if err := cfg.Save(); err != nil {
		return err
	}
	url, _ := syn.RemoteURL(root, remote)
	syn.Info("registered %q -> %s (%s)", name, remote, url)
	syn.Info("next: `syn auto on` to keep it in sync by itself")
	return nil
}

func cmdList(cfg *syn.Config) error {
	if len(cfg.Repos) == 0 {
		fmt.Println("no repositories registered - run `syn init` inside a git repo")
		return nil
	}
	width := 4
	for _, r := range cfg.Repos {
		if n := len(r.Name); n > width {
			width = n
		}
	}

	for _, r := range cfg.Repos {
		branch := r.Branch
		if branch == "" {
			branch = "(current branch)"
		}
		every := fmt.Sprintf("every %s", humanMinutes(cfg.IntervalFor(&r)))
		if !r.Enabled {
			every = "paused"
		}

		fmt.Printf("  %-*s  %-16s  %s\n", width, r.Name, every, r.Remote+"/"+branch)
		fmt.Printf("  %-*s  %s\n", width, "", r.Path)
		if !r.Hooks.IsEmpty() {
			n := len(r.Hooks.AfterPull) + len(r.Hooks.AfterPush) + len(r.Hooks.OnConflict)
			fmt.Printf("  %-*s  %s\n", width, "", plural(n, "hook")+" configured")
		}
	}
	fmt.Printf("\n  Name one of these, or \"all\", when you pull, push or sync.\n")
	return nil
}

func humanMinutes(m int) string {
	switch {
	case m%(24*60) == 0 && m >= 24*60:
		return plural(m/(24*60), "day")
	case m%60 == 0 && m >= 60:
		return plural(m/60, "hour")
	default:
		return plural(m, "minute")
	}
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func cmdRemove(cfg *syn.Config, key string) error {
	if key == "" {
		return fmt.Errorf("usage: syn remove <repo>")
	}
	target := cfg.Find(key)
	if target == nil {
		return fmt.Errorf("unknown repository %q", key)
	}
	kept := cfg.Repos[:0]
	for _, r := range cfg.Repos {
		if r.Name != target.Name {
			kept = append(kept, r)
		}
	}
	cfg.Repos = kept
	if err := cfg.Save(); err != nil {
		return err
	}
	syn.Info("stopped tracking %q (files untouched)", key)
	return nil
}

func cmdToggle(cfg *syn.Config, key string, enabled bool) error {
	r := cfg.Find(key)
	if r == nil {
		return fmt.Errorf("unknown repository %q", key)
	}
	r.Enabled = enabled
	if err := cfg.Save(); err != nil {
		return err
	}
	syn.Info("%s is now %s", r.Name, map[bool]string{true: "enabled", false: "disabled"}[enabled])
	return nil
}

func cmdSet(cfg *syn.Config, key, field, value string) error {
	r := cfg.Find(key)
	if r == nil {
		return fmt.Errorf("unknown repository %q", key)
	}
	switch field {
	case "name":
		r.Name = value
	case "remote":
		r.Remote = value
	case "branch":
		r.Branch = value
	case "interval":
		n, err := parseMinutes(value)
		if err != nil || n < 0 {
			return fmt.Errorf("interval must be minutes, or a duration like 30m, 12h, 1d (0 = use the global setting)")
		}
		r.IntervalMinutes = n
	case "require_checks", "require-checks":
		switch value {
		case "on", "true", "yes":
			r.RequireStatusCheck = true
		case "off", "false", "no":
			r.RequireStatusCheck = false
		default:
			return fmt.Errorf("require_checks is on or off, not %q", value)
		}
	default:
		return fmt.Errorf("cannot set %q - use name, remote, branch, interval or require_checks", field)
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	syn.Info("%s.%s = %q", r.Name, field, value)
	return nil
}

func cmdAuth(cfg *syn.Config, creds *syn.Credentials, rest []string, has func(string) bool, values map[string]string) error {
	sub := ""
	if len(rest) > 0 {
		sub = rest[0]
	}
	switch sub {
	case "login":
		pat := values["--pat"]
		if pat == "" && has("--pat") {
			return fmt.Errorf("--pat requires a token value")
		}
		if pat != "" {
			display, err := syn.LoginPAT(creds, cfg, pat)
			if err != nil {
				return err
			}
			syn.Info("logged in as %s", display)
			return nil
		}

		clientID := cfg.GitHubClientID
		if clientID == "" {
			fmt.Fprintln(os.Stderr, "No GitHub OAuth App configured. Options:")
			fmt.Fprintln(os.Stderr, "  1. Paste a Personal Access Token below (github.com/settings/tokens)")
			fmt.Fprintln(os.Stderr, "  2. Run `syn auth setup <client_id>` to enable Device Flow")
			fmt.Fprint(os.Stderr, "\nToken: ")
			var token string
			fmt.Fscan(os.Stdin, &token)
			token = strings.TrimSpace(token)
			if token == "" {
				return fmt.Errorf("no token entered")
			}
			display, err := syn.LoginPAT(creds, cfg, token)
			if err != nil {
				return err
			}
			syn.Info("logged in as %s", display)
			return nil
		}
		display, err := syn.LoginDeviceFlow(creds, clientID)
		if err != nil {
			return err
		}
		syn.Info("logged in as %s", display)
		return nil

	case "setup":
		if len(rest) < 2 {
			return fmt.Errorf("usage: syn auth setup <github-oauth-client-id>")
		}
		cfg.GitHubClientID = rest[1]
		if err := cfg.Save(); err != nil {
			return err
		}
		syn.Info("GitHub OAuth App configured - run `syn auth login` to authenticate")
		return nil

	case "logout":
		creds.Set("github.com", "")
		if err := creds.Save(); err != nil {
			return err
		}
		syn.Info("logged out")
		return nil

	case "status":
		display := syn.AuthStatus(creds)
		if display == "" {
			fmt.Println("not logged in (run `syn auth login`)")
		} else {
			fmt.Printf("logged in as %s\n", display)
		}
		return nil

	default:
		return fmt.Errorf("usage: syn auth <login|logout|status|setup>")
	}
}

func cmdRepo(cfg *syn.Config, creds *syn.Credentials, engine *syn.Engine, rest []string, has func(string) bool, values map[string]string) error {
	sub := ""
	if len(rest) > 0 {
		sub = rest[0]
	}
	switch sub {
	case "add":
		if len(rest) < 2 {
			return fmt.Errorf("usage: syn repo add <owner/repo> [--path ./dir] [--branch main] [--interval 5]")
		}
		target := rest[1]

		// A full URL means GitLab, a company server, or anything else with a
		// git endpoint. syn syncs git; it should not insist on GitHub.
		if looksLikeURL(target) {
			return cmdRepoAddURL(cfg, target, values)
		}

		parts := strings.SplitN(target, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("give owner/repo for GitHub (torvalds/linux),\n" +
				"        or a full URL for anywhere else (https://gitlab.com/group/thing.git)")
		}
		return cmdRepoAdd(cfg, creds, target, values, has("--force"))

	case "ls", "list":
		return cmdRepoList(creds)

	case "create":
		if len(rest) < 2 {
			return fmt.Errorf("usage: syn repo create <name> [--public] [--path ./dir] [--desc \"...\"]")
		}
		return cmdRepoCreate(cfg, creds, rest[1], false, has("--public"), values)

	default:
		return fmt.Errorf("usage: syn repo <add|create|ls>")
	}
}

// confirmDestination shows where the clone will land and, when a person is
// watching, lets them put it somewhere else before anything is downloaded.
func confirmDestination(slug, suggested string) (string, error) {
	fmt.Println()
	fmt.Printf("  %s will be cloned into:\n", slug)
	fmt.Printf("      %s%s%s\n\n", cBold, suggested, cReset)

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", nil
	}

	fmt.Printf("  %sPress Enter to accept, or type another folder:%s ", cDim, cReset)
	in := bufio.NewReader(os.Stdin)
	answer, err := in.ReadString('\n')
	if err != nil {
		fmt.Println()
		return "", nil
	}
	answer = strings.TrimSpace(answer)
	fmt.Println()
	if answer == "" {
		return "", nil
	}
	chosen, err := filepath.Abs(answer)
	if err != nil {
		return "", fmt.Errorf("%q is not a usable path: %w", answer, err)
	}
	syn.Info("using %s", chosen)
	return chosen, nil
}

func cmdRepoAdd(cfg *syn.Config, creds *syn.Credentials, slug string, values map[string]string, force bool) error {
	token := creds.Token("github.com")
	gh := syn.NewGitHub(token)
	parts := strings.SplitN(slug, "/", 2)
	owner, repoName := parts[0], parts[1]

	syn.Info("fetching repo info from GitHub...")
	info, err := gh.RepoInfo(owner, repoName)
	if err != nil {
		return fmt.Errorf("could not find %s on GitHub: %w\n(private repo? run `syn auth login` first)", slug, err)
	}

	destPath := values["--path"]
	if destPath == "" {
		destPath = "./" + repoName
	}
	abs, err := filepath.Abs(destPath)
	if err != nil {
		return err
	}

	branch := values["--branch"]
	if branch == "" {
		branch = info.DefaultBranch
	}

	interval := 0
	if v := values["--interval"]; v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return fmt.Errorf("--interval must be a positive number of minutes")
		}
		interval = n
	}

	if err := confirmSecondCopy(cfg, slug, branch, force); err != nil {
		return err
	}

	if values["--path"] == "" {
		if chosen, err := confirmDestination(slug, abs); err != nil {
			return err
		} else if chosen != "" {
			abs = chosen
		}
	}

	if existing := cfg.Find(abs); existing != nil {
		if syn.IsRepo(abs) {
			return fmt.Errorf("already registered as %q", existing.Name)
		}
		// Registered, but the working tree is gone - this is the repair the
		// error message tells people to run, so let it through.
		syn.Info("%s is registered but no longer a git repository - cloning it again", existing.Name)
		if entries, err := os.ReadDir(abs); err == nil && len(entries) > 0 {
			return fmt.Errorf("%s still has files in it\n"+
				"        move or delete them first, then run this again\n"+
				"        (or stop tracking it with: syn remove %s)", abs, existing.Name)
		}
	}

	if syn.IsRepo(abs) {
		syn.Info("directory %s is already a git repo - linking it", abs)
	} else {
		visibility := "public"
		if info.Private {
			visibility = "private"
			if token == "" {
				return fmt.Errorf("%s is a private repository - run `syn auth login` first", slug)
			}
		}
		syn.Info("cloning %s %s repo (~%d KB)...", visibility, slug, info.Size)
		cloneArgs := []string{"--quiet"}
		if branch != info.DefaultBranch {
			cloneArgs = append(cloneArgs, "--branch", branch)
		}
		if err := syn.CloneWithToken(info.CloneURL, abs, token, cloneArgs...); err != nil {
			return fmt.Errorf("clone failed: %w", err)
		}
		syn.Info("cloned to %s", abs)
	}

	name := uniqueName(cfg, repoName)

	repo := syn.Repo{
		Name:            name,
		Path:            abs,
		Remote:          "origin",
		Branch:          branch,
		Enabled:         true,
		GithubSlug:      slug,
		Private:         info.Private,
		IntervalMinutes: interval,
	}
	cfg.Repos = append(cfg.Repos, repo)
	if err := cfg.Save(); err != nil {
		return err
	}
	extra := ""
	if interval > 0 {
		extra = fmt.Sprintf(", syncing every %d min", interval)
	}
	syn.Info("registered %q (%s, branch %s%s)", name, slug, branch, extra)
	if info.Private {
		syn.Info("token will be used automatically for push/pull")
	}
	return nil
}

func cmdRepoList(creds *syn.Credentials) error {
	token := creds.Token("github.com")
	if token == "" {
		return fmt.Errorf("not logged in - run `syn auth login` first")
	}
	gh := syn.NewGitHub(token)
	syn.Info("fetching your GitHub repositories...")
	repos, err := gh.ListRepos("")
	if err != nil {
		return err
	}
	fmt.Printf("%-40s %-8s %s\n", "REPO", "PRIVATE", "DEFAULT BRANCH")
	fmt.Println(strings.Repeat("-", 65))
	for _, r := range repos {
		priv := "no"
		if r.Private {
			priv = "yes"
		}
		desc := r.Description
		if len(desc) > 35 {
			desc = desc[:32] + "..."
		}
		fmt.Printf("%-40s %-8s %-14s %s\n", r.FullName, priv, r.DefaultBranch, desc)
	}
	fmt.Printf("\n%d repositories\n", len(repos))
	return nil
}

func cmdHook(cfg *syn.Config, e *syn.Engine, rest []string, paths string) error {
	if len(rest) == 0 || rest[0] == "ls" || rest[0] == "list" {
		return listHooks(cfg)
	}

	switch rest[0] {
	case "add":
		if len(rest) < 4 {
			return fmt.Errorf("usage: syn hook add <repo> <after-pull|after-push|on-conflict> <command...>")
		}
		r := cfg.Find(rest[1])
		if r == nil {
			return fmt.Errorf("unknown repository %q", rest[1])
		}
		paths := splitPaths(paths)
		command := strings.Join(rest[3:], " ")
		if command == "" {
			return fmt.Errorf("hook add needs a command to run")
		}
		list, ok := r.Hooks.For(rest[2])
		if !ok {
			return fmt.Errorf("event must be after-pull, after-push or on-conflict")
		}
		r.Hooks.Set(rest[2], append(list, syn.Hook{Run: command, Paths: paths}))
		if err := cfg.Save(); err != nil {
			return err
		}
		if len(paths) > 0 {
			syn.Info("%s: %s hook added, only for %s", r.Name, rest[2], strings.Join(paths, ", "))
		} else {
			syn.Info("%s: %s hook added", r.Name, rest[2])
		}
		return nil

	case "clear":
		if len(rest) < 2 {
			return fmt.Errorf("usage: syn hook clear <repo> [after-pull|after-push|on-conflict]")
		}
		r := cfg.Find(rest[1])
		if r == nil {
			return fmt.Errorf("unknown repository %q", rest[1])
		}
		which := ""
		if len(rest) > 2 {
			which = rest[2]
		}
		switch which {
		case "after-pull":
			r.Hooks.AfterPull = nil
		case "after-push":
			r.Hooks.AfterPush = nil
		case "on-conflict":
			r.Hooks.OnConflict = nil
		case "":
			r.Hooks = syn.Hooks{}
		default:
			return fmt.Errorf("event must be after-pull, after-push or on-conflict")
		}
		if err := cfg.Save(); err != nil {
			return err
		}
		syn.Info("%s: hooks cleared", r.Name)
		return nil

	case "rollback":
		if len(rest) < 3 {
			return fmt.Errorf("usage: syn hook rollback <repo> on|off")
		}
		r := cfg.Find(rest[1])
		if r == nil {
			return fmt.Errorf("unknown repository %q", rest[1])
		}
		switch rest[2] {
		case "on", "true", "yes":
			r.Hooks.Rollback = true
		case "off", "false", "no":
			r.Hooks.Rollback = false
		default:
			return fmt.Errorf("rollback is on or off, not %q", rest[2])
		}
		if err := cfg.Save(); err != nil {
			return err
		}
		if r.Hooks.Rollback {
			syn.Info("%s: a failing after-pull hook will put the repository back", r.Name)
		} else {
			syn.Info("%s: a failing after-pull hook leaves the pull in place", r.Name)
		}
		return nil

	case "timeout":
		if len(rest) < 3 {
			return fmt.Errorf("usage: syn hook timeout <repo> <seconds>")
		}
		r := cfg.Find(rest[1])
		if r == nil {
			return fmt.Errorf("unknown repository %q", rest[1])
		}
		n, err := strconv.Atoi(rest[2])
		if err != nil || n <= 0 {
			return fmt.Errorf("timeout must be a positive number of seconds")
		}
		r.Hooks.TimeoutSeconds = n
		if err := cfg.Save(); err != nil {
			return err
		}
		syn.Info("%s: hook timeout set to %ds", r.Name, n)
		return nil

	case "run":
		if len(rest) < 3 {
			return fmt.Errorf("usage: syn hook run <repo> <after-pull|after-push|on-conflict>")
		}
		r := cfg.Find(rest[1])
		if r == nil {
			return fmt.Errorf("unknown repository %q", rest[1])
		}
		return e.RunHooksNow(r, rest[2])
	}
	return fmt.Errorf("unknown hook subcommand %q (try: add, ls, clear, timeout, rollback, run)", rest[0])
}

func listHooks(cfg *syn.Config) error {
	any := false
	for i := range cfg.Repos {
		r := &cfg.Repos[i]
		if r.Hooks.IsEmpty() {
			continue
		}
		any = true
		rollback := ""
		if r.Hooks.Rollback {
			rollback = ", rolls back a pull if the hook fails"
		}
		fmt.Printf("%s  (timeout %ds%s)\n", r.Name, r.Hooks.Timeout(), rollback)
		show := func(label string, hooks syn.HookList) {
			for _, h := range hooks {
				fmt.Printf("  %-12s %s\n", label, h.Run)
				if len(h.Paths) > 0 {
					fmt.Printf("  %-12s   only when these change: %s\n", "", strings.Join(h.Paths, ", "))
				}
			}
		}
		show("after-pull", r.Hooks.AfterPull)
		show("after-push", r.Hooks.AfterPush)
		show("on-conflict", r.Hooks.OnConflict)
		fmt.Println()
	}
	if !any {
		fmt.Println("No hooks configured.")
		fmt.Println()
		fmt.Println("A hook runs a command on this machine after a sync actually changed something.")
		fmt.Println("Example - restart a bot whenever new code arrives:")
		fmt.Println()
		fmt.Println(`  syn hook add my-bot after-pull "systemctl restart my-bot"`)
		fmt.Println()
		fmt.Println("Hooks are stored in ~/.synchronize/config.json and never travel through git.")
	}
	return nil
}

func cmdFiles(cfg *syn.Config, rest []string) error {
	sub := ""
	if len(rest) > 0 {
		sub = rest[0]
	}
	// `syn files <repo>` is the form the docs use and the one people reach for.
	if sub != "" && cfg.Find(sub) != nil {
		rest = append([]string{"show"}, rest...)
		sub = "show"
	}

	switch sub {
	case "show":
		if len(rest) < 2 {
			return fmt.Errorf("usage: syn files show <repo>")
		}
		r := cfg.Find(rest[1])
		if r == nil {
			return fmt.Errorf("unknown repository %q", rest[1])
		}
		fmt.Printf("%s:\n", r.Name)
		if len(r.Include) == 0 && len(r.Exclude) == 0 {
			fmt.Println("  (no rules - all files are synced)")
			fmt.Println()
			fmt.Print("  active sparse-checkout:\n  ")
			fmt.Println(syn.SparseStatus(r))
			return nil
		}
		fmt.Println("  include:")
		for _, p := range r.Include {
			fmt.Printf("    %s\n", p)
		}
		fmt.Println("  exclude:")
		for _, p := range r.Exclude {
			fmt.Printf("    %s\n", p)
		}
		fmt.Print("\n  active sparse-checkout:\n  ")
		fmt.Println(syn.SparseStatus(r))
		return nil

	case "include", "exclude":
		if len(rest) < 3 {
			return fmt.Errorf("usage: syn files %s <repo> <glob>", sub)
		}
		r := cfg.Find(rest[1])
		if r == nil {
			return fmt.Errorf("unknown repository %q", rest[1])
		}
		pattern := rest[2]
		if sub == "include" {
			r.Include = appendUniq(r.Include, pattern)
		} else {
			r.Exclude = appendUniq(r.Exclude, pattern)
		}
		if err := cfg.Save(); err != nil {
			return err
		}
		if err := syn.ApplySparseCheckout(r); err != nil {
			syn.Warn("sparse-checkout update failed: %v", err)
		}
		syn.Info("%s: %s pattern %q added", r.Name, sub, pattern)
		return nil

	case "remove-pattern":
		if len(rest) < 4 {
			return fmt.Errorf("usage: syn files remove-pattern <repo> <include|exclude> <glob>")
		}
		r := cfg.Find(rest[1])
		if r == nil {
			return fmt.Errorf("unknown repository %q", rest[1])
		}
		which, pattern := rest[2], rest[3]
		switch which {
		case "include":
			r.Include = removePattern(r.Include, pattern)
		case "exclude":
			r.Exclude = removePattern(r.Exclude, pattern)
		default:
			return fmt.Errorf("must be include or exclude")
		}
		if err := cfg.Save(); err != nil {
			return err
		}
		if err := syn.ApplySparseCheckout(r); err != nil {
			syn.Warn("sparse-checkout update failed: %v", err)
		}
		syn.Info("%s: %s pattern %q removed", r.Name, which, pattern)
		return nil

	case "clear":
		if len(rest) < 2 {
			return fmt.Errorf("usage: syn files clear <repo>")
		}
		r := cfg.Find(rest[1])
		if r == nil {
			return fmt.Errorf("unknown repository %q", rest[1])
		}
		r.Include = nil
		r.Exclude = nil
		if err := cfg.Save(); err != nil {
			return err
		}
		if err := syn.ApplySparseCheckout(r); err != nil {
			syn.Warn("sparse-checkout disable failed: %v", err)
		}
		syn.Info("%s: all file rules cleared (all files will be synced)", r.Name)
		return nil

	case "":
		return fmt.Errorf("usage: syn files <repo>, or syn files <include|exclude|clear> <repo> <glob>")
	default:
		return fmt.Errorf("unknown files subcommand %q", sub)
	}
}

func appendUniq(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}

func removePattern(slice []string, s string) []string {
	out := slice[:0]
	for _, v := range slice {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}

func cmdStatus(e *syn.Engine, cfg *syn.Config, selector string, has func(string) bool) error {
	repos, err := cfg.Selected(selector)
	if err != nil {
		return err
	}
	fetch := has("--fetch")

	if has("--json") {
		return printJSON(statusRows(e, repos, fetch))
	}
	if has("--porcelain") {
		printPorcelain(statusRows(e, repos, fetch))
		return nil
	}
	for _, r := range repos {
		fmt.Print(e.Status(r, fetch).String())
		fmt.Println()
	}
	return nil
}

func cmdBackups(cfg *syn.Config, selector string) error {
	repos, err := cfg.Selected(selector)
	if err != nil {
		return err
	}
	for _, r := range repos {
		branches, err := syn.Backups(r)
		if err != nil {
			return err
		}
		fmt.Printf("%s:\n", r.Name)
		if len(branches) == 0 {
			fmt.Println("  (none)")
			continue
		}
		for _, b := range branches {
			fmt.Printf("  %s\n", b)
		}
	}
	return nil
}

func cmdPrune(e *syn.Engine, cfg *syn.Config, selector string, days int) error {
	repos, err := cfg.Selected(selector)
	if err != nil {
		return err
	}
	for _, r := range repos {
		n, err := e.PruneBackups(r, days)
		if err != nil {
			return err
		}
		syn.Info("%s: removed %d old backup branch(es)", r.Name, n)
	}
	return nil
}

func cmdConfig(cfg *syn.Config, rest []string) error {
	if len(rest) == 0 {
		fmt.Printf("config file: %s\n", filepath.Join(syn.Home(), "config.json"))
		fmt.Printf("log file:    %s\n\n", syn.LogPath())
		fmt.Printf("strategy          %s\n", cfg.Strategy)
		fmt.Printf("interval_minutes  %d\n", cfg.IntervalMinutes)
		fmt.Printf("backup            %t\n", cfg.Backup)
		fmt.Printf("push_backup       %t\n", cfg.PushBackup)
		fmt.Printf("keep_backup_days   %d\n", cfg.KeepBackupDays)
		fmt.Printf("notify            %s\n", cfg.Notify)
		fmt.Printf("commit_template   %s\n", cfg.CommitTemplate)
		return nil
	}
	switch rest[0] {
	case "telegram", "test":
		fmt.Printf("syn config %s is coming soon — not available yet\n", rest[0])
		return nil
	case "set":
		if len(rest) < 3 {
			return fmt.Errorf("usage: syn config set <key> <value>")
		}
		return applyConfigKey(cfg, rest[1], strings.Join(rest[2:], " "))
	}
	return fmt.Errorf("unknown config subcommand %q", rest[0])
}

func mustHost() string {
	h, _ := os.Hostname()
	if h == "" {
		return "unknown host"
	}
	return h
}

func applyConfigKey(cfg *syn.Config, key, value string) error {
	switch key {
	case "strategy":
		if value != "rebase" && value != "merge" {
			return fmt.Errorf("strategy must be rebase or merge")
		}
		cfg.Strategy = value
	case "interval_minutes":
		n, err := parseMinutes(value)
		if err != nil || n <= 0 {
			return fmt.Errorf("interval_minutes must be minutes, or a duration like 30m, 12h, 1d")
		}
		cfg.IntervalMinutes = n
	case "keep_backup_days":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return fmt.Errorf("keep_backup_days must be zero or more")
		}
		cfg.KeepBackupDays = n
	case "backup":
		cfg.Backup = truthy(value)
	case "push_backup":
		cfg.PushBackup = truthy(value)
	case "notify":
		if value != "never" && value != "conflict" && value != "always" {
			return fmt.Errorf("notify must be never, conflict or always")
		}
		cfg.Notify = value
	case "commit_template":
		if unknown := syn.UnknownPlaceholders(value); len(unknown) > 0 {
			return fmt.Errorf("%s means nothing to syn and would end up in the commit as written\n"+
				"        the ones it fills in are: {host}, {time}",
				strings.Join(unknown, ", "))
		}
		cfg.CommitTemplate = value
	default:
		return fmt.Errorf("unknown key %q", key)
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	syn.Info("%s = %s", key, value)
	return nil
}

func truthy(v string) bool {
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// cmdAuto keeps the three background behaviours apart, because they are
// separate decisions: syncing when you sit down, keeping up while you work,
// and publishing before the machine goes down.
func cmdAuto(cfg *syn.Config, cmd string, rest []string) error {
	switch cmd {
	case "install":
		rest = []string{"on"}
	case "uninstall":
		rest = []string{"off"}
	}

	if len(rest) == 0 {
		return showAuto(cfg)
	}

	what := strings.ToLower(rest[0])
	value := ""
	if len(rest) > 1 {
		value = strings.ToLower(rest[1])
	}

	// `auto on` / `auto off` still turn everything on or off at once.
	if on, ok := parseSwitch(what); ok {
		return setAuto(cfg, on, on, on)
	}

	// `auto <repo> on|off` reads better than a separate enable/disable pair,
	// and keeps every "should this sync by itself" question in one place.
	if r := cfg.Find(what); r != nil {
		on, ok := parseSwitch(value)
		if !ok {
			return fmt.Errorf("say `syn auto %s on` or `syn auto %s off`", r.Name, r.Name)
		}
		if r.Enabled != on {
			r.Enabled = on
			if err := cfg.Save(); err != nil {
				return err
			}
		}
		return showAuto(cfg)
	}

	switch what {
	case "login", "startup":
		on, ok := parseSwitch(value)
		if !ok {
			return fmt.Errorf("say `syn auto login on` or `syn auto login off`")
		}
		return setAuto(cfg, on, cfg.KeepWatching, syn.ShutdownEnabled())

	case "shutdown":
		on, ok := parseSwitch(value)
		if !ok {
			return fmt.Errorf("say `syn auto shutdown on` or `syn auto shutdown off`")
		}
		return setAuto(cfg, syn.LoginEnabled(), cfg.KeepWatching, on)

	case "watch":
		if on, ok := parseSwitch(value); ok {
			return setAuto(cfg, syn.LoginEnabled() || on, on, syn.ShutdownEnabled())
		}
		if value == "" {
			return fmt.Errorf("say `syn auto watch on`, `syn auto watch off`, or an interval like `syn auto watch 30m`")
		}
		mins, err := parseMinutes(value)
		if err != nil || mins <= 0 {
			return fmt.Errorf("interval must be minutes, or a duration like 30m, 12h, 1d")
		}
		cfg.IntervalMinutes = mins
		if err := cfg.Save(); err != nil {
			return err
		}
		return setAuto(cfg, true, true, syn.ShutdownEnabled())
	}
	return fmt.Errorf("say `syn auto`, or one of: auto on | auto off | auto login on|off | auto shutdown on|off | auto watch on|off|<interval>")
}

func parseSwitch(v string) (bool, bool) {
	switch v {
	case "on", "enable", "yes", "true":
		return true, true
	case "off", "disable", "no", "false":
		return false, true
	}
	return false, false
}

func setAuto(cfg *syn.Config, login, keepWatching, shutdown bool) error {
	if (login || shutdown) && len(cfg.EnabledRepos()) == 0 {
		return fmt.Errorf("nothing to sync yet - add a repository first with `syn repo add owner/name`")
	}

	if login {
		if err := syn.EnableLogin(keepWatching); err != nil {
			return err
		}
	} else if err := syn.DisableLogin(); err != nil {
		return err
	}

	if shutdown {
		if err := syn.EnableShutdown(); err != nil {
			return err
		}
	} else if err := syn.DisableShutdown(); err != nil {
		return err
	}

	if cfg.KeepWatching != keepWatching {
		cfg.KeepWatching = keepWatching
		if err := cfg.Save(); err != nil {
			return err
		}
	}
	return showAuto(cfg)
}

func showAuto(cfg *syn.Config) error {
	login := syn.LoginEnabled()
	shutdown := syn.ShutdownEnabled()
	watching := login && cfg.KeepWatching

	mark := func(on bool) string {
		if on {
			return "on "
		}
		return "off"
	}

	fmt.Println()
	fmt.Printf("  login      %s   %s\n", mark(login), pickText(login,
		"sync once when you log in",
		"nothing happens when you log in"))
	fmt.Printf("  watch      %s   %s\n", mark(watching), pickText(watching,
		"then keep syncing every "+humanMinutes(cfg.IntervalMinutes),
		"no repeated syncing while you work"))
	fmt.Printf("  shutdown   %s   %s\n", mark(shutdown), pickText(shutdown,
		"publish before the machine goes down, and hold it until done",
		"the machine may shut down with your work unpublished"))
	fmt.Println()

	if len(cfg.Repos) > 0 {
		width := 0
		for i := range cfg.Repos {
			if n := len(cfg.Repos[i].Name); n > width {
				width = n
			}
		}
		fmt.Println("  Repositories")
		for i := range cfg.Repos {
			r := &cfg.Repos[i]
			var state string
			switch {
			case !r.Enabled:
				state = "paused - left out of automatic syncing"
			case !login && !shutdown:
				state = "would be covered, but nothing above is on"
			case watching:
				state = "every " + humanMinutes(cfg.IntervalFor(r))
			default:
				state = "covered"
			}
			fmt.Printf("    %-*s   %s\n", width, r.Name, state)
		}
		fmt.Println()
	}
	fmt.Println("  All three:      syn auto on|off")
	fmt.Println("  One at a time:  syn auto login|watch|shutdown on|off")
	fmt.Println("  How often:      syn auto watch 30m")
	fmt.Println("  One repo:       syn auto <repo> on|off")
	return nil
}

func pickText(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}

func cmdDoctor(e *syn.Engine, cfg *syn.Config) error {
	fmt.Printf("synchronize %s on %s/%s\n", Version, runtime.GOOS, runtime.GOARCH)
	if out, err := exechelp(); err != nil {
		fmt.Printf("  git:        NOT FOUND (%v)\n", err)
	} else {
		fmt.Printf("  git:        %s\n", out)
	}
	fmt.Printf("  home:       %s\n", syn.Home())
	fmt.Printf("  repos:      %d registered\n", len(cfg.Repos))
	state := "OFF - nothing syncs on its own (turn on: syn auto on)"
	if syn.AutostartInstalled() {
		state = "ON - syncs at login, publishes before shutdown"
	}
	fmt.Printf("  background: %s\n", state)

	var problems []string
	for i := range cfg.Repos {
		st := e.Status(&cfg.Repos[i], false)
		if st.Err != nil || st.Conflict != nil || st.InProgres != "" {
			problems = append(problems, st.String())
		}
	}

	fmt.Println()
	if len(problems) == 0 {
		fmt.Println("  every repository looks healthy")
		return nil
	}
	if len(problems) == 1 {
		fmt.Print("  1 repository needs attention:\n\n")
	} else {
		fmt.Printf("  %d repositories need attention:\n\n", len(problems))
	}
	for _, p := range problems {
		fmt.Print(p)
	}
	return nil
}

func cmdRepoCreate(cfg *syn.Config, creds *syn.Credentials, name string, temp, public bool, values map[string]string) error {
	token := creds.Token("github.com")
	if token == "" {
		return fmt.Errorf("not logged in - run `syn auth login` first")
	}
	gh := syn.NewGitHub(token)

	desc := values["--desc"]
	if temp && desc == "" {
		desc = "syn temporary transfer repo"
	}
	private := !public

	syn.Info("creating GitHub repository %q...", name)
	info, err := gh.CreateRepo(name, desc, private)
	if err != nil {
		return fmt.Errorf("could not create repo: %w", err)
	}
	syn.Info("created %s", info.FullName)

	destPath := values["--path"]
	if destPath == "" {
		destPath = "./" + name
	}
	abs, err := filepath.Abs(destPath)
	if err != nil {
		return err
	}

	syn.Info("cloning to %s...", abs)
	if err := syn.CloneWithToken(info.CloneURL, abs, token, "--quiet"); err != nil {
		return fmt.Errorf("clone failed: %w", err)
	}

	repoName := name
	for cfg.Find(repoName) != nil {
		repoName = repoName + "-2"
	}
	repo := syn.Repo{
		Name:       repoName,
		Path:       abs,
		Remote:     "origin",
		Branch:     info.DefaultBranch,
		Enabled:    true,
		GithubSlug: info.FullName,
		Private:    private,
		Temp:       temp,
	}
	cfg.Repos = append(cfg.Repos, repo)
	if err := cfg.Save(); err != nil {
		return err
	}
	kind := "permanent"
	if temp {
		kind = "temporary"
	}
	syn.Info("registered %q (%s, %s)", repoName, info.FullName, kind)
	syn.Info("token will be used automatically for push/pull")
	if temp {
		syn.Info("drop with: syn tmp drop %s", repoName)
	}
	return nil
}

func cmdTmp(cfg *syn.Config, creds *syn.Credentials, rest []string, has func(string) bool, values map[string]string) error {
	sub := ""
	if len(rest) > 0 {
		sub = rest[0]
	}
	switch sub {
	case "create":
		name := ""
		if len(rest) >= 2 {
			name = rest[1]
		}
		if name == "" {
			name = fmt.Sprintf("syn-tmp-%d", time.Now().Unix())
		}
		return cmdRepoCreate(cfg, creds, name, true, has("--public"), values)

	case "drop":
		key := ""
		if len(rest) >= 2 {
			key = rest[1]
		}
		if key == "" {
			return fmt.Errorf("usage: syn tmp drop <name>")
		}
		repo := cfg.Find(key)
		if repo == nil {
			return fmt.Errorf("repo %q not found", key)
		}
		if !repo.Temp {
			return fmt.Errorf("%q is not a temporary repo - use `syn remove %s` to untrack, or `syn tmp drop --force %s` to delete it from GitHub anyway", key, key, key)
		}
		token := creds.Token("github.com")
		if token == "" {
			return fmt.Errorf("not logged in - run `syn auth login` first")
		}
		gh := syn.NewGitHub(token)
		slug := repo.GithubSlug
		syn.Info("deleting %s from GitHub...", slug)
		if err := gh.DeleteRepo(slug); err != nil {
			return err
		}
		syn.Info("deleted from GitHub")
		cfg.Repos = func() []syn.Repo {
			out := cfg.Repos[:0]
			for _, r := range cfg.Repos {
				if r.Name != repo.Name {
					out = append(out, r)
				}
			}
			return out
		}()
		if err := cfg.Save(); err != nil {
			return err
		}
		syn.Info("unregistered %q (local files at %s are kept)", repo.Name, repo.Path)
		return nil

	case "ls", "list", "":
		found := false
		for i := range cfg.Repos {
			r := &cfg.Repos[i]
			if r.Temp {
				status := "enabled"
				if !r.Enabled {
					status = "disabled"
				}
				fmt.Printf("  %-24s  %s  %s\n", r.Name, r.GithubSlug, status)
				found = true
			}
		}
		if !found {
			fmt.Println("no temporary repos (create one with `syn tmp create [name]`)")
		}
		return nil

	default:
		return fmt.Errorf("usage: syn tmp <create|drop|ls>")
	}
}

func cmdSend(cfg *syn.Config, creds *syn.Credentials, engine *syn.Engine, rest []string, values map[string]string) error {

	if len(rest) == 0 {
		return fmt.Errorf("usage: syn send <file> [@repo-name]\n  or:  syn send <file> (creates a new tmp repo)")
	}

	var files []string
	var target string
	for _, a := range rest {
		if strings.HasPrefix(a, "@") {
			target = strings.TrimPrefix(a, "@")
		} else {
			files = append(files, a)
		}
	}
	if len(files) == 0 {
		return fmt.Errorf("no files specified")
	}

	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			return fmt.Errorf("file not found: %s", f)
		}
	}

	if target != "" {
		repo := cfg.Find(target)
		if repo == nil {
			return fmt.Errorf("repo %q not registered", target)
		}
		return engine.SendFiles(repo, files)
	}

	name, err := engine.SendViaNewTmp(creds, cfg, files)
	if err != nil {
		return err
	}
	syn.Info("receive on other machine: syn pull %s", name)
	syn.Info("then drop when done:      syn tmp drop %s", name)
	return nil
}

func cmdEncrypt(cfg *syn.Config, creds *syn.Credentials, rest []string, values map[string]string) error {
	sub := ""
	if len(rest) > 0 {
		sub = rest[0]
	}
	switch sub {
	case "init":

		if len(rest) < 2 {
			return fmt.Errorf("usage: syn encrypt init <repo-name>")
		}
		repoName := rest[1]
		repo := cfg.Find(repoName)
		if repo == nil {
			return fmt.Errorf("repo %q not registered", repoName)
		}
		key, err := syn.GenerateKey(repoName)
		if err != nil {
			return err
		}
		if err := syn.SetupEncryption(repo.Path, repoName); err != nil {
			return err
		}
		hexKey, _ := syn.ExportKey(repoName)
		syn.Info("encryption enabled for %q", repoName)
		fmt.Printf("\nKey (share with other machines):\n  %s\n\n", hexKey)
		fmt.Printf("On other machine run:\n  syn encrypt add-key %s <key>\n\n", repoName)
		_ = key
		return nil

	case "add-key":

		if len(rest) < 3 {
			return fmt.Errorf("usage: syn encrypt add-key <repo-name> <hex-key>")
		}
		repoName, hexKey := rest[1], rest[2]
		repo := cfg.Find(repoName)
		if repo == nil {
			return fmt.Errorf("repo %q not registered", repoName)
		}
		if err := syn.ImportKey(repoName, hexKey); err != nil {
			return err
		}
		if err := syn.SetupEncryption(repo.Path, repoName); err != nil {
			return err
		}
		syn.Info("key imported and encryption filter installed for %q", repoName)
		return nil

	case "show-key":
		if len(rest) < 2 {
			return fmt.Errorf("usage: syn encrypt show-key <repo-name>")
		}
		hexKey, err := syn.ExportKey(rest[1])
		if err != nil {
			return err
		}
		fmt.Println(hexKey)
		return nil

	case "clean":
		if len(rest) < 2 {
			return fmt.Errorf("clean: need repo name")
		}
		key, err := syn.LoadKey(rest[1])
		if err != nil {
			return err
		}
		return syn.Encrypt(os.Stdout, os.Stdin, key)

	case "smudge":
		if len(rest) < 2 {
			return fmt.Errorf("smudge: need repo name")
		}
		key, err := syn.LoadKey(rest[1])
		if err != nil {
			return err
		}
		return syn.Decrypt(os.Stdout, os.Stdin, key)

	default:
		return fmt.Errorf("usage: syn encrypt <init|add-key|show-key> <repo-name>")
	}
}

func exechelp() (string, error) {
	return syn.GitVersion()
}
