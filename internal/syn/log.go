package syn

import (
	"bufio"
	"bytes"
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
	f, err := os.Open(LogPath())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if f != nil {

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

func TailLines(out chan<- string, stop <-chan struct{}) {
	defer close(out)
	pr, pw := io.Pipe()
	go func() {
		<-stop
		pw.Close()
	}()
	go LiveTail(pw, stop)

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
