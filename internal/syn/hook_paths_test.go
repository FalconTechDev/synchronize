package syn

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookWantsOnlyItsPaths(t *testing.T) {
	h := Hook{Run: "restart", Paths: []string{"src/**", "go.mod"}}
	for _, tc := range []struct {
		changed []string
		want    bool
	}{
		{[]string{"src/main.go"}, true},
		{[]string{"go.mod"}, true},
		{[]string{"README.md"}, false},
		{[]string{"docs/a.md", "src/deep/b.go"}, true},
		{[]string{"srcnotreally/x"}, false},
		// Nothing to go on is not the same as nothing matching: a hook that
		// would otherwise never fire should fire.
		{nil, true},
	} {
		if got := h.Wants(tc.changed); got != tc.want {
			t.Errorf("Wants(%v) = %v, want %v", tc.changed, got, tc.want)
		}
	}
	if !(Hook{Run: "always"}).Wants([]string{"anything"}) {
		t.Error("a hook without paths must run for everything")
	}
}

// Configs written before paths existed hold plain strings. They have to keep
// loading, and a hook that gains nothing from the new shape should be
// written back in the old one rather than churn the file.
func TestHookJSONIsBackwardCompatible(t *testing.T) {
	var hooks Hooks
	old := `{"after_pull":["systemctl restart bot"],"timeout_seconds":60}`
	if err := json.Unmarshal([]byte(old), &hooks); err != nil {
		t.Fatalf("an old config no longer loads: %v", err)
	}
	if len(hooks.AfterPull) != 1 || hooks.AfterPull[0].Run != "systemctl restart bot" {
		t.Fatalf("after_pull = %+v", hooks.AfterPull)
	}
	if hooks.AfterPull[0].Paths != nil {
		t.Error("a plain string hook must not invent paths")
	}

	out, err := json.Marshal(hooks)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `["systemctl restart bot"]`) {
		t.Errorf("a pathless hook should round-trip as a plain string, got %s", out)
	}

	hooks.AfterPull[0].Paths = []string{"src/**"}
	out, err = json.Marshal(hooks)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"paths":["src/**"]`) {
		t.Errorf("a hook with paths should be written as an object, got %s", out)
	}
	var back Hooks
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.AfterPull[0].Paths) != 1 || back.AfterPull[0].Paths[0] != "src/**" {
		t.Errorf("paths did not survive the round trip: %+v", back.AfterPull[0])
	}
}

func TestHookWithPathsSitsOutAnUnrelatedPull(t *testing.T) {
	env := newTestEnv(t)
	fired := filepath.Join(env.dir, "fired.txt")
	env.b.Hooks.AfterPull = HookList{{
		Run:   writeVar("SYN_EVENT", fired),
		Paths: []string{"src/**"},
	}}

	// A change nowhere near src/ must not wake the hook.
	env.write(env.a, "README.md", "docs only\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	if res := env.engine.Pull(env.b); !res.OK() || res.Pulled == 0 {
		t.Fatalf("pull: %+v (%v)", res, res.Err)
	}
	if _, err := os.Stat(fired); err == nil {
		t.Fatal("the hook ran for a change outside the paths it asked for")
	}

	// One inside src/ must.
	if err := os.MkdirAll(filepath.Join(env.a.Path, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	env.write(env.a, filepath.Join("src", "main.go"), "package main\n")
	if res := env.engine.Push(env.a); !res.OK() {
		t.Fatalf("push: %v", res.Err)
	}
	if res := env.engine.Pull(env.b); !res.OK() {
		t.Fatalf("pull: %+v (%v)", res, res.Err)
	}
	if _, err := os.Stat(fired); err != nil {
		t.Fatalf("the hook did not run for a change under src/: %v", err)
	}
}
