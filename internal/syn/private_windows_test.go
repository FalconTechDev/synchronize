//go:build windows

package syn

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func aceCount(t *testing.T, path string) (int, string) {
	t.Helper()
	out, err := exec.Command("icacls", path).CombinedOutput()
	if err != nil {
		t.Skipf("icacls is not usable here: %v", err)
	}
	text := decodeConsole(out)
	n := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, ":(") {
			n++
		}
	}
	return n, text
}

// The token files were readable by every account the folder's ACL listed,
// because 0600 means nothing on Windows and the file inherited the rest.
func TestCredentialsAreReadableByTheOwnerOnly(t *testing.T) {
	t.Setenv("SYNCHRONIZE_HOME", t.TempDir())

	c := LoadCredentials()
	c.Set("github.com", "ghp_test")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(Home(), "credentials.json")
	n, text := aceCount(t, path)
	if n != 1 {
		t.Errorf("credentials.json lists %d accounts with access, want only its owner:\n%s", n, text)
	}
	// The file still has to work.
	if got := LoadCredentials().Token("github.com"); got != "ghp_test" {
		t.Errorf("the token did not survive being restricted: %q", got)
	}
}

// The Telegram token lives in config.json.
func TestConfigIsReadableByTheOwnerOnly(t *testing.T) {
	t.Setenv("SYNCHRONIZE_HOME", t.TempDir())

	cfg := DefaultConfig()
	cfg.Telegram = Telegram{Token: "123:abc", ChatID: "42"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	n, text := aceCount(t, filepath.Join(Home(), "config.json"))
	if n != 1 {
		t.Errorf("config.json lists %d accounts with access, want only its owner:\n%s", n, text)
	}
	if back, err := LoadConfig(); err != nil || back.Telegram.ChatID != "42" {
		t.Errorf("the config did not survive being restricted: %v %+v", err, back)
	}
}

// Saving again must keep it private and keep working - the second save
// replaces a file that already has the restricted ACL.
func TestSavingAgainKeepsItPrivate(t *testing.T) {
	t.Setenv("SYNCHRONIZE_HOME", t.TempDir())
	for i := 0; i < 3; i++ {
		c := LoadCredentials()
		c.Set("github.com", "ghp_"+strings.Repeat("x", i+1))
		if err := c.Save(); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}
	path := filepath.Join(Home(), "credentials.json")
	if n, text := aceCount(t, path); n != 1 {
		t.Errorf("after three saves %d accounts have access:\n%s", n, text)
	}
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Error("a temporary file was left behind")
	}
}
