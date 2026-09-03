package syn

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (e *Engine) Resolve(r *Repo, mode string) Result {
	res := Result{Repo: r.Name}
	op := InProgress(r.Path)
	if op != "" {
		switch mode {
		case "abort":
			if _, err := git(r.Path, op, "--abort"); err != nil {
				res.Err = err
				return res
			}
			Info("%s: aborted the in-progress %s", r.Name, op)
			e.clearConflict(r.Name)
			return res
		default:
			if _, err := git(r.Path, "add", "-A"); err != nil {
				res.Err = err
				return res
			}
			if _, err := git(r.Path, op, "--continue"); err != nil {
				if files := ConflictedFiles(r.Path); len(files) > 0 {
					res.Err = fmt.Errorf("still conflicted: %s", strings.Join(files, ", "))
					return res
				}
				res.Err = err
				return res
			}
			Info("%s: finished the %s", r.Name, op)
		}
	}
	if mode == "abort" {
		e.clearConflict(r.Name)
		return res
	}
	if files := ConflictedFiles(r.Path); len(files) > 0 {
		res.Err = fmt.Errorf("unresolved conflicts remain: %s", strings.Join(files, ", "))
		return res
	}
	if op == "" {
		
		
		
		if c, ok := e.State.Conflicts[r.Name]; ok {
			if replayed, err := e.replay(r, c.Branch, mode); err != nil {
				res.Err = err
				return res
			} else if replayed {
				res.Branch = c.Branch
				res.Err = fmt.Errorf("conflict markers are now in your working tree (%s) - fix them, then run `syn resolve %s`",
					strings.Join(ConflictedFiles(r.Path), ", "), r.Name)
				return res
			}
		}
	}
	return e.Sync(r)
}

func (e *Engine) replay(r *Repo, branch, mode string) (bool, error) {
	if branch == "" {
		b, err := CurrentBranch(r.Path)
		if err != nil {
			return false, err
		}
		branch = b
	}
	token := e.tokenFor(r)
	if _, err := gitWithToken(r.Path, token, "fetch", "--prune", r.Remote); err != nil {
		return false, err
	}
	ref := r.Remote + "/" + branch
	args := []string{"rebase"}
	if e.Cfg.Strategy == "merge" {
		args = []string{"merge", "--no-edit"}
	}
	
	
	switch mode {
	case "ours":
		args = append(args, pickX(e.Cfg.Strategy, "-Xtheirs", "-Xours"))
	case "theirs":
		args = append(args, pickX(e.Cfg.Strategy, "-Xours", "-Xtheirs"))
	}
	args = append(args, ref)
	if _, err := git(r.Path, args...); err != nil {
		if len(ConflictedFiles(r.Path)) > 0 || InProgress(r.Path) != "" {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

func pickX(strategy, rebaseOpt, mergeOpt string) string {
	if strategy == "merge" {
		return mergeOpt
	}
	return rebaseOpt
}

func (e *Engine) ForcePull(r *Repo) Result {
	res := Result{Repo: r.Name}
	branch, err := e.prepare(r)
	if err != nil {
		res.Err = err
		return res
	}
	res.Branch = branch
	token := e.tokenFor(r)
	if _, err := gitWithToken(r.Path, token, "fetch", "--prune", r.Remote); err != nil {
		res.Err = err
		return res
	}
	if !RemoteBranchExists(r.Path, r.Remote, branch) {
		res.Err = fmt.Errorf("%s/%s does not exist on the remote", r.Remote, branch)
		return res
	}
	
	if committed, err := e.autoCommit(r); err != nil {
		res.Err = err
		return res
	} else {
		res.Committed = committed
	}
	res.Backup = e.backup(r, branch)
	if _, err := git(r.Path, "reset", "--hard", r.Remote+"/"+branch); err != nil {
		res.Err = err
		return res
	}
	_, behind, _ := AheadBehind(r.Path, branch, r.Remote)
	res.Pulled = behind
	e.clearConflict(r.Name)
	e.markSynced(r)
	Info("%s: reset to %s/%s (previous state kept on %s)", r.Name, r.Remote, branch, res.Backup)
	return res
}

func (e *Engine) ForcePush(r *Repo) Result {
	res := Result{Repo: r.Name}
	branch, err := e.prepare(r)
	if err != nil {
		res.Err = err
		return res
	}
	res.Branch = branch
	token := e.tokenFor(r)
	committed, err := e.autoCommit(r)
	if err != nil {
		res.Err = err
		return res
	}
	res.Committed = committed
	if _, err := gitWithToken(r.Path, token, "fetch", "--prune", r.Remote); err != nil {
		res.Err = err
		return res
	}
	res.Backup = e.backup(r, branch)
	if _, err := gitWithToken(r.Path, token, "push", "--force-with-lease", r.Remote, branch); err != nil {
		res.Err = err
		return res
	}
	ahead, _, _ := AheadBehind(r.Path, branch, r.Remote)
	res.Pushed = ahead
	e.clearConflict(r.Name)
	e.markSynced(r)
	Info("%s: force-pushed %s (remote state kept on %s)", r.Name, branch, res.Backup)
	return res
}

func Backups(r *Repo) ([]string, error) {
	out, err := git(r.Path, "for-each-ref", "--sort=-creatordate", "--format=%(refname:short)", "refs/heads/syn/backup")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

func (e *Engine) RestoreBackup(r *Repo, backupBranch string) error {
	branch, err := e.prepare(r)
	if err != nil {
		return err
	}
	if !gitOK(r.Path, "rev-parse", "--verify", "--quiet", backupBranch) {
		return fmt.Errorf("no such backup branch: %s", backupBranch)
	}
	if dirty, _ := IsDirty(r.Path); dirty {
		if _, err := e.autoCommit(r); err != nil {
			return err
		}
	}
	safety := e.backup(r, branch)
	if _, err := git(r.Path, "reset", "--hard", backupBranch); err != nil {
		return err
	}
	Info("%s: %s restored from %s (previous state kept on %s)", r.Name, branch, backupBranch, safety)
	return nil
}

func (e *Engine) PruneBackups(r *Repo, days int) (int, error) {
	if days < 0 {
		days = e.Cfg.KeepBackupDays
		if days <= 0 {
			return 0, nil
		}
	}
	out, err := git(r.Path, "for-each-ref", "--format=%(refname:short) %(creatordate:unix)", "refs/heads/syn/backup")
	if err != nil || out == "" {
		return 0, err
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	deleted := 0
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		name := fields[0]
		ts, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		if ts <= cutoff {
			if _, err := git(r.Path, "branch", "-D", name); err == nil {
				deleted++
				if e.Cfg.PushBackup {
					_, _ = gitWithToken(r.Path, e.tokenFor(r), "push", "--quiet", r.Remote, "--delete", name)
				}
			}
		}
	}
	return deleted, nil
}
