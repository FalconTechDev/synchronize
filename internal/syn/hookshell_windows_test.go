//go:build windows

package syn

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// A path with a space needs quoting, and quoting is exactly what Go's own
// argument escaping used to break: cmd got \" and refused the line.
func TestHookHandlesAQuotedPathWithASpace(t *testing.T) {
	env := newTestEnv(t)
	dir := filepath.Join(env.dir, "with space")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "written.txt")

	run := env.engine.runHook(env.a,
		hookInfo{Event: "after-pull"},
		`echo %SYN_EVENT% > "`+out+`"`,
		30*time.Second)
	if run.Err != nil {
		t.Fatalf("the hook failed: %v (%v)", run.Err, run.Output)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("nothing was written: %v", err)
	}
	if got := strings.TrimSpace(string(b)); got != "after-pull" {
		t.Errorf("file holds %q, want after-pull", got)
	}
}

func TestHookQuotingSurvivesInsideTheCommandToo(t *testing.T) {
	env := newTestEnv(t)
	out := filepath.Join(env.dir, "quoted.txt")
	run := env.engine.runHook(env.a,
		hookInfo{Event: "after-pull"},
		`echo "hello there" > "`+out+`"`,
		30*time.Second)
	if run.Err != nil {
		t.Fatalf("the hook failed: %v (%v)", run.Err, run.Output)
	}
	b, _ := os.ReadFile(out)
	if !strings.Contains(string(b), "hello there") {
		t.Errorf("file holds %q", b)
	}
}

// A hook that fails has to say why in the log, and cmd.exe says it in the
// console code page. Whatever the words are, they have to arrive as text.
func TestFailingHookOutputIsReadableText(t *testing.T) {
	env := newTestEnv(t)
	run := env.engine.runHook(env.a,
		hookInfo{Event: "after-pull"},
		`type C:\definitely\no\such\file.txt`,
		30*time.Second)
	if run.Err == nil {
		t.Fatal("that command was supposed to fail")
	}
	if len(run.Output) == 0 {
		t.Fatal("a failing hook told us nothing at all")
	}
	for _, line := range run.Output {
		if !utf8.ValidString(line) {
			t.Errorf("the log line is not text: %q", line)
		}
		if strings.ContainsRune(line, '\uFFFD') {
			t.Errorf("the log line came through mangled: %q", line)
		}
	}
	t.Logf("cmd said: %q", run.Output)
}

func TestDecodeConsoleLeavesUTF8Alone(t *testing.T) {
	// A hook of your own that writes UTF-8 must not be decoded as OEM.
	for _, s := range []string{"plain ascii", "привет", "日本語", ""} {
		if got := decodeConsole([]byte(s)); got != s {
			t.Errorf("decodeConsole(%q) = %q", s, got)
		}
	}
}

func TestDecodeConsoleRecoversOEMBytes(t *testing.T) {
	// CP866 for "Ошибка" - not valid UTF-8, so it takes the decoding path.
	oem := []byte{0x8E, 0xE8, 0xA8, 0xA1, 0xAA, 0xA0}
	if utf8.Valid(oem) {
		t.Skip("those bytes are valid UTF-8 after all")
	}
	got := decodeConsole(oem)
	if got == string(oem) {
		t.Skip("this machine's console code page is not CP866; nothing to compare against")
	}
	if !utf8.ValidString(got) {
		t.Fatalf("decoded to something that is not text: %q", got)
	}
	if got != "Ошибка" {
		t.Errorf("decoded to %q, want %q", got, "Ошибка")
	}
}
