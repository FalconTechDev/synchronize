

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

 - automatic git sync across your machines

USAGE
  syn <command> [repo] [flags]

  Most commands take a repository name (as shown by ` + "`syn list`" + `) or a path.
  Pass --all, or nothing at all, to act on every enabled repository.

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
  repo ls                list your GitHub repos (requires auth)
  list                   show registered repositories
  remove <repo>          stop tracking a repository (files untouched)
  enable <repo>          include it in --all runs
  disable <repo>         exclude it from --all runs
  set <repo> <k> <v>     change name, remote, branch, interval

FILE RULES  (which files to sync; default: everything)
  files <repo>           show active include/exclude patterns
  files include <repo> <glob>   only sync matching paths (e.g. "docs/**")
  files exclude <repo> <glob>   never sync matching paths (e.g. "*.log")
  files clear <repo>     remove all patterns (back to syncing everything)

INTERACTIVE
  shell                  open a session that stays until you type exit

SYNCING
  pull  [repo] [--force]    bring remote changes in    (--force: reset to remote)
  push  [repo] [--force]    commit + publish local work (--force: force-with-lease)
  sync  [repo]              pull, then push
  status [repo] [--fetch]   show what would happen
  watch [repo]              run the loop: sync at per-repo interval, push on exit

HOOKS  (run a command on THIS machine after a sync changed something)
  hook ls                          show configured hooks
  hook add <repo> <event> <cmd>    event: after-pull, after-push, on-conflict
  hook run <repo> <event>          try it now without syncing
  hook clear <repo> [event]        remove hooks
  hook timeout <repo> <seconds>    how long a hook may run (default 300)

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
  prune [repo] [--days N]  delete safety branches older than N days
                           (default: keep_backup_days; --days 0 deletes all)

AUTOSTART
  install                run at startup / on shutdown via the OS scheduler
  uninstall              remove those entries
  autostart              show whether they are installed
  start                  start the installed watcher now

OTHER
  config                 print the current configuration
  config set <k> <v>     backup, push_backup, strategy, interval_minutes,
                         notify, commit_template, keep_backup_days
  notify [text]          send yourself a desktop notification
  log [n]                last n log lines (default 40)
  log --live             stream the log in real time
  diff [repo]            show uncommitted diff
  serve [--addr :7391]   web dashboard with live log and pull/push buttons
  doctor                 check that everything is wired up
  version                print the version

GLOBAL FLAGS
  --all       act on every enabled repository
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
		selector = ""
	}

	if needsGit(cmd) {
		if err := requireGit(); err != nil {
			return err
		}
	}

	return dispatch(cmd, rest, arg, selector, has, values)
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
		if has("--live") || has("-f") {
			stop := make(chan struct{})
			fmt.Println("streaming log (Ctrl+C to stop)...")
			return syn.LiveTail(os.Stdout, stop)
		}
		n := 50
		if v := values["--lines"]; v != "" {
			fmt.Sscanf(v, "%d", &n)
		}
		fmt.Println(syn.TailLog(n))
		return nil

	case "diff":
		repos, err := cfg.Selected(selector)
		if err != nil {
			return err
		}
		for _, r := range repos {
			branch := r.Branch
			if branch == "" {
				branch, _ = syn.CurrentBranch(r.Path)
			}
			out, err := syn.Diff(r.Path, r.Remote, branch)
			if err != nil {
				return err
			}
			if len(repos) > 1 {
				fmt.Printf("=== %s ===\n", r.Name)
			}
			if out == "" {
				fmt.Println("(no changes)")
			} else {
				fmt.Println(out)
			}
		}
		return nil

	case "tray":
		syn.RunTray(cfg, engine)
		return nil

	case "notify":
		title, body := "synchronize", strings.Join(rest, " ")
		if body == "" {
			body = "notifications are working"
		}
		if i := strings.Index(body, ": "); i > 0 && i < 40 {
			title, body = body[:i], body[i+2:]
		}
		syn.ToastNotify(title, body)
		syn.Info("notification sent")
		return nil

	case "send", "receive", "encrypt", "tmp":
		fmt.Printf("syn %s is coming soon — not available yet\n", cmd)
		return nil

	case "shell", "sh", "i":
		return cmdShell()

	case "hook", "hooks":
		return cmdHook(cfg, engine, rest)

	case "serve":
		addr := values["--addr"]
		return engine.Serve(cfg, addr)
	case "list":
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
		return runOp(engine, cfg, selector, pick(has("--force"), engine.ForcePull, engine.Pull))
	case "push":
		return runOp(engine, cfg, selector, pick(has("--force"), engine.ForcePush, engine.Push))
	case "sync":
		return runOp(engine, cfg, selector, engine.Sync)

	case "status":
		return cmdStatus(engine, cfg, selector, has("--fetch"))
	case "watch":
		repos, err := cfg.Selected(selector)
		if err != nil {
			return err
		}
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

	case "install":
		if err := syn.InstallAutostart(); err != nil {
			return err
		}
		syn.Info("synchronize will now sync on startup and push on shutdown")
		return nil
	case "uninstall":
		return syn.UninstallAutostart()
	case "autostart":
		fmt.Print(syn.AutostartStatus())
		return nil
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

func runOp(e *syn.Engine, cfg *syn.Config, selector string, op func(*syn.Repo) syn.Result) error {
	repos, err := cfg.Selected(selector)
	if err != nil {
		return err
	}
	results := e.RunAll(repos, op)
	failed := 0
	for _, r := range results {
		if !r.OK() {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d repositor(ies) need attention - see `syn status`", failed, len(results))
	}
	return nil
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
	name := filepath.Base(root)
	for cfg.Find(name) != nil {
		name += "-2"
	}
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
	syn.Info("next: `syn install` to sync automatically at startup and shutdown")
	return nil
}

func cmdList(cfg *syn.Config) error {
	if len(cfg.Repos) == 0 {
		fmt.Println("no repositories registered - run `syn init` inside a git repo")
		return nil
	}
	for _, r := range cfg.Repos {
		state := "enabled"
		if !r.Enabled {
			state = "disabled"
		}
		branch := r.Branch
		if branch == "" {
			branch = "(current)"
		}
		fmt.Printf("%-20s %-8s %s [%s %s]\n", r.Name, state, r.Path, r.Remote, branch)
	}
	return nil
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
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return fmt.Errorf("interval must be a non-negative number of minutes (0 = use global)")
		}
		r.IntervalMinutes = n
	default:
		return fmt.Errorf("cannot set %q - use name, remote, branch or interval", field)
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
		slug := rest[1]
		parts := strings.SplitN(slug, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("repository must be owner/repo (e.g. torvalds/linux)")
		}
		return cmdRepoAdd(cfg, creds, slug, values)

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

func cmdRepoAdd(cfg *syn.Config, creds *syn.Credentials, slug string, values map[string]string) error {
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

	if values["--path"] == "" {
		if chosen, err := confirmDestination(slug, abs); err != nil {
			return err
		} else if chosen != "" {
			abs = chosen
		}
	}

	if existing := cfg.Find(abs); existing != nil {
		return fmt.Errorf("already registered as %q", existing.Name)
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

	name := repoName
	for cfg.Find(name) != nil {
		name = slug + "-2"
	}

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

func cmdHook(cfg *syn.Config, e *syn.Engine, rest []string) error {
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
		command := strings.Join(rest[3:], " ")
		switch rest[2] {
		case "after-pull":
			r.Hooks.AfterPull = append(r.Hooks.AfterPull, command)
		case "after-push":
			r.Hooks.AfterPush = append(r.Hooks.AfterPush, command)
		case "on-conflict":
			r.Hooks.OnConflict = append(r.Hooks.OnConflict, command)
		default:
			return fmt.Errorf("event must be after-pull, after-push or on-conflict")
		}
		if err := cfg.Save(); err != nil {
			return err
		}
		syn.Info("%s: %s hook added", r.Name, rest[2])
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
	return fmt.Errorf("unknown hook subcommand %q (try: add, ls, clear, timeout, run)", rest[0])
}

func listHooks(cfg *syn.Config) error {
	any := false
	for i := range cfg.Repos {
		r := &cfg.Repos[i]
		if r.Hooks.IsEmpty() {
			continue
		}
		any = true
		fmt.Printf("%s  (timeout %ds)\n", r.Name, r.Hooks.Timeout())
		show := func(label string, cmds []string) {
			for _, c := range cmds {
				fmt.Printf("  %-12s %s\n", label, c)
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
		return fmt.Errorf("usage: syn files <show|include|exclude|remove-pattern|clear> <repo> [pattern]")
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

func cmdStatus(e *syn.Engine, cfg *syn.Config, selector string, fetch bool) error {
	repos, err := cfg.Selected(selector)
	if err != nil {
		return err
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
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return fmt.Errorf("interval_minutes must be a positive number")
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

func cmdDoctor(e *syn.Engine, cfg *syn.Config) error {
	fmt.Printf("synchronize %s on %s/%s\n", Version, runtime.GOOS, runtime.GOARCH)
	if out, err := exechelp(); err != nil {
		fmt.Printf("  git:        NOT FOUND (%v)\n", err)
	} else {
		fmt.Printf("  git:        %s\n", out)
	}
	fmt.Printf("  home:       %s\n", syn.Home())
	fmt.Printf("  repos:      %d registered\n", len(cfg.Repos))
	fmt.Printf("  telegram:   %s\n", map[bool]string{true: "configured", false: "not configured"}[cfg.Telegram.Token != ""])
	fmt.Println("  autostart:")
	fmt.Print(syn.AutostartStatus())

	problems := 0
	for i := range cfg.Repos {
		st := e.Status(&cfg.Repos[i], false)
		if st.Err != nil || st.Conflict != nil || st.InProgres != "" {
			problems++
			fmt.Print(st.String())
		}
	}
	if problems == 0 {
		fmt.Println("  all repositories look healthy")
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
