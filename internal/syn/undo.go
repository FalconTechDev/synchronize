package syn

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// LastCommitSubject reads the subject line of HEAD.
func LastCommitSubject(dir string) (string, error) {
	out, err := git(dir, "log", "-1", "--pretty=format:%s")
	if err != nil {
		return "", err
	}
	if out == "" {
		return "", fmt.Errorf("there are no commits yet")
	}
	return out, nil
}

// IsAutoCommit decides whether a subject looks like one syn wrote, by taking
// the fixed part of the commit template - everything before the first
// placeholder - and asking whether the subject starts with it.
func IsAutoCommit(subject, template string) bool {
	if template == "" {
		template = DefaultConfig().CommitTemplate
	}
	prefix := template
	if i := strings.Index(prefix, "{"); i >= 0 {
		prefix = prefix[:i]
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return false
	}
	return strings.HasPrefix(subject, prefix)
}

// IsCommitOnRemote reports whether HEAD has already been published, so undo
// can say plainly that taking it back means rewriting shared history.
func IsCommitOnRemote(dir, remote string) (bool, error) {
	head, err := CurrentCommit(dir)
	if err != nil {
		return false, err
	}
	out, err := git(dir, "branch", "--remotes", "--contains", head)
	if err != nil {
		return false, nil // no remote-tracking ref; treat as unpublished
	}
	return strings.Contains(out, remote+"/"), nil
}

// UndoLastCommit removes the commit but keeps every change in the working
// tree. Nothing you wrote is thrown away - only the commit that wrapped it.
func (e *Engine) UndoLastCommit(r *Repo, wasPushed bool) error {
	lock, err := LockRepo(r.Name, 30*time.Second)
	if err != nil {
		return err
	}
	defer lock.Release()

	branch, err := e.prepare(r)
	if err != nil {
		return err
	}
	subject, err := LastCommitSubject(r.Path)
	if err != nil {
		return err
	}

	safety := e.backup(r, branch)
	if _, err := git(r.Path, "reset", "--soft", "HEAD~1"); err != nil {
		return fmt.Errorf("could not take the commit back: %w", err)
	}
	// --soft leaves the changes staged; unstage them so the working tree
	// looks the way it did before syn committed.
	if _, err := git(r.Path, "reset"); err != nil {
		return fmt.Errorf("could not unstage: %w", err)
	}

	Info("%s: took back %q - the changes are still in %s",
		r.Name, subject, filepath.Base(r.Path))
	if safety != "" {
		Info("%s: the commit is still on %s if you want it back", r.Name, safety)
	}

	if !wasPushed {
		return nil
	}
	Warn("%s: it was already on %s - publish the correction with:", r.Name, r.Remote)
	Warn("        syn push %s --force", r.Name)
	return nil
}
