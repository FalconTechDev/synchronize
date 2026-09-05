package syn

import (
	"fmt"
	"strings"
)

// CommitCheck is what GitHub says about one commit: whether its checks have
// finished, and whether they passed.
type CommitCheck struct {
	State   string // success, pending, failure, or "" when nothing reported
	Total   int
	Failing []string
}

func (c CommitCheck) OK() bool { return c.State == "success" || c.Total == 0 }

// CommitStatus asks GitHub how a commit fared. It reads both of the two
// systems GitHub has for this - the older commit statuses, which is what
// most external CI still posts to, and check runs, which is what Actions
// uses - because a repository can be using either and a gate that only
// knows one of them would wave the other through.
func (g *GitHub) CommitStatus(slug, sha string) (CommitCheck, error) {
	var check CommitCheck

	var combined struct {
		State    string `json:"state"`
		Statuses []struct {
			State   string `json:"state"`
			Context string `json:"context"`
		} `json:"statuses"`
	}
	if err := g.get("/repos/"+slug+"/commits/"+sha+"/status", &combined); err != nil {
		return check, err
	}
	check.Total = len(combined.Statuses)
	if check.Total > 0 {
		check.State = combined.State
		for _, s := range combined.Statuses {
			if s.State != "success" {
				check.Failing = append(check.Failing, s.Context)
			}
		}
	}

	var runs struct {
		Total int `json:"total_count"`
		Runs  []struct {
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		} `json:"check_runs"`
	}
	if err := g.get("/repos/"+slug+"/commits/"+sha+"/check-runs", &runs); err != nil {
		// The older half already answered; do not fail the whole gate
		// because this half is unavailable.
		if check.Total > 0 {
			return check, nil
		}
		return check, err
	}

	pending, failed := false, false
	for _, run := range runs.Runs {
		check.Total++
		if run.Status != "completed" {
			pending = true
			check.Failing = append(check.Failing, run.Name+" (still running)")
			continue
		}
		switch run.Conclusion {
		case "success", "neutral", "skipped":
		default:
			failed = true
			check.Failing = append(check.Failing, run.Name+" ("+run.Conclusion+")")
		}
	}
	// A failure outranks something still running: waiting will not save it.
	switch {
	case failed:
		check.State = "failure"
	case pending:
		check.State = "pending"
	case check.Total > 0 && check.State == "":
		check.State = "success"
	}
	return check, nil
}

// checkGate stops a pull when the commit it would bring in has not passed
// its checks. It is off unless the repository asks for it, and only means
// anything for a GitHub repository, since it is GitHub being asked.
func (e *Engine) checkGate(r *Repo, sha string) error {
	if !r.RequireStatusCheck || sha == "" || e.SkipChecks {
		return nil
	}
	if r.GithubSlug == "" {
		return fmt.Errorf("status checks are required, but %s is not a GitHub repository\n"+
			"        turn the requirement off:  syn set %s require_checks off", r.Name, r.Name)
	}
	token := e.Creds.Token("github.com")
	if token == "" {
		return fmt.Errorf("status checks are required, but nobody is signed in to GitHub\n" +
			"        sign in:  syn auth login")
	}

	check, err := NewGitHub(token).CommitStatus(r.GithubSlug, sha)
	if err != nil {
		return fmt.Errorf("status checks are required, but GitHub could not be asked: %w\n"+
			"        the pull was not made - nothing has changed", err)
	}
	if check.OK() {
		if check.Total > 0 {
			Debug("%s: %s passed %d check(s)", r.Name, short(sha), check.Total)
		}
		return nil
	}
	return fmt.Errorf("%s has not passed its checks (%s): %s\n"+
		"        nothing was pulled. once CI is green it will go through on its own,\n"+
		"        or take it anyway with:  syn pull %s --to %s --no-checks",
		short(sha), check.State, strings.Join(check.Failing, ", "), r.Name, short(sha))
}
