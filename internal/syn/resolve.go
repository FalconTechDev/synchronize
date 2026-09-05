package syn

import (
	"fmt"
	"os"
	"path/filepath"
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
		// A stash that would not go back leaves files unmerged with no rebase
		// to continue, so --ours and --theirs have to settle them here - the
		// replay below would never be reached.
		if mode == "ours" || mode == "theirs" {
			if err := e.takeSide(r, files, mode); err != nil {
				res.Err = err
				return res
			}
			Info("%s: kept the %s side of %s", r.Name,
				pickText(mode == "ours", "local", "remote"), strings.Join(files, ", "))
			e.clearConflict(r.Name)
			return e.Sync(r)
		}
		// The markers being gone is how someone says they have finished. Git
		// cannot tell: the index stays unmerged until the file is staged. So
		// stage it for them, rather than sending them round the same loop.
		if left := withMarkers(r.Path, files); len(left) > 0 {
			res.Err = fmt.Errorf("conflict markers are still in: %s\n"+
				"        keep the part you want, delete the <<<<<<< ======= >>>>>>> lines,\n"+
				"        then run `syn resolve %s` again\n"+
				"        or take one side outright: syn resolve %s --ours | --theirs",
				strings.Join(left, ", "), r.Name, r.Name)
			return res
		}
		for _, f := range files {
			if _, err := git(r.Path, "add", "--", f); err != nil {
				res.Err = fmt.Errorf("could not stage %s: %w", f, err)
				return res
			}
		}
		Info("%s: markers are gone from %s", r.Name, strings.Join(files, ", "))
		e.clearConflict(r.Name)
		e.dropAutoStash(r)
		return e.Sync(r)
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

	if committed, _, err := e.autoCommit(r); err != nil {
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
	committed, _, err := e.autoCommit(r)
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
		if _, _, err := e.autoCommit(r); err != nil {
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

// PruneBackups removes safety branches older than days, locally and - because
// no other machine can reach them - the ones this machine pushed as well.
// Branches another machine left on the remote are its own to clear.
func (e *Engine) PruneBackups(r *Repo, days int) (int, error) {
	if days < 0 {
		days = e.Cfg.KeepBackupDays
		if days <= 0 {
			return 0, nil
		}
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()

	deleted := 0
	for _, b := range agedBackups(r.Path, "refs/heads/syn/backup", cutoff) {
		if _, err := git(r.Path, "branch", "-D", b); err != nil {
			Warn("%s: could not delete %s: %v", r.Name, b, err)
			continue
		}
		deleted++
		e.deleteRemoteBackup(r, b)
	}

	// Ones we pushed in the past stay on the remote even once push_backup is
	// off, so clear those too rather than leaving them there for good.
	for _, b := range agedBackups(r.Path, "refs/remotes/"+r.Remote+"/syn/backup", cutoff) {
		e.deleteRemoteBackup(r, strings.TrimPrefix(b, r.Remote+"/"))
	}
	return deleted, nil
}

func agedBackups(dir, refPrefix string, cutoff int64) []string {
	out, err := git(dir, "for-each-ref", "--format=%(refname:short) %(creatordate:unix)", refPrefix)
	if err != nil || out == "" {
		return nil
	}
	var old []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		ts, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || ts > cutoff {
			continue
		}
		old = append(old, fields[0])
	}
	return old
}

// deleteRemoteBackup reports a failure rather than swallowing it: a prune that
// quietly does nothing is how the branch list grows without anyone noticing.
func (e *Engine) deleteRemoteBackup(r *Repo, branch string) {
	if !gitOK(r.Path, "rev-parse", "--verify", "--quiet", r.Remote+"/"+branch) {
		return
	}
	if _, err := gitWithToken(r.Path, e.tokenFor(r), "push", "--quiet", r.Remote, "--delete", branch); err != nil {
		Warn("%s: %s is still on %s: %v", r.Name, branch, r.Remote, err)
		return
	}
	Debug("%s: removed %s from %s", r.Name, branch, r.Remote)
}

// takeSide settles unmerged paths without a rebase to continue. A file one
// side deleted has no version to check out, so it is added or removed
// instead of being restored.
func (e *Engine) takeSide(r *Repo, files []string, mode string) error {
	// Reaching here means a stash would not go back on, and in that conflict
	// git's sides are the other way round: stage 2 holds what we just pulled,
	// stage 3 the edit that was set aside. So "keep mine" is git's --theirs.
	flag := "--theirs"
	if mode == "theirs" {
		flag = "--ours"
	}
	for _, f := range files {
		if _, err := git(r.Path, "checkout", flag, "--", f); err != nil {
			if mode == "ours" {
				_, err = git(r.Path, "add", "--", f)
			} else {
				_, err = git(r.Path, "rm", "--force", "--quiet", "--", f)
			}
			if err != nil {
				return fmt.Errorf("could not settle %s: %w", f, err)
			}
			continue
		}
		if _, err := git(r.Path, "add", "--", f); err != nil {
			return fmt.Errorf("could not settle %s: %w", f, err)
		}
	}
	return nil
}

func pickText(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}

// withMarkers reports which of these files still carry conflict markers, so
// "I have finished editing" can be told apart from "I have not started".
func withMarkers(dir string, files []string) []string {
	var left []string
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			continue // deleted on one side; there is nothing to read
		}
		text := string(b)
		if strings.Contains(text, "\n<<<<<<< ") || strings.HasPrefix(text, "<<<<<<< ") {
			left = append(left, f)
		}
	}
	return left
}

// dropAutoStash removes the stash a pull left behind once its content has been
// dealt with, so copies of your work do not pile up in the stash list unseen.
func (e *Engine) dropAutoStash(r *Repo) {
	out, err := git(r.Path, "stash", "list", "--format=%gd %gs")
	if err != nil || out == "" {
		return
	}
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "syn-auto-stash") {
			continue
		}
		if ref := strings.Fields(line); len(ref) > 0 {
			if _, err := git(r.Path, "stash", "drop", ref[0]); err == nil {
				Debug("%s: dropped %s", r.Name, ref[0])
			}
		}
		return
	}
}
