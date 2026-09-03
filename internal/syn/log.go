package syn

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	logMu sync.Mutex
	// Quiet suppresses stdout output; the log file is still written.
	Quiet bool
	// Verbose adds debug lines.
	Verbose bool
)

// LogPath is where every run appends its trail.
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

// Info logs a normal progress line.
func Info(format string, a ...any) { writeLine("INFO", fmt.Sprintf(format, a...)) }

// Warn logs a recoverable problem.
func Warn(format string, a ...any) { writeLine("WARN", fmt.Sprintf(format, a...)) }

// Errorf logs a failure.
func Errorf(format string, a ...any) { writeLine("ERROR", fmt.Sprintf(format, a...)) }

// Debug logs only when -v is passed.
func Debug(format string, a ...any) { writeLine("DEBUG", fmt.Sprintf(format, a...)) }

// colorLine adds ANSI color codes based on log level.
func colorLine(line string) string {
	switch {
	case strings.Contains(line, "[ERROR]"):
		return "\033[31m" + line + "\033[0m" // red
	case strings.Contains(line, "[WARN]"):
		return "\033[33m" + line + "\033[0m" // yellow
	case strings.Contains(line, "[INFO]"):
		return "\033[32m" + line + "\033[0m" // green
	case strings.Contains(line, "[DEBUG]"):
		return "\033[90m" + line + "\033[0m" // grey
	}
	return line
}

// LiveTail streams new lines appended to the log file to w until stop is closed.
func LiveTail(w io.Writer, stop <-chan struct{}) error {
	f, err := os.Open(LogPath())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if f != nil {
		// Seek to end so we only show new lines.
		f.Seek(0, io.SeekEnd)
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
		// (Re)open if the file appeared after we started.
		if f == nil {
			f, err = os.Open(LogPath())
			if err != nil {
				f = nil
				continue
			}
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			fmt.Fprintln(w, colorLine(scanner.Text()))
		}
	}
}

// TailLog returns the last n lines of the log file.
func TailLog(n int) string {
	b, err := os.ReadFile(LogPath())
	if err != nil {
		return "(no log yet)"
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
