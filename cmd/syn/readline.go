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
	pending := "" // set by Tab, shown until the next keystroke

	promptW := visibleWidth(prompt)

	// Everything lives on the prompt line: the hint trails the input in grey
	// and the cursor is put back afterwards. No line above or below is ever
	// touched, so scrolling cannot leave a stray hint in the scrollback.
	draw := func() {
		hint := pending
		if hint == "" {
			_, hint = complete(string(line[:pos]))
		}

		out := "\r\033[K" + prompt + string(line)
		if hint != "" {
			room := termWidth() - promptW - len(line) - 4
			if room > 8 {
				out += "   " + cDim + clip(hint, room) + cReset
			}
		}
		out += "\r"
		if n := promptW + pos; n > 0 {
			out += fmt.Sprintf("\033[%dC", n)
		}
		fmt.Print(out)
	}

	// finish drops the hint and leaves the cursor on a fresh line.
	finish := func() {
		pending = ""
		fmt.Print("\r\033[K" + prompt + string(line) + "\r\n")
	}

	draw()

	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			finish()
			return "", false
		}

		switch b := buf[0]; b {
		case 3, 4: // Ctrl+C, Ctrl+D
			finish()
			return "", false

		case 13, 10: // Enter
			finish()
			s := strings.TrimSpace(string(line))
			if s != "" {
				e.history = append(e.history, s)
			}
			return s, true

		case 9: // Tab
			options, _ := complete(string(line[:pos]))
			switch {
			case len(options) == 1:
				_, start := lastWord(string(line[:pos]))
				rest := []rune(options[0] + " ")
				line = append([]rune(string(line[:start])), append(rest, line[pos:]...)...)
				pos = start + len(rest)
				pending = ""
			case len(options) > 1:
				pending = strings.Join(options, "   ")
			}
			draw()

		case 127, 8: // Backspace
			if pos > 0 {
				line = append(line[:pos-1], line[pos:]...)
				pos--
				pending = ""
				draw()
			}

		case 27: // escape sequence
			seq := make([]byte, 2)
			if n, _ := os.Stdin.Read(seq); n < 2 {
				continue
			}
			if seq[0] != '[' {
				continue
			}
			pending = ""
			switch seq[1] {
			case 'A': // up
				if e.hpos > 0 {
					e.hpos--
					line = []rune(e.history[e.hpos])
					pos = len(line)
					draw()
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
				draw()
			case 'C': // right
				if pos < len(line) {
					pos++
					draw()
				}
			case 'D': // left
				if pos > 0 {
					pos--
					draw()
				}
			case 'H':
				pos = 0
				draw()
			case 'F':
				pos = len(line)
				draw()
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
				rest := make([]byte, extra)
				if extra > 0 {
					os.Stdin.Read(rest)
				}
				rs := []rune(string(append([]byte{b}, rest...)))
				if len(rs) == 0 {
					continue
				}
				r = rs[0]
			}
			line = append(line[:pos], append([]rune{r}, line[pos:]...)...)
			pos++
			pending = ""
			draw()
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
// belongs there: a command, a subcommand, a repository or a hook event, plus
// a reminder of what the command still expects.
func completeLine(line string, commands, repos []string) ([]string, string) {
	// Typing the program name is a habit; ignore it here the same way the
	// shell does when the line is run.
	if strings.HasPrefix(line, "syn ") {
		line = line[4:]
	}

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
		return matching(events, word), fit("event: " + strings.Join(events, "   "))
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

var subcommands = map[string][]string{
	"auth":   {"login", "logout", "status", "setup"},
	"repo":   {"add", "ls"},
	"hook":   {"ls", "add", "run", "clear", "timeout"},
	"files":  {"include", "exclude", "clear"},
	"config": {"set"},
	"autostart": {"on", "off"},
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

func termWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w < 20 {
		return 100
	}
	return w
}

// fit keeps a hint within the terminal so the prompt line never wraps.
func fit(s string) string { return clip(s, termWidth()-1) }

func clip(s string, max int) string {
	r := []rune(s)
	if max < 2 || len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

// visibleWidth measures a string as the terminal draws it, ignoring colours.
func visibleWidth(s string) int {
	n, inEsc := 0, false
	for _, r := range s {
		switch {
		case inEsc:
			if r == 'm' {
				inEsc = false
			}
		case r == 0x1b:
			inEsc = true
		default:
			n++
		}
	}
	return n
}
