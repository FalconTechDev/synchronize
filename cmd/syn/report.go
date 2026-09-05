package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/FalconTechDev/synchronize/internal/syn"
)

// StatusRow is what --json prints. The web dashboard already answered this
// question; there was no way to ask it without starting an HTTP server.
type StatusRow struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Slug     string `json:"slug,omitempty"`
	Branch   string `json:"branch"`
	Tracking string `json:"tracking,omitempty"`
	Ahead    int    `json:"ahead"`
	Behind   int    `json:"behind"`
	Dirty    bool   `json:"dirty"`
	Changed  int    `json:"changed"`
	Auto     bool   `json:"auto"`
	Conflict string `json:"conflict,omitempty"`
	LastSync string `json:"last_sync,omitempty"`
	Error    string `json:"error,omitempty"`
}

func statusRows(e *syn.Engine, repos []*syn.Repo, fetch bool) []StatusRow {
	rows := make([]StatusRow, 0, len(repos))
	for _, r := range repos {
		st := e.Status(r, fetch)
		row := StatusRow{
			Name:    r.Name,
			Path:    r.Path,
			Slug:    r.GithubSlug,
			Branch:  st.Branch,
			Ahead:   st.Ahead,
			Behind:  st.Behind,
			Dirty:   st.Dirty,
			Changed: len(st.Changed),
			Auto:    r.Enabled,
		}
		if st.Branch != "" && r.Remote != "" {
			row.Tracking = r.Remote + "/" + st.Branch
		}
		if !st.LastSync.IsZero() {
			row.LastSync = st.LastSync.UTC().Format("2006-01-02T15:04:05Z")
		}
		if st.Conflict != nil {
			row.Conflict = st.Conflict.Reason
		}
		if st.Err != nil {
			row.Error = st.Err.Error()
		}
		rows = append(rows, row)
	}
	return rows
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// printPorcelain gives one tab-separated line per repository, for the times
// a shell loop is easier to write than a JSON parser.
func printPorcelain(rows []StatusRow) {
	for _, r := range rows {
		state := "ok"
		switch {
		case r.Error != "":
			state = "error"
		case r.Conflict != "":
			state = "conflict"
		case r.Dirty:
			state = "dirty"
		case r.Behind > 0:
			state = "behind"
		case r.Ahead > 0:
			state = "ahead"
		}
		fmt.Printf("%s\t%s\t%d\t%d\t%d\t%s\t%s\n",
			r.Name, state, r.Ahead, r.Behind, r.Changed, r.Branch, r.Path)
	}
}

// OpReport is what a pull, push or sync prints under --json.
type OpReport struct {
	Repo      string   `json:"repo"`
	OK        bool     `json:"ok"`
	Summary   string   `json:"summary"`
	Committed bool     `json:"committed"`
	Pulled    int      `json:"pulled"`
	Pushed    int      `json:"pushed"`
	Files     []string `json:"files,omitempty"`
	Conflict  string   `json:"conflict,omitempty"`
	Error     string   `json:"error,omitempty"`
}

func opReports(results []syn.Result) []OpReport {
	out := make([]OpReport, 0, len(results))
	for _, res := range results {
		rep := OpReport{
			Repo:      res.Repo,
			OK:        res.OK(),
			Summary:   res.Summary(),
			Committed: res.Committed,
			Pulled:    res.Pulled,
			Pushed:    res.Pushed,
		}
		for _, f := range res.CommittedFiles {
			rep.Files = append(rep.Files, f.Path)
		}
		for _, f := range res.PulledFiles {
			rep.Files = append(rep.Files, f.Path)
		}
		if res.Conflict != nil {
			rep.Conflict = res.Conflict.Reason
		}
		if res.Err != nil {
			rep.Error = res.Err.Error()
		}
		out = append(out, rep)
	}
	return out
}
