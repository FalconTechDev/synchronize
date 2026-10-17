package main

import (
	"strings"
	"testing"

	"github.com/FalconTechDev/synchronize/internal/syn"
)

// Three thousand lines of argument handling had no tests at all. Every fault
// found in it so far - a --paths value swallowed by the flag parser, a `--`
// left in the command it was separating, an exit code that said success
// while reporting a failure - was found by running the thing by hand.

func TestParseMinutes(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"30", 30, false},
		{"30m", 30, false},
		{"90m", 90, false},
		{"2h", 120, false},
		{"12h", 720, false},
		{"1d", 1440, false},
		{"0", 0, false}, // means "use the global setting"
		{"", 0, true},
		{"abc", 0, true},
		{"-5", 0, true},
		{"5x", 0, true},
		{"1.5h", 90, false}, // a fraction of an hour is a perfectly good interval
	} {
		got, err := parseMinutes(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseMinutes(%q) = %d, wanted an error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseMinutes(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseMinutes(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseSwitch(t *testing.T) {
	for in, want := range map[string]struct{ on, ok bool }{
		"on":    {true, true},
		"true":  {true, true},
		"yes":   {true, true},
		"off":   {false, true},
		"false": {false, true},
		"no":    {false, true},
		"maybe": {false, false},
		"":      {false, false},
	} {
		on, ok := parseSwitch(in)
		if on != want.on || ok != want.ok {
			t.Errorf("parseSwitch(%q) = (%v, %v), want (%v, %v)", in, on, ok, want.on, want.ok)
		}
	}
}

// The value of --paths is a comma-separated list; a trailing comma is a typo,
// not a pattern that matches nothing.
func TestSplitPaths(t *testing.T) {
	for in, want := range map[string][]string{
		"":                  nil,
		"src/**":            {"src/**"},
		"src/**,go.mod":     {"src/**", "go.mod"},
		" src/** , go.mod ": {"src/**", "go.mod"},
		"src/**,,go.mod":    {"src/**", "go.mod"},
		"src/**,":           {"src/**"},
		",":                 nil,
		"имя.txt,док/**":    {"имя.txt", "док/**"},
	} {
		got := splitPaths(in)
		if len(got) != len(want) {
			t.Errorf("splitPaths(%q) = %v, want %v", in, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("splitPaths(%q) = %v, want %v", in, got, want)
				break
			}
		}
	}
}

func TestLooksLikeURL(t *testing.T) {
	for in, want := range map[string]bool{
		"https://github.com/a/b.git": true,
		"http://example.com/a.git":   true,
		"ssh://git@host/a.git":       true,
		"git://host/a.git":           true,
		"file:///tmp/a.git":          true,
		"owner/repo":                 false,
		"repo":                       false,
		"":                           false,
		// Not a URL scheme we clone from, so it must not be treated as one.
		"javascript:alert(1)": false,
	} {
		if got := looksLikeURL(in); got != want {
			t.Errorf("looksLikeURL(%q) = %v, want %v", in, got, want)
		}
	}
}

// Adding a second copy of a repository must not produce a name with a slash
// in it, and must not loop forever looking for a free one - both of which it
// did at one point.
func TestUniqueName(t *testing.T) {
	cfg := syn.DefaultConfig()
	cfg.Repos = []syn.Repo{{Name: "bot"}, {Name: "bot-2"}, {Name: "bot-3"}}

	got := uniqueName(cfg, "bot")
	if got == "bot" || got == "bot-2" || got == "bot-3" {
		t.Errorf("uniqueName returned a name already taken: %q", got)
	}
	if strings.ContainsAny(got, `/\`) {
		t.Errorf("a repository name must not contain a path separator: %q", got)
	}
	if free := uniqueName(cfg, "other"); free != "other" {
		t.Errorf("a free name should be used as-is, got %q", free)
	}
}

// The exit code is what a script reads. Being ahead or behind is the normal
// state between syncs and must not be reported as a failure.
func TestStatusExit(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rows    []StatusRow
		wantErr bool
		mention string
	}{
		{"everything fine", []StatusRow{{Name: "a"}}, false, ""},
		{"behind is not a fault", []StatusRow{{Name: "a", Behind: 3}}, false, ""},
		{"ahead is not a fault", []StatusRow{{Name: "a", Ahead: 2, Dirty: true, Changed: 5}}, false, ""},
		{"a broken repository", []StatusRow{{Name: "a", Error: "gone"}}, true, "a"},
		{"a conflict", []StatusRow{{Name: "a", Conflict: "merge"}}, true, "resolve"},
		{"both at once", []StatusRow{{Name: "a", Error: "gone"}, {Name: "b", Conflict: "merge"}}, true, "b"},
		{"nothing tracked", nil, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := statusExit(tc.rows)
			if tc.wantErr != (err != nil) {
				t.Fatalf("statusExit = %v, wanted an error: %v", err, tc.wantErr)
			}
			if err != nil && tc.mention != "" && !strings.Contains(err.Error(), tc.mention) {
				t.Errorf("the message should mention %q: %v", tc.mention, err)
			}
		})
	}
}

func TestHumanMinutes(t *testing.T) {
	for in, want := range map[int]string{
		1:    "1 minute",
		10:   "10 minutes",
		60:   "1 hour",
		90:   "90 minutes",
		120:  "2 hours",
		1440: "1 day",
	} {
		if got := humanMinutes(in); got != want {
			t.Errorf("humanMinutes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestAppendAndRemovePattern(t *testing.T) {
	s := appendUniq(nil, "a")
	s = appendUniq(s, "b")
	s = appendUniq(s, "a") // already there
	if len(s) != 2 {
		t.Errorf("appendUniq let a duplicate in: %v", s)
	}
	s = removePattern(s, "a")
	if len(s) != 1 || s[0] != "b" {
		t.Errorf("removePattern = %v, want [b]", s)
	}
	// Removing something that is not there changes nothing.
	if got := removePattern(s, "zzz"); len(got) != 1 {
		t.Errorf("removePattern dropped something it should not have: %v", got)
	}
}

// Every command the dispatcher accepts should appear in the catalogue, or
// the shell offers no hint for it and `syn help` does not list it. This is
// the check that would have caught `restart` and `app` shipping without one.
func TestEveryCommandHasAHint(t *testing.T) {
	for _, name := range knownCommands() {
		if hint := hintFor(name); strings.TrimSpace(hint) == "" {
			t.Errorf("%q has no hint in the catalogue", name)
		}
	}
}

// needsGit decides whether to check for git before running. Commands that
// only read configuration must not refuse to run on a machine without it.
func TestNeedsGit(t *testing.T) {
	for cmd, want := range map[string]bool{
		"pull":    true,
		"push":    true,
		"sync":    true,
		"status":  true,
		"help":    false,
		"version": false,
		"config":  false,
		"log":     false,
		"auth":    false,
	} {
		if got := needsGit(cmd); got != want {
			t.Errorf("needsGit(%q) = %v, want %v", cmd, got, want)
		}
	}
}

func TestTruthy(t *testing.T) {
	for _, yes := range []string{"true", "yes", "on", "1"} {
		if !truthy(yes) {
			t.Errorf("truthy(%q) should be true", yes)
		}
	}
	for _, no := range []string{"false", "no", "off", "0", "", "maybe"} {
		if truthy(no) {
			t.Errorf("truthy(%q) should be false", no)
		}
	}
}
