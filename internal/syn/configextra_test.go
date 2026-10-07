package syn

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The case that actually bites: two copies of syn on one machine, one older
// than the other. The older one does not know about `app` or `pull_only`,
// and saving from it used to wipe them.
func TestAnOlderVersionSavingDoesNotWipeNewerRepoSettings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SYNCHRONIZE_HOME", dir)

	// What a newer syn wrote. "app" and "pull_only" stand in for anything
	// this build might not have.
	raw := `{
      "interval_minutes": 10,
      "repos": [
        {
          "name": "bot",
          "path": "/srv/bot",
          "remote": "origin",
          "enabled": true,
          "pull_only": true,
          "from_the_future": {"nested": ["values", 1, true]}
        }
      ],
      "future_global": "keep"
    }`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	// Something this build does understand changes, as an ordinary command
	// would change it.
	r := cfg.Find("bot")
	if r == nil {
		t.Fatal("the repository did not load")
	}
	r.IntervalMinutes = 5
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"from_the_future", "future_global", "nested"} {
		if !strings.Contains(string(after), want) {
			t.Errorf("%s was dropped:\n%s", want, after)
		}
	}
	// And the change this build made is there, and the file is still valid.
	var check map[string]any
	if err := json.Unmarshal(after, &check); err != nil {
		t.Fatalf("the config is no longer valid JSON: %v\n%s", err, after)
	}
	reloaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Find("bot"); got == nil || got.IntervalMinutes != 5 {
		t.Errorf("the edit did not survive: %+v", got)
	}
	if got := reloaded.Find("bot"); got != nil && !got.PullOnly {
		t.Error("a field this build does know was lost")
	}
}

// Saving twice with nothing changed must produce the same bytes, or every
// sync would look like a config edit.
func TestSavingTwiceIsStable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SYNCHRONIZE_HOME", dir)
	raw := `{"interval_minutes":10,"repos":[{"name":"a","path":"/a","remote":"origin","enabled":true,"zz":1,"aa":2}],"zzz":3,"aaa":4}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg2, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg2.Save(); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("saving twice changed the file:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

// A config with no unknown keys must not grow any, and must stay readable.
func TestOrdinaryConfigIsUnaffected(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SYNCHRONIZE_HOME", dir)
	cfg := DefaultConfig()
	cfg.Repos = []Repo{{Name: "a", Path: "/a", Remote: "origin", Enabled: true}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var check map[string]any
	if err := json.Unmarshal(b, &check); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, b)
	}
	back, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Repos) != 1 || back.Repos[0].Name != "a" {
		t.Errorf("round trip lost the repository: %+v", back.Repos)
	}
}
