package syn

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	stdlog "log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	logMu sync.Mutex

	Quiet bool

	Verbose bool
)

func LogPath() string { return filepath.Join(Home(), "synchronize.log") }

func writeLine(level, msg string) {
	line := fmt.Sprintf("%s [%s] %s", time.Now().Format("2006-01-02 15:04:05"), level, msg)
	logMu.Lock()
	defer logMu.Unlock()
	if err := os.MkdirAll(Home(), 0o755); err == nil {
		if f, err := os.OpenFile(LogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			fmt.Fprintln(f, line)
			f.Close()
		}
	}
	if Quiet {
		return
	}
	switch level {
	case "ERROR", "WARN":
		fmt.Fprintln(os.Stderr, line)
	case "DEBUG":
		if Verbose {
			fmt.Println(line)
		}
	default:
		fmt.Println(line)
	}
}

func Info(format string, a ...any) { writeLine("INFO", fmt.Sprintf(format, a...)) }

func Warn(format string, a ...any) { writeLine("WARN", fmt.Sprintf(format, a...)) }

func Errorf(format string, a ...any) { writeLine("ERROR", fmt.Sprintf(format, a...)) }

func Debug(format string, a ...any) { writeLine("DEBUG", fmt.Sprintf(format, a...)) }

func colorLine(line string) string {
	switch {
	case strings.Contains(line, "[ERROR]"):
		return "\033[31m" + line + "\033[0m"
	case strings.Contains(line, "[WARN]"):
		return "\033[33m" + line + "\033[0m"
	case strings.Contains(line, "[INFO]"):
		return "\033[32m" + line + "\033[0m"
	case strings.Contains(line, "[DEBUG]"):
		return "\033[90m" + line + "\033[0m"
	}
	return line
}

func LiveTail(w io.Writer, stop <-chan struct{}) error {
	return LiveTailFor(w, "", stop)
}

// LiveTailFor streams the log, optionally only the lines about one
// repository.
func LiveTailFor(w io.Writer, repo string, stop <-chan struct{}) error {
	f, err := os.Open(LogPath())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if f != nil {

		_, _ = f.Seek(0, io.SeekEnd)
	}
	for {
		select {
		case <-stop:
			if f != nil {
				f.Close()
			}
			return nil
		case <-time.After(250 * time.Millisecond):
		}

		if f == nil {
			f, err = os.Open(LogPath())
			if err != nil {
				f = nil
				continue
			}
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if repo != "" && !lineMentions(line, repo) {
				continue
			}
			fmt.Fprintln(w, colorLine(line))
		}
	}
}

func TailLines(out chan<- string, stop <-chan struct{}) {
	defer close(out)
	pr, pw := io.Pipe()
	go func() {
		<-stop
		pw.Close()
	}()
	go func() { _ = LiveTail(pw, stop) }()

	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 1024)
	for {
		n, err := pr.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			for {
				i := bytes.IndexByte(buf, '\n')
				if i < 0 {
					break
				}
				line := strings.TrimRight(string(buf[:i]), "\r")
				buf = buf[i+1:]
				if line != "" {
					select {
					case out <- line:
					case <-stop:
						return
					}
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func TailLog(n int) string { return TailLogFor("", n) }

// TailLogFor is the log narrowed to one repository. One file holds every
// repository, and with several of them syncing on a timer the lines you
// wanted are a few in a hundred.
func TailLogFor(repo string, n int) string {
	b, err := os.ReadFile(LogPath())
	if err != nil {
		return "(no log yet)"
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if repo != "" {
		kept := make([]string, 0, len(lines))
		for _, line := range lines {
			if lineMentions(line, repo) {
				kept = append(kept, line)
			}
		}
		if len(kept) == 0 {
			return fmt.Sprintf("(nothing logged for %s yet)", repo)
		}
		lines = kept
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// lineMentions asks whether a log line is about this repository. syn puts the
// name at the front of its own lines ("repo: pulled 2 commits"), but hook
// output is indented under it and a path can carry the name inside a longer
// word, so the test is the name standing on its own - not appearing anywhere
// at all, which would make "bot" match "bot-staging".
func lineMentions(line, repo string) bool {
	for i := 0; i <= len(line)-len(repo); {
		j := strings.Index(line[i:], repo)
		if j < 0 {
			return false
		}
		start := i + j
		end := start + len(repo)
		if !isNameChar(byteAt(line, start-1)) && !isNameChar(byteAt(line, end)) {
			return true
		}
		i = end
	}
	return false
}

func byteAt(s string, i int) byte {
	if i < 0 || i >= len(s) {
		return ' '
	}
	return s[i]
}

func isNameChar(c byte) bool {
	return c == '-' || c == '_' || c == '.' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// stdLogSink hands anything the standard library's log package prints to our
// own log file. Libraries we depend on - systray most of all - report their
// troubles that way, and unattended those lines land in the console window
// the watcher is trying to keep out of sight.
type stdLogSink struct{}

func (stdLogSink) Write(p []byte) (int, error) {
	writeLine("WARN", strings.TrimSpace(string(p)))
	return len(p), nil
}

// CaptureLibraryLogs keeps library chatter in the log file instead of on
// screen. Only worth doing for the long-running commands.
func CaptureLibraryLogs() {
	stdlog.SetFlags(0)
	stdlog.SetOutput(stdLogSink{})
}
