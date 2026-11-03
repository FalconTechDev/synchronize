package main

import (
	"testing"
	"time"

	"github.com/FalconTechDev/synchronize/internal/syn"
)

// The error for a bad duration says "like 30m, 2h or 1d", and 1d used to be
// refused with that very message.
func TestPauseTakesTheFormsItsErrorOffers(t *testing.T) {
	t.Setenv("SYNCHRONIZE_HOME", t.TempDir())

	for _, tc := range []struct {
		arg  string
		want time.Duration
	}{
		{"30m", 30 * time.Minute},
		{"2h", 2 * time.Hour},
		{"1d", 24 * time.Hour},
		{"2h30m", 150 * time.Minute},
		{"90", 90 * time.Minute}, // a bare number is minutes, as for an interval
	} {
		cfg := syn.DefaultConfig()
		cfg.Repos = []syn.Repo{{Name: "r", Path: t.TempDir(), Enabled: true}}

		before := time.Now()
		if err := cmdPause(cfg, []string{"r", tc.arg}); err != nil {
			t.Errorf("pause %s: %v", tc.arg, err)
			continue
		}
		got := cfg.Repos[0].PausedUntil.Sub(before)
		if got < tc.want-time.Minute || got > tc.want+time.Minute {
			t.Errorf("pause %s lasts %v, want about %v", tc.arg, got.Round(time.Minute), tc.want)
		}
	}
}

func TestPauseRefusesNonsense(t *testing.T) {
	t.Setenv("SYNCHRONIZE_HOME", t.TempDir())
	for _, bad := range []string{"abc", "-1h", "-5", "1.x", "h"} {
		cfg := syn.DefaultConfig()
		cfg.Repos = []syn.Repo{{Name: "r", Path: t.TempDir(), Enabled: true}}
		if err := cmdPause(cfg, []string{"r", bad}); err == nil {
			t.Errorf("pause %q should have been refused", bad)
		}
		if !cfg.Repos[0].PausedUntil.IsZero() {
			t.Errorf("pause %q was refused but still paused the repository", bad)
		}
	}
}

func TestPauseOffEndsIt(t *testing.T) {
	t.Setenv("SYNCHRONIZE_HOME", t.TempDir())
	cfg := syn.DefaultConfig()
	cfg.Repos = []syn.Repo{{Name: "r", Path: t.TempDir(), Enabled: true,
		PausedUntil: time.Now().Add(time.Hour)}}
	for _, word := range []string{"off", "0"} {
		cfg.Repos[0].PausedUntil = time.Now().Add(time.Hour)
		if err := cmdPause(cfg, []string{"r", word}); err != nil {
			t.Fatal(err)
		}
		if !cfg.Repos[0].PausedUntil.IsZero() {
			t.Errorf("pause %s did not end the pause", word)
		}
	}
}
