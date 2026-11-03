package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/FalconTechDev/synchronize/internal/syn"
)

var shells = []string{"bash", "zsh", "fish", "powershell"}

func TestCompletableCommands(t *testing.T) {
	got := completable()
	seen := map[string]bool{}
	for _, c := range got {
		if seen[c] {
			t.Errorf("%q is listed twice", c)
		}
		seen[c] = true
	}
	// These only exist inside `syn shell`; offering them at a real prompt
	// would complete to something that is not a command there.
	for _, c := range []string{"clear", "exit"} {
		if seen[c] {
			t.Errorf("%q is a shell-only command and must not be completed", c)
		}
	}
	for _, c := range []string{"pull", "push", "status", "help", "version", "completion"} {
		if !seen[c] {
			t.Errorf("%q is missing", c)
		}
	}
}

// takesRepo is written out by hand, which is how a command gets misspelt in
// it and quietly never completes a repository.
func TestEveryRepoCommandIsARealCommand(t *testing.T) {
	for name := range takesRepo {
		if _, ok := catalog[name]; !ok {
			t.Errorf("takesRepo names %q, which is not in the catalogue", name)
		}
	}
}

func TestFlagsComeFromTheCatalogue(t *testing.T) {
	d := buildCompletionData()
	for cmd, want := range map[string]string{"pull": "--force", "pull ": "--to", "sync": "--dry-run", "status": "--json"} {
		cmd = strings.TrimSpace(cmd)
		found := false
		for _, f := range d.Flags[cmd] {
			if f == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s does not offer %s: %v", cmd, want, d.Flags[cmd])
		}
	}
}

func TestEveryShellKnowsEveryCommandAndSubcommand(t *testing.T) {
	d := buildCompletionData()
	for _, shell := range shells {
		script, err := completionScript(shell)
		if err != nil {
			t.Fatalf("%s: %v", shell, err)
		}
		for _, c := range d.Commands {
			if !strings.Contains(script, c) {
				t.Errorf("%s: no mention of the command %q", shell, c)
			}
		}
		for cmd, subs := range d.Subs {
			for _, sub := range subs {
				if !strings.Contains(script, sub) {
					t.Errorf("%s: no mention of %s's subcommand %q", shell, cmd, sub)
				}
			}
		}
		if !strings.Contains(script, "ls --names") {
			t.Errorf("%s: repository names are not taken from `syn ls --names`", shell)
		}
	}
}

func TestUnknownShell(t *testing.T) {
	if _, err := completionScript("klingon"); err == nil {
		t.Error("an unknown shell should be refused")
	}
	if _, err := completionScript("pwsh"); err != nil {
		t.Errorf("pwsh is PowerShell: %v", err)
	}
}

// runShell runs a program with a deadline. A launcher that hangs - the WSL
// stub that sits at C:\Windows\System32\bash.exe does exactly that when no
// distribution is installed - must skip the test, not stall the suite.
func runShell(t *testing.T, name string, args ...string) (string, bool) {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, args...).CombinedOutput()
	if ctx.Err() != nil {
		t.Logf("%s timed out and was skipped", name)
		return "", false
	}
	if err != nil {
		t.Logf("%s exited with %v: %s", name, err, out)
		return string(out), false
	}
	return string(out), true
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(p)
}

// The real thing: source the script in a real bash and ask it what Tab would
// offer. repositories come from a function standing in for `syn ls --names`.
func TestBashCompletionInARealBash(t *testing.T) {
	if v, ok := runShell(t, "bash", "--version"); !ok || !strings.Contains(v, "GNU bash") {
		t.Skip("no usable GNU bash here")
	}
	script, _ := completionScript("bash")
	file := writeTemp(t, "syn.bash", script)

	if out, ok := runShell(t, "bash", "-n", file); !ok {
		t.Fatalf("the script does not parse: %s", out)
	}

	driver := writeTemp(t, "drive.sh", `
syn() { printf 'my-notes\nmy-bot\n'; }
source "$1"
try() { COMP_CWORD=$1; shift; COMP_WORDS=("$@"); _syn; printf '%s => %s\n' "$*" "${COMPREPLY[*]}"; }
try 1 syn pu
try 2 syn hook ro
try 2 syn pull --f
try 2 syn pull my
try 3 syn hook add my-n
try 2 syn completion ""
try 2 syn config ""
`)
	out, ok := runShell(t, "bash", driver, file)
	if !ok {
		t.Fatalf("driving the completion failed: %s", out)
	}
	for _, want := range []string{
		"syn pu => pull push",
		"syn hook ro => rollback",
		"syn pull --f => --force",
		"syn pull my => my-notes my-bot",
		"syn hook add my-n => my-notes",
		"syn completion  => bash fish powershell zsh",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// `config` takes no repository, so it must not be offered one.
	if strings.Contains(out, "syn config  => all") || strings.Contains(out, "my-notes") && strings.Contains(out, "config") && strings.Contains(out, "syn config  => my-notes") {
		t.Errorf("a command that takes no repository was offered repositories:\n%s", out)
	}
}

func TestPowerShellCompletionParsesAndCompletes(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell is only exercised on Windows")
	}
	script, _ := completionScript("powershell")
	file := writeTemp(t, "syn.ps1", script)

	cmd := `$s = Get-Content -Raw -LiteralPath '` + file + `'; ` +
		`[void][scriptblock]::Create($s); Invoke-Expression $s; ` +
		`$r = TabExpansion2 -inputScript 'syn pu' -cursorColumn 6; ` +
		`($r.CompletionMatches | ForEach-Object CompletionText) -join ' '`
	out, ok := runShell(t, "powershell", "-NoProfile", "-NonInteractive", "-Command", cmd)
	if !ok {
		t.Skip("PowerShell is not usable here")
	}
	if !strings.Contains(out, "pull") || !strings.Contains(out, "push") {
		t.Errorf("`syn pu<Tab>` gave %q, want pull and push", out)
	}
}

// A command that works but is not in the catalogue has no hint under the
// shell prompt, no `--help`, and no completion: three at once, and nothing
// complains. `path`, `pause` and `ignore` were like that. This reads the
// README and the guide - the commands a person is told to type - and fails
// for any that the catalogue does not know.
func TestEveryDocumentedCommandIsInTheCatalogue(t *testing.T) {
	known := map[string]bool{}
	for _, c := range completable() {
		known[c] = true
	}
	// Spellings that are real but are another name for one in the catalogue.
	for _, alias := range []string{"list", "rm"} {
		known[alias] = true
	}

	for _, doc := range []string{"../../README.md", "../../GUIDE.md"} {
		b, err := os.ReadFile(doc)
		if err != nil {
			t.Skipf("cannot read %s: %v", doc, err)
		}
		// Only what is meant to be typed: inside a fenced block, or between
		// a pair of backticks. "syn is a wrapper around git" is prose.
		inFence := false
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inFence = !inFence
				continue
			}
			var candidates []string
			if inFence {
				typed := strings.TrimPrefix(strings.TrimSpace(line), "$")
				candidates = []string{strings.TrimSpace(typed)}
			} else {
				for i, piece := range strings.Split(line, "`") {
					if i%2 == 1 {
						candidates = append(candidates, piece)
					}
				}
			}
			for _, part := range candidates {
				if !strings.HasPrefix(part, "syn ") {
					continue
				}
				words := strings.Fields(part)
				if len(words) < 2 {
					continue
				}
				word := words[1]
				if strings.HasPrefix(word, "-") || strings.ContainsAny(word, "<>|$./\\\"'[(") {
					continue
				}
				if !known[word] {
					t.Errorf("%s tells people to run `syn %s`, which the catalogue does not know", filepath.Base(doc), word)
				}
			}
		}
	}
}

// Completion runs `syn ls --names` on every Tab, so it has to be nothing but
// the names, one to a line, and cheap.
func TestLsNamesPrintsOnlyNames(t *testing.T) {
	t.Setenv("SYNCHRONIZE_HOME", t.TempDir())
	cfg := syn.DefaultConfig()
	cfg.Repos = []syn.Repo{
		{Name: "my-notes", Path: t.TempDir(), Enabled: true},
		{Name: "my-bot", Path: t.TempDir(), Enabled: true},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := run([]string{"ls", "--names"})
	w.Close()
	os.Stdout = old

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	if runErr != nil {
		t.Fatal(runErr)
	}
	if got := strings.Split(strings.TrimSpace(string(buf[:n])), "\n"); len(got) != 2 ||
		strings.TrimSpace(got[0]) != "my-notes" || strings.TrimSpace(got[1]) != "my-bot" {
		t.Errorf("ls --names printed %q, want the two names and nothing else", buf[:n])
	}
}
