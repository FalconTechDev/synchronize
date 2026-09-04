package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"golang.org/x/term"
)

type editor struct {
	history []string
	hpos    int
	fd      int
	raw     bool
	plain   *bufio.Scanner
}

func newEditor() *editor {
	fd := int(os.Stdin.Fd())
	return &editor{fd: fd, raw: term.IsTerminal(fd)}
}

// Read returns the next line. ok is false when the user asked to leave
// (Ctrl+C, Ctrl+D or end of input).
func (e *editor) Read(prompt string, complete func(string) ([]string, string)) (string, bool) {
	if !e.raw {
		return e.readPlain(prompt)
	}

	state, err := term.MakeRaw(e.fd)
	if err != nil {
		e.raw = false
		return e.readPlain(prompt)
	}
	defer term.Restore(e.fd, state)

	var line []rune
	pos := 0
	e.hpos = len(e.history)
	hintShown := false
	// Set by Tab to show the candidates for one keystroke only, so the list
	// never ends up in the scrollback.
	pending := ""

	redraw := func() {
		fmt.Print("\r\033[K" + prompt + string(line))
		if pos < len(line) {
			fmt.Printf("\033[%dD", len(line)-pos)
		}
	}

	clearHint := func() {
		if hintShown {
			fmt.Print("\0337\n\033[K\0338")
			hintShown = false
		}
	}

	showHint := func() {
		clearHint()
		hint := pending
		pending = ""
		if hint == "" {
			_, hint = complete(string(line[:pos]))
		}
		if hint == "" {
			return
		}
		fmt.Print("\0337\n\033[K" + cDim + hint + cReset + "\0338")
		hintShown = true
	}

	redraw()
	showHint()

	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			clearHint()
			fmt.Print("\r\n")
			return "", false
		}

		switch b := buf[0]; b {
		case 3, 4: // Ctrl+C, Ctrl+D
			clearHint()
			fmt.Print("\r\n")
			return "", false

		case 13, 10: // Enter
			clearHint()
			fmt.Print("\r\n")
			s := strings.TrimSpace(string(line))
			if s != "" {
				e.history = append(e.history, s)
			}
			return s, true

		case 9: // Tab
			options, _ := complete(string(line[:pos]))
			if len(options) == 1 {
				_, start := lastWord(string(line[:pos]))
				rest := []rune(options[0] + " ")
				line = append([]rune(string(line[:start])), append(rest, line[pos:]...)...)
				pos = start + len(rest)
				redraw()
				showHint()
			} else if len(options) > 1 {
				pending = fit(strings.Join(options, "   "))
				showHint()
			}

		case 127, 8: // Backspace
			if pos > 0 {
				line = append(line[:pos-1], line[pos:]...)
				pos--
				redraw()
				showHint()
			}

		case 27: // escape sequence
			seq := make([]byte, 2)
			if n, _ := os.Stdin.Read(seq); n < 2 {
				continue
			}
			if seq[0] != '[' {
				continue
			}
			switch seq[1] {
			case 'A': // up
				if e.hpos > 0 {
					e.hpos--
					line = []rune(e.history[e.hpos])
					pos = len(line)
					redraw()
					showHint()
				}
			case 'B': // down
				if e.hpos < len(e.history)-1 {
					e.hpos++
					line = []rune(e.history[e.hpos])
				} else {
					e.hpos = len(e.history)
					line = nil
				}
				pos = len(line)
				redraw()
				showHint()
			case 'C': // right
				if pos < len(line) {
					pos++
					redraw()
				}
			case 'D': // left
				if pos > 0 {
					pos--
					redraw()
				}
			case 'H':
				pos = 0
				redraw()
			case 'F':
				pos = len(line)
				redraw()
			}

		default:
			if b < 32 {
				continue
			}
			r := rune(b)
			if b >= 0x80 {
				// finish the UTF-8 sequence so non-ASCII input survives
				extra := 0
				switch {
				case b>>5 == 0b110:
					extra = 1
				case b>>4 == 0b1110:
					extra = 2
				case b>>3 == 0b11110:
					extra = 3
				}
				seq := append([]byte{b}, make([]byte, extra)...)
				if extra > 0 {
					os.Stdin.Read(seq[1:])
				}
				rs := []rune(string(seq))
				if len(rs) == 0 {
					continue
				}
				r = rs[0]
			}
			line = append(line[:pos], append([]rune{r}, line[pos:]...)...)
			pos++
			redraw()
			showHint()
		}
	}
}

func (e *editor) readPlain(prompt string) (string, bool) {
	if e.plain == nil {
		e.plain = bufio.NewScanner(os.Stdin)
		e.plain.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	}
	fmt.Print(prompt)
	if !e.plain.Scan() {
		return "", false
	}
	return strings.TrimSpace(e.plain.Text()), true
}

func lastWord(s string) (string, int) {
	i := strings.LastIndexAny(s, " \t")
	return s[i+1:], i + 1
}

// completeLine works out which word the cursor sits on and offers whatever
// belongs there: a command, a subcommand, a repository, or nothing but a
// reminder of what the command still expects.
func completeLine(line string, commands, repos []string) ([]string, string) {
	word, _ := lastWord(line)
	fields := strings.Fields(line)

	idx := len(fields)
	if !strings.HasSuffix(line, " ") && idx > 0 {
		idx--
	}

	if idx == 0 {
		options := matching(commands, word)
		switch {
		case word == "":
			return options, ""
		case len(options) == 0:
			return nil, "no command starts with " + word
		case len(options) == 1 && options[0] == word:
			return options, hintFor(word)
		default:
			return options, fit("commands: " + strings.Join(options, "   "))
		}
	}

	cmd := fields[0]
	subs, hasSubs := subcommands[cmd]

	if hasSubs && idx == 1 {
		options := matching(subs, word)
		switch {
		case len(options) == 0:
			return nil, fit(cmd + " takes: " + strings.Join(subs, "   "))
		case len(options) == 1 && options[0] == word:
			return options, hintFor(cmd + " " + word)
		default:
			return options, fit(cmd + ": " + strings.Join(options, "   "))
		}
	}

	key := cmd
	if hasSubs && len(fields) > 1 {
		key = cmd + " " + fields[1]
	}

	if at, ok := repoWord[key]; ok && idx == at {
		if len(repos) == 0 {
			return nil, "no repositories registered yet - try: repo add owner/name"
		}
		pool := append([]string{"all"}, repos...)
		options := matching(pool, word)
		if word != "" && len(options) == 0 {
			return nil, "no repository matches " + word
		}
		return options, fit("repos: " + strings.Join(pool, "   "))
	}

	if events, ok := eventWord[key]; ok && idx == eventPos[key] {
		options := matching(events, word)
		if len(options) == 0 {
			return nil, fit("event: " + strings.Join(events, "   "))
		}
		return options, fit("event: " + strings.Join(events, "   "))
	}

	return nil, hintFor(key)
}

// repoWord says which word of a command names a repository, and eventWord
// which one names a hook event.
var repoWord = map[string]int{
	"pull": 1, "push": 1, "sync": 1, "status": 1, "diff": 1, "watch": 1,
	"remove": 1, "enable": 1, "disable": 1, "set": 1, "resolve": 1,
	"backups": 1, "restore": 1, "prune": 1,
	"files include": 2, "files exclude": 2, "files clear": 2,
	"hook add": 2, "hook run": 2, "hook clear": 2, "hook timeout": 2,
}

var hookEvents = []string{"after-pull", "after-push", "on-conflict"}

var eventWord = map[string][]string{
	"hook add": hookEvents, "hook run": hookEvents, "hook clear": hookEvents,
}

var eventPos = map[string]int{"hook add": 3, "hook run": 3, "hook clear": 3}

// subcommands drive the second word of the commands that have one.
var subcommands = map[string][]string{
	"auth":   {"login", "logout", "status", "setup"},
	"repo":   {"add", "ls"},
	"hook":   {"ls", "add", "run", "clear", "timeout"},
	"files":  {"include", "exclude", "clear"},
	"config": {"set"},
}

// hintFor explains what a command still needs, so the prompt is never silent
// about what comes next.
func hintFor(key string) string {
	if h, ok := argHints[key]; ok {
		return fit(h)
	}
	return ""
}

var argHints = map[string]string{
	"hook add":     "<repo> <after-pull|after-push|on-conflict> <command>",
	"hook run":     "<repo> <after-pull|after-push|on-conflict>",
	"hook clear":   "<repo> [event]",
	"hook timeout": "<repo> <seconds>",
	"repo add":     "<owner/name>   [--path ./dir] [--branch main] [--interval 10]",
	"auth login":   "[--pat <token>]",
	"auth setup":   "<github oauth client id>",
	"config set":   "<key> <value>   keys: strategy, interval_minutes, backup, notify, …",
	"files":        "<repo>   then: include | exclude | clear",
	"set":          "<repo> <name|remote|branch|interval> <value>",
	"restore":      "<repo> <backup-branch>",
	"resolve":      "<repo> [--ours | --theirs | --abort]",
	"prune":        "<repo> [--days N]",
	"pull":         "[--force]",
	"push":         "[--force]",
	"status":       "[--fetch]",
	"log":          "[n] [--live]",
	"init":         "[path]   defaults to the current directory",
}

func takesRepo(cmd string) bool {
	switch cmd {
	case "pull", "push", "sync", "status", "diff", "remove", "enable", "disable",
		"set", "resolve", "backups", "restore", "prune", "files", "hook", "watch":
		return true
	}
	return false
}

func matching(pool []string, prefix string) []string {
	var out []string
	for _, s := range pool {
		if strings.HasPrefix(strings.ToLower(s), strings.ToLower(prefix)) {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// fit keeps a hint on one line so it never wraps and corrupts the redraw.
func fit(s string) string {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width < 20 {
		width = 100
	}
	r := []rune(s)
	if len(r) <= width-1 {
		return s
	}
	return string(r[:width-2]) + "…"
}
