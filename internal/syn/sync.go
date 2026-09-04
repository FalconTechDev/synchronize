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
	Backup    string
	Conflict  *Conflict
	Err       error
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
	for _, line := range r.PushedLog {
		fmt.Fprintf(&b, "\n           up    %s", line)
	}
	return b.String()
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
		return "", fmt.Errorf("%s is not a git working tree", r.Path)
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

func (e *Engine) autoCommit(r *Repo) (bool, error) {
	dirty, err := IsDirty(r.Path)
	if err != nil || !dirty {
		return false, err
	}
	if _, err := git(r.Path, "add", "-A"); err != nil {
		return false, err
	}
	msg := strings.NewReplacer(
		"{host}", hostname(),
		"{time}", time.Now().Format("2006-01-02 15:04"),
	).Replace(e.Cfg.CommitTemplate)
	if _, err := gitCommit(r.Path, msg); err != nil {
		return false, err
	}
	return true, nil
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
	body := fmt.Sprintf("synchronize: конфликт\n\nРепо: %s\nВетка: %s\nМашина: %s\nПричина: %s\nФайлы: %s\nБэкап-ветка: %s\n\nПочини вручную и запусти: syn resolve %s",
		r.Name, branch, hostname(), reason, strings.Join(files, ", "), backupBranch, r.Name)
	e.Notifier.Send("conflict", body)
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

func (e *Engine) Pull(r *Repo) Result {
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
	res := Result{Repo: r.Name, Remote: r.Remote, Path: r.Path}
	branch, err := e.prepare(r)
	if err != nil {
		res.Err = err
		return res
	}
	res.Branch = branch

	committed, err := e.autoCommit(r)
	if err != nil {
		res.Err = err
		return res
	}
	res.Committed = committed

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
			Info("%s", res.Detail())
		}
		out = append(out, res)
	}
	return out
}
