package main

import (
	"strings"
	"testing"
)

// Asking how a command works must not depend on having set anything up.
func TestCommandHelpNeedsNoRepositories(t *testing.T) {
	t.Setenv("SYNCHRONIZE_HOME", t.TempDir())
	for _, args := range [][]string{
		{"pull", "--help"}, {"push", "-h"}, {"help", "hook"}, {"hook", "--help"},
	} {
		if err := run(args); err != nil {
			t.Errorf("syn %s: %v", strings.Join(args, " "), err)
		}
	}
}

func TestCommandHelpSaysWhatTheCommandTakes(t *testing.T) {
	text, ok := commandHelp("pull")
	if !ok {
		t.Fatal("pull has no help")
	}
	for _, want := range []string{"syn pull", "USAGE", "--force", "--to <ref>"} {
		if !strings.Contains(text, want) {
			t.Errorf("the help for pull does not mention %q:\n%s", want, text)
		}
	}
	// It is help for pull, not the whole manual.
	if strings.Contains(text, "GITHUB AUTH") || strings.Contains(text, "auth login") {
		t.Errorf("one command's help turned into the full help:\n%s", text)
	}
}

func TestCommandHelpListsSubcommands(t *testing.T) {
	text, ok := commandHelp("hook")
	if !ok {
		t.Fatal("hook has no help")
	}
	for _, want := range []string{"SUBCOMMANDS", "rollback", "timeout", "--paths"} {
		if !strings.Contains(text, want) {
			t.Errorf("the help for hook does not mention %q", want)
		}
	}
}

func TestEveryCommandHasHelp(t *testing.T) {
	for _, name := range knownCommands() {
		text, ok := commandHelp(name)
		if !ok || !strings.HasPrefix(text, "syn "+name) {
			t.Errorf("%q has no usable help: ok=%v %q", name, ok, text)
		}
	}
}

func TestUnknownCommandHasNoHelp(t *testing.T) {
	if _, ok := commandHelp("frobnicate"); ok {
		t.Error("made up help for a command that does not exist")
	}
}

func TestUsageSectionTakesEveryFormOfACommand(t *testing.T) {
	const sample = "" +
		"  pull  <repo|all>   bring changes in\n" +
		"  pull  <repo> --to <ref>   go to one ref\n" +
		"                      and stay there\n" +
		"  push  <repo|all>   publish\n" +
		"\n" +
		"  pulley  not this one\n"
	got := usageSection(sample, "pull")
	for _, want := range []string{"bring changes in", "go to one ref", "and stay there"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, bad := range []string{"publish", "not this one"} {
		if strings.Contains(got, bad) {
			t.Errorf("%q belongs to another command:\n%s", bad, got)
		}
	}
}

func TestSuggestionsPutTheNearestFirst(t *testing.T) {
	for typed, want := range map[string]string{
		"psuh":   "push",
		"stauts": "status",
		"pul":    "pull",
		"hlep":   "help",
	} {
		got := suggest(typed)
		if len(got) == 0 || got[0] != want {
			t.Errorf("suggest(%q) = %v, want %q first", typed, got, want)
		}
	}
	if got := suggest("zzzzzzzz"); len(got) != 0 {
		t.Errorf("suggested %v for something unrelated", got)
	}
}

func TestEditDistance(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"", "", 0}, {"push", "push", 0},
		// a swap of neighbours is one slip, not two
		{"push", "psuh", 1}, {"status", "stauts", 1}, {"abc", "", 3}, {"kitten", "sitting", 3},
	} {
		if got := editDistance(tc.a, tc.b); got != tc.want {
			t.Errorf("editDistance(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
