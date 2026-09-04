package syn

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type RepoStatus struct {
	Repo      *Repo
	Branch    string
	Dirty     bool
	Changed   []string
	Ahead     int
	Behind    int
	InProgres string
	Conflict  *Conflict
	LastSync  time.Time
	Err       error
}

func (e *Engine) Status(r *Repo, fetch bool) RepoStatus {
	st := RepoStatus{Repo: r}
	if c, ok := e.State.Conflicts[r.Name]; ok {
		st.Conflict = &c
	}
	st.LastSync = e.State.LastSync[r.Name]
	if !IsRepo(r.Path) {
		st.Err = notAWorkingTree(r)
		return st
	}
	st.InProgres = InProgress(r.Path)
	branch, err := CurrentBranch(r.Path)
	if err != nil {
		st.Err = err
		return st
	}
	st.Branch = branch
	if fetch {
		if _, err := git(r.Path, "fetch", "--prune", r.Remote); err != nil {
			st.Err = err
			return st
		}
	}
	if files, err := ChangedFiles(r.Path); err == nil {
		st.Changed = files
		st.Dirty = len(files) > 0
	}
	st.Ahead, st.Behind, _ = AheadBehind(r.Path, branch, r.Remote)
	return st
}

func (s RepoStatus) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  (%s)\n", s.Repo.Name, s.Repo.Path)
	if s.Err != nil {
		fmt.Fprintf(&b, "  error:    %v\n", s.Err)
		return b.String()
	}
	fmt.Fprintf(&b, "  branch:   %s -> %s/%s\n", s.Branch, s.Repo.Remote, orDefault(s.Repo.Branch, s.Branch))
	state := "clean"
	if s.Dirty {
		state = fmt.Sprintf("%d uncommitted change(s)", len(s.Changed))
	}
	fmt.Fprintf(&b, "  local:    %s\n", state)
	fmt.Fprintf(&b, "  vs remote: %d ahead, %d behind\n", s.Ahead, s.Behind)
	if s.InProgres != "" {
		fmt.Fprintf(&b, "  warning:  a %s is in progress\n", s.InProgres)
	}
	if s.Conflict != nil {
		fmt.Fprintf(&b, "  CONFLICT: %s (%s)\n", s.Conflict.Reason, s.Conflict.At.Format("2006-01-02 15:04"))
		fmt.Fprintf(&b, "            files:  %s\n", strings.Join(s.Conflict.Files, ", "))
		if s.Conflict.BackupBranch != "" {
			fmt.Fprintf(&b, "            backup: %s\n", s.Conflict.BackupBranch)
		}
		fmt.Fprintf(&b, "            fix it, then run: syn resolve %s\n", s.Repo.Name)
	}
	if !s.LastSync.IsZero() {
		fmt.Fprintf(&b, "  last sync: %s (%s)\n", s.LastSync.Format("2006-01-02 15:04:05"), timeAgo(s.LastSync))
	}
	if !s.Repo.Enabled {
		fmt.Fprintf(&b, "  disabled: not touched by --all\n")
	}
	return b.String()
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 10*time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%d sec ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}

// notAWorkingTree explains the two ways this normally happens - the folder
// moved or its .git was deleted - and what to do about each.
func notAWorkingTree(r *Repo) error {
	if _, err := os.Stat(r.Path); os.IsNotExist(err) {
		return fmt.Errorf("%s no longer exists\n"+
			"        it may be on a drive that is not plugged in.\n"+
			"        if it is gone for good:  syn remove %s", r.Path, r.Name)
	}
	slug := r.GithubSlug
	if slug == "" {
		slug = "<owner>/<name>"
	}
	return fmt.Errorf("%s is no longer a git repository (its .git is missing)\n"+
		"        clone it again:  syn repo add %s --path %s\n"+
		"        or stop tracking it:  syn remove %s", r.Path, slug, r.Path, r.Name)
}
