package main

import (
	"fmt"
	"sort"
	"strings"
)

// Help for one command.
//
// `syn help` prints all of it, which is a hundred and ninety lines and the
// right thing when you want to browse. When you want to know what `pull`
// takes it is the wrong thing, and `syn pull --help` used to complain that no
// repository was registered, because it read --help as a flag to pull.
//
// The text is not written a second time. It is lifted out of the long usage
// above - the paragraph that starts with the command and the lines indented
// under it - so the two cannot disagree. What this adds is the catalogue's
// one-line description and the subcommands.

// commandHelp builds the help for one command. The second result is false if
// there is no such command.
func commandHelp(name string) (string, bool) {
	info, ok := catalog[name]
	if !ok {
		return "", false
	}

	var b strings.Builder
	fmt.Fprintf(&b, "syn %s - %s\n\n", name, info.Desc)

	if info.Args != "" {
		fmt.Fprintf(&b, "USAGE\n  syn %s %s\n\n", name, info.Args)
	} else {
		fmt.Fprintf(&b, "USAGE\n  syn %s\n\n", name)
	}

	if sub := subcommandsOf(name); len(sub) > 0 {
		b.WriteString("SUBCOMMANDS\n")
		width := 0
		for _, s := range sub {
			if len(s.name) > width {
				width = len(s.name)
			}
		}
		for _, s := range sub {
			fmt.Fprintf(&b, "  %-*s  %s\n", width, s.name, s.desc)
		}
		b.WriteString("\n")
	}

	if detail := usageSection(usage, name); detail != "" {
		b.WriteString("DETAIL\n")
		b.WriteString(detail)
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "Everything else:  syn help\n")
	return b.String(), true
}

type subInfo struct{ name, desc string }

// subcommandsOf are the catalogue entries of the form "<name> <word>".
func subcommandsOf(name string) []subInfo {
	var out []subInfo
	for key, info := range subHints {
		if rest, ok := strings.CutPrefix(key, name+" "); ok {
			label := rest
			if info.Args != "" {
				label += " " + info.Args
			}
			out = append(out, subInfo{label, info.Desc})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// usageSection returns the lines of the long help that belong to a command:
// each paragraph whose first word is the command, with the lines indented
// beneath it. A command can appear more than once - pull has a plain form and
// a --to form - and all of them come back.
func usageSection(text, name string) string {
	var out []string
	inside := false
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.TrimSpace(line) == "":
			inside = false
		case strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   "):
			// "  word ..." starts an entry; deeper indentation continues one.
			first := strings.Fields(line)[0]
			inside = first == name
			if inside {
				out = append(out, line)
			}
		case inside && strings.HasPrefix(line, "    "):
			out = append(out, line)
		default:
			inside = false
		}
	}
	return strings.Join(out, "\n") + "\n"
}

// suggest names the commands that look like what was typed, so a slip says
// what was probably meant instead of only that it was wrong.
func suggest(typed string) []string {
	var close []string
	for _, name := range knownCommands() {
		if name == typed {
			continue
		}
		if strings.HasPrefix(name, typed) || strings.HasPrefix(typed, name) ||
			editDistance(name, typed) <= 2 {
			close = append(close, name)
		}
	}
	// Nearest first: for "stauts" the useful answer is status, not start.
	sort.SliceStable(close, func(i, j int) bool {
		return editDistance(close[i], typed) < editDistance(close[j], typed)
	})
	if len(close) > 3 {
		close = close[:3]
	}
	return close
}

// editDistance counts the edits between two words, where swapping two
// neighbouring letters is one edit rather than two. That is the shape of most
// typing slips - "psuh", "stauts" - and under plain Levenshtein "stauts" is as
// far from "start" as it is from "status", so the right answer would tie with
// a wrong one.
func editDistance(a, b string) int {
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
		d[i][0] = i
	}
	for j := 0; j <= len(b); j++ {
		d[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(a)][len(b)]
}

// unknownCommand is the error for a command that does not exist, with what
// it was probably meant to be.
func unknownCommand(typed string) error {
	if close := suggest(typed); len(close) > 0 {
		return fmt.Errorf("unknown command %q - did you mean: %s?\n        all of them:  syn help",
			typed, strings.Join(close, ", "))
	}
	return fmt.Errorf("unknown command %q - run `syn help`", typed)
}
