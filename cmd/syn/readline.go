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
		_, hint := complete(string(line[:pos]))
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
				clearHint()
				fmt.Print("\r\n" + cDim + strings.Join(options, "   ") + cReset + "\r\n")
				redraw()
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

// completeLine offers command names for the first word and repository names
// for anything that takes a repo, plus a one-line hint under the prompt.
func completeLine(line string, commands, repos []string) ([]string, string) {
	word, _ := lastWord(line)
	fields := strings.Fields(line)
	first := len(fields) == 0 || (len(fields) == 1 && !strings.HasSuffix(line, " "))

	if first {
		return matching(commands, word), ""
	}

	cmd := fields[0]
	if !takesRepo(cmd) {
		return nil, ""
	}

	if len(repos) == 0 {
		return nil, "no repositories registered yet - try: repo add owner/name"
	}

	// Once a repository has been typed, stop offering the list again.
	typedRepo := len(fields) > 1 && strings.HasSuffix(line, " ")

	options := matching(repos, word)
	switch {
	case typedRepo:
		return options, ""
	case word != "" && len(options) == 0:
		return nil, "no repository matches " + word
	default:
		return options, "repos: " + strings.Join(repos, "   ")
	}
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
