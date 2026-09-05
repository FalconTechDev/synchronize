package syn

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeGitHub answers the two endpoints the gate asks about.
func fakeGitHub(t *testing.T, statuses, checkRuns string) *GitHub {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/status"):
			w.Write([]byte(statuses))
		case strings.HasSuffix(r.URL.Path, "/check-runs"):
			w.Write([]byte(checkRuns))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return &GitHub{Client: srv.Client(), Base: srv.URL}
}

const noStatuses = `{"state":"pending","statuses":[]}`
const noRuns = `{"total_count":0,"check_runs":[]}`

func TestCommitStatusReadsBothSystems(t *testing.T) {
	for _, tc := range []struct {
		name      string
		statuses  string
		runs      string
		wantState string
		wantOK    bool
	}{
		{
			name:      "nothing reported at all is not a failure",
			statuses:  noStatuses,
			runs:      noRuns,
			wantState: "",
			wantOK:    true,
		},
		{
			name:      "an old-style status alone can pass",
			statuses:  `{"state":"success","statuses":[{"state":"success","context":"ci/travis"}]}`,
			runs:      noRuns,
			wantState: "success",
			wantOK:    true,
		},
		{
			name:      "an old-style status alone can fail",
			statuses:  `{"state":"failure","statuses":[{"state":"failure","context":"ci/travis"}]}`,
			runs:      noRuns,
			wantState: "failure",
			wantOK:    false,
		},
		{
			name:      "a check run alone can pass",
			statuses:  noStatuses,
			runs:      `{"total_count":1,"check_runs":[{"name":"build","status":"completed","conclusion":"success"}]}`,
			wantState: "success",
			wantOK:    true,
		},
		{
			name:      "a check run still going is not green yet",
			statuses:  noStatuses,
			runs:      `{"total_count":1,"check_runs":[{"name":"build","status":"in_progress"}]}`,
			wantState: "pending",
			wantOK:    false,
		},
		{
			name:      "a skipped job does not count against it",
			statuses:  noStatuses,
			runs:      `{"total_count":2,"check_runs":[{"name":"build","status":"completed","conclusion":"success"},{"name":"deploy","status":"completed","conclusion":"skipped"}]}`,
			wantState: "success",
			wantOK:    true,
		},
		{
			name:      "green in one system does not excuse red in the other",
			statuses:  `{"state":"success","statuses":[{"state":"success","context":"ci/travis"}]}`,
			runs:      `{"total_count":1,"check_runs":[{"name":"build","status":"completed","conclusion":"failure"}]}`,
			wantState: "failure",
			wantOK:    false,
		},
		{
			name:      "a failure outranks something still running",
			statuses:  noStatuses,
			runs:      `{"total_count":2,"check_runs":[{"name":"a","status":"in_progress"},{"name":"b","status":"completed","conclusion":"failure"}]}`,
			wantState: "failure",
			wantOK:    false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			check, err := fakeGitHub(t, tc.statuses, tc.runs).CommitStatus("o/r", "abc123")
			if err != nil {
				t.Fatal(err)
			}
			if check.State != tc.wantState {
				t.Errorf("State = %q, want %q", check.State, tc.wantState)
			}
			if check.OK() != tc.wantOK {
				t.Errorf("OK() = %v, want %v (failing: %v)", check.OK(), tc.wantOK, check.Failing)
			}
		})
	}
}

func TestCheckGateIsOffUnlessAsked(t *testing.T) {
	e := &Engine{Creds: &Credentials{}}
	r := &Repo{Name: "bot"}
	if err := e.checkGate(r, "abc"); err != nil {
		t.Errorf("a repository that never asked for checks was gated: %v", err)
	}
}

func TestCheckGateRefusesRatherThanGuess(t *testing.T) {
	e := &Engine{Creds: &Credentials{}}

	// Asked for, but there is no GitHub to ask.
	r := &Repo{Name: "bot", RequireStatusCheck: true}
	err := e.checkGate(r, "abc")
	if err == nil || !strings.Contains(err.Error(), "not a GitHub repository") {
		t.Errorf("a non-GitHub repository requiring checks should say so, got %v", err)
	}

	// A GitHub repository, but nobody signed in: a gate that cannot check
	// has to stop, not wave things through.
	r = &Repo{Name: "bot", RequireStatusCheck: true, GithubSlug: "o/r"}
	err = e.checkGate(r, "abc")
	if err == nil || !strings.Contains(err.Error(), "auth login") {
		t.Errorf("with no token the gate should refuse and say how to fix it, got %v", err)
	}
}

func TestSkipChecksOverridesTheGate(t *testing.T) {
	e := &Engine{Creds: &Credentials{}, SkipChecks: true}
	r := &Repo{Name: "bot", RequireStatusCheck: true, GithubSlug: "o/r"}
	if err := e.checkGate(r, "abc"); err != nil {
		t.Errorf("--no-checks should get past the gate: %v", err)
	}
}
