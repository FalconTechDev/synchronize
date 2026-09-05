package syn

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Result struct {
	Repo      string
	Branch    string
	Remote    string
	Path      string
	Committed bool
	Pulled    int
	Pushed    int
	PulledLog []string
	PushedLog []string

	// The files behind those commits: what arrived, and what syn committed
	// on your behalf. Auto-committing your work without saying what went in
	// is how surprises happen.
	PulledFiles    []FileChange
	CommittedFiles []FileChange
	Backup    string
	Conflict  *Conflict
	Err       error

	summaryLogged bool
}

func (r Result) OK() bool { return r.Err == nil && r.Conflict == nil }

func (r Result) tracking() string {
	if r.Remote == "" || r.Branch == "" {
		return r.Branch
	}
	return fmt.Sprintf("%s/%s", r.Remote, r.Branch)
}

func (r Result) Summary() string {
	switch {
	case r.Err != nil:
		return fmt.Sprintf("%s: FAILED - %v", r.Repo, r.Err)
	case r.Conflict != nil:
		return fmt.Sprintf("%s: CONFLICT on %s (%s) - backup branch %s",
			r.Repo, r.Branch, strings.Join(r.Conflict.Files, ", "), r.Conflict.BackupBranch)
	}
	var parts []string
	if r.Committed {
		parts = append(parts, "committed local changes")
	}
	if r.Pulled > 0 {
		parts = append(parts, fmt.Sprintf("pulled %s from %s", plural(r.Pulled, "commit"), r.tracking()))
	}
	if r.Pushed > 0 {
		parts = append(parts, fmt.Sprintf("pushed %s to %s", plural(r.Pushed, "commit"), r.tracking()))
	}
	if len(parts) == 0 {
		parts = append(parts, "already up to date with "+r.tracking())
	}
	return fmt.Sprintf("%s: %s", r.Repo, strings.Join(parts, ", "))
}

func (r Result) Detail() string {
	var b strings.Builder
	b.WriteString(r.Summary())
	for _, line := range r.PulledLog {
		fmt.Fprintf(&b, "\n           down  %s", line)
	}
	writeFiles(&b, r.PulledFiles)
	for _, line := range r.PushedLog {
		fmt.Fprintf(&b, "\n           up    %s", line)
	}
	writeFiles(&b, r.CommittedFiles)
	return b.String()
}

// writeFiles lists what actually moved, trimmed so one busy sync cannot
// bury everything else in the log.
func writeFiles(b *strings.Builder, files []FileChange) {
	const max = 12
	for i, f := range files {
		if i == max {
			fmt.Fprintf(b, "\n                 ... and %d more", len(files)-max)
			break
		}
		fmt.Fprintf(b, "\n                 %-9s %s", f.Status, f.Path)
	}
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

type Engine struct {
	Cfg      *Config
	State    *State
	Creds    *Credentials
	Notifier *Notifier
}

func NewEngine(cfg *Config) *Engine {
	return &Engine{Cfg: cfg, State: LoadState(), Creds: LoadCredentials(), Notifier: NewNotifier(cfg)}
}

func (e *Engine) tokenFor(r *Repo) string {
	rawURL, err := RemoteURL(r.Path, r.Remote)
	if err != nil {
		return ""
	}
	host := RemoteHost(rawURL)
	if host == "github.com" && !strings.HasPrefix(rawURL, "git@") {
		return e.Creds.Token("github.com")
	}
	return ""
}

func (e *Engine) gitR(r *Repo, args ...string) (string, error) {
	return gitWithToken(r.Path, e.tokenFor(r), args...)
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown-host"
	}
	return h
}

func (e *Engine) prepare(r *Repo) (string, error) {
	if !IsRepo(r.Path) {
		return "", notAWorkingTree(r)
	}
	if op := InProgress(r.Path); op != "" {
		return "", fmt.Errorf("a %s is already in progress - finish it or run `syn resolve %s --abort`", op, r.Name)
	}
	branch := r.Branch
	cur, err := CurrentBranch(r.Path)
	if err != nil {
		return "", err
	}
	if branch == "" {
		branch = cur
	} else if cur != branch {
		return "", fmt.Errorf("repo is on %q but is pinned to %q - switch branches or run `syn set %s branch %s`", cur, branch, r.Name, cur)
	}
	if _, err := RemoteURL(r.Path, r.Remote); err != nil {
		return "", fmt.Errorf("remote %q is not configured in %s", r.Remote, r.Path)
	}
	return branch, nil
}

func (e *Engine) backup(r *Repo, branch string) string {
	if !e.Cfg.Backup {
		return ""
	}
	base := fmt.Sprintf("syn/backup/%s/%s/%s",
		sanitizeRef(hostname()), sanitizeRef(branch), time.Now().Format("20060102-150405"))
	
	name := base
	for i := 2; gitOK(r.Path, "rev-parse", "--verify", "--quiet", name); i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	if _, err := git(r.Path, "branch", name, "HEAD"); err != nil {
		Warn("%s: could not create backup branch: %v", r.Name, err)
		return ""
	}
	Debug("%s: backup branch %s", r.Name, name)
	if e.Cfg.PushBackup {
		if _, err := gitWithToken(r.Path, e.Creds.Token("github.com"), "push", "--quiet", r.Remote, name); err != nil {
			Warn("%s: backup branch %s stayed local: %v", r.Name, name, err)
		}
	}
	return name
}

func sanitizeRef(s string) string {
	repl := strings.NewReplacer(" ", "-", "~", "-", "^", "-", ":", "-", "?", "-", "*", "-", "[", "-", "\\", "-")
	return strings.Trim(repl.Replace(s), "-.")
}

func (e *Engine) autoCommit(r *Repo) (bool, []FileChange, error) {
	dirty, err := IsDirty(r.Path)
	if err != nil || !dirty {
		return false, nil, err
	}
	files := ChangedFilesDetailed(r.Path)
	if _, err := git(r.Path, "add", "-A"); err != nil {
		return false, nil, err
	}
	msg := strings.NewReplacer(
		"{host}", hostname(),
		"{time}", time.Now().Format("2006-01-02 15:04"),
	).Replace(e.Cfg.CommitTemplate)
	if _, err := gitCommit(r.Path, msg); err != nil {
		return false, nil, err
	}
	return true, files, nil
}

func (e *Engine) recordConflict(r *Repo, branch, backupBranch, reason string, files []string) *Conflict {
	c := &Conflict{
		Repo:         r.Name,
		Branch:       branch,
		BackupBranch: backupBranch,
		Files:        files,
		Reason:       reason,
		At:           time.Now(),
	}
	e.State.Conflicts[r.Name] = *c
	if err := e.State.Save(); err != nil {
		Warn("could not persist conflict state: %v", err)
	}
	Errorf("%s: conflict on %s (%s)", r.Name, branch, reason)
	e.Notifier.Send("conflict", Alert{
		Title: "synchronize: конфликт",
		Short: fmt.Sprintf("%s (%s) — запусти: syn resolve %s", r.Name, branch, r.Name),
		Full: fmt.Sprintf("synchronize: конфликт\n\nРепо: %s\nВетка: %s\nМашина: %s\nПричина: %s\nФайлы: %s\nБэкап-ветка: %s\n\nПочини вручную и запусти: syn resolve %s",
			r.Name, branch, hostname(), reason, strings.Join(files, ", "), backupBranch, r.Name),
	})
	return c
}

func (e *Engine) clearConflict(name string) {
	if _, ok := e.State.Conflicts[name]; ok {
		delete(e.State.Conflicts, name)
		_ = e.State.Save()
	}
}

func (e *Engine) integrate(r *Repo, branch string) (conflicted bool, files []string, err error) {
	ref := r.Remote + "/" + branch
	var gerr error
	if e.Cfg.Strategy == "merge" {
		_, gerr = git(r.Path, "merge", "--no-edit", ref)
	} else {
		_, gerr = git(r.Path, "rebase", ref)
	}
	if gerr == nil {
		return false, nil, nil
	}
	files = ConflictedFiles(r.Path)
	if len(files) == 0 && InProgress(r.Path) == "" {
		
		return false, nil, gerr
	}
	if e.Cfg.Strategy == "merge" {
		_, _ = git(r.Path, "merge", "--abort")
	} else {
		_, _ = git(r.Path, "rebase", "--abort")
	}
	return true, files, nil
}

// withHooks runs the user's hooks around one sync operation. Hooks live in
// ~/.synchronize/config.json - never inside the repository - so a command can
// only ever come from this machine, not from whoever pushed last.
func (e *Engine) withHooks(r *Repo, op string, run func(*Repo) Result) Result {
	res := run(r)

	if res.Conflict != nil {
		e.runHooks(r, "on-conflict", r.Hooks.OnConflict)
		return res
	}
	if res.Err != nil {
		return res
	}

	firePull := res.Pulled > 0 && len(r.Hooks.AfterPull) > 0
	firePush := op == "push" && res.Pushed > 0 && len(r.Hooks.AfterPush) > 0
	if !firePull && !firePush {
		return res
	}

	// The caller logs the summary only after we return, so say what happened
	// first - otherwise the hook output reads as if it ran before the sync.
	Info("%s", res.Detail())
	res.summaryLogged = true

	if firePull {
		e.runHooks(r, "after-pull", r.Hooks.AfterPull)
	}
	if firePush {
		e.runHooks(r, "after-push", r.Hooks.AfterPush)
	}
	return res
}

func (e *Engine) Pull(r *Repo) Result {
	return e.withHooks(r, "pull", e.pull)
}

func (e *Engine) pull(r *Repo) Result {
	res := Result{Repo: r.Name, Remote: r.Remote, Path: r.Path}
	branch, err := e.prepare(r)
	if err != nil {
		res.Err = err
		return res
	}
	res.Branch = branch

	if _, err := e.gitR(r, "fetch", "--prune", r.Remote); err != nil {
		res.Err = err
		return res
	}
	if !RemoteBranchExists(r.Path, r.Remote, branch) {
		Info("%s: %s/%s does not exist yet, nothing to pull", r.Name, r.Remote, branch)
		return res
	}
	ahead, behind, err := AheadBehind(r.Path, branch, r.Remote)
	if err != nil {
		res.Err = err
		return res
	}
	dirty, err := IsDirty(r.Path)
	if err != nil {
		res.Err = err
		return res
	}
	if behind == 0 {
		e.markSynced(r)
		return res
	}

	res.PulledLog = CommitLog(r.Path, "HEAD", r.Remote+"/"+branch, 5)

	if ahead > 0 || dirty {
		res.Backup = e.backup(r, branch)
	}

	stashed := false
	if dirty {
		if _, err := git(r.Path, "stash", "push", "--include-untracked", "-m", "syn-auto-stash"); err != nil {
			res.Err = fmt.Errorf("could not set aside local changes: %w", err)
			return res
		}
		stashed = true
	}

	// Remember where we stood, so afterwards we can say which files arrived
	// rather than only how many commits did.
	before, _ := CurrentCommit(r.Path)

	conflicted, files, err := e.integrate(r, branch)
	if err != nil {
		if stashed {
			_, _ = git(r.Path, "stash", "pop")
		}
		res.Err = err
		return res
	}
	if conflicted {
		if stashed {
			_, _ = git(r.Path, "stash", "pop")
		}
		res.Conflict = e.recordConflict(r, branch, res.Backup, "local commits conflict with the remote", files)
		return res
	}
	res.Pulled = behind
	if before != "" {
		if now, err := CurrentCommit(r.Path); err == nil {
			res.PulledFiles = FilesBetween(r.Path, before, now)
		}
	}

	if stashed {
		if _, err := git(r.Path, "stash", "pop"); err != nil {
			if cf := ConflictedFiles(r.Path); len(cf) > 0 {
				res.Conflict = e.recordConflict(r, branch, res.Backup,
					"your uncommitted edits conflict with the pulled changes", cf)
				return res
			}
			res.Err = fmt.Errorf("could not restore local changes (they are safe in `git stash list`): %w", err)
			return res
		}
	}
	e.clearConflict(r.Name)
	e.markSynced(r)
	return res
}

func (e *Engine) Push(r *Repo) Result {
	return e.withHooks(r, "push", e.push)
}

func (e *Engine) push(r *Repo) Result {
	res := Result{Repo: r.Name, Remote: r.Remote, Path: r.Path}
	branch, err := e.prepare(r)
	if err != nil {
		res.Err = err
		return res
	}
	res.Branch = branch

	committed, files, err := e.autoCommit(r)
	if err != nil {
		res.Err = err
		return res
	}
	res.Committed = committed
	res.CommittedFiles = files

	if _, err := e.gitR(r, "fetch", "--prune", r.Remote); err != nil {
		res.Err = err
		return res
	}
	ahead, behind, err := AheadBehind(r.Path, branch, r.Remote)
	if err != nil {
		res.Err = err
		return res
	}
	if ahead == 0 && behind == 0 {
		e.markSynced(r)
		return res
	}
	if ahead > 0 {
		res.Backup = e.backup(r, branch)
	}
	if behind > 0 {
		res.PulledLog = CommitLog(r.Path, "HEAD", r.Remote+"/"+branch, 5)
		conflicted, files, err := e.integrate(r, branch)
		if err != nil {
			res.Err = err
			return res
		}
		if conflicted {
			res.Conflict = e.recordConflict(r, branch, res.Backup,
				"your commits conflict with what is already on the remote", files)
			return res
		}
		res.Pulled = behind
	}
	ahead, _, err = AheadBehind(r.Path, branch, r.Remote)
	if err != nil {
		res.Err = err
		return res
	}
	if ahead == 0 {
		e.markSynced(r)
		return res
	}
	res.PushedLog = CommitLog(r.Path, r.Remote+"/"+branch, "HEAD", 5)
	if _, err := e.gitR(r, "push", r.Remote, branch); err != nil {
		res.Err = err
		return res
	}
	res.Pushed = ahead
	e.clearConflict(r.Name)
	e.markSynced(r)
	return res
}

func (e *Engine) Sync(r *Repo) Result {
	res := e.Pull(r)
	if !res.OK() {
		return res
	}
	push := e.Push(r)
	push.Pulled += res.Pulled
	if push.Backup == "" {
		push.Backup = res.Backup
	}
	return push
}

func (e *Engine) markSynced(r *Repo) {
	e.State.LastSync[r.Name] = time.Now()
	_ = e.State.Save()
}

func (e *Engine) RunAll(repos []*Repo, op func(*Repo) Result) []Result {
	out := make([]Result, 0, len(repos))
	for _, r := range repos {
		res := op(r)
		switch {
		case res.Err != nil:
			Errorf("%s", res.Summary())
		case res.Conflict != nil:
			
		default:
			if !res.summaryLogged {
				Info("%s", res.Detail())
			}
		}
		out = append(out, res)
	}
	return out
}
