package syn

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Knowing whether the watcher is actually running.
//
// Until now there was no way to ask. `syn auto` said whether autostart was
// switched on, which is a different question - the task can be registered and
// the process dead, or killed by hand an hour ago, and everything would look
// fine while nothing synced. So the watcher leaves a note saying it is here,
// and refreshes it as it goes.

type Heartbeat struct {
	PID     int       `json:"pid"`
	Started time.Time `json:"started"`
	Updated time.Time `json:"updated"`
	Repos   int       `json:"repos"`
}

func heartbeatPath() string { return filepath.Join(Home(), "watcher.json") }

// staleAfter is how long without a refresh means "not running". The loop
// comes round every few seconds, so a minute is a wide margin; a crashed or
// killed watcher is reported as gone rather than lingering as a ghost.
const staleAfter = time.Minute

type heartbeatWriter struct {
	hb Heartbeat
}

func startHeartbeat(repos int) *heartbeatWriter {
	w := &heartbeatWriter{hb: Heartbeat{
		PID:     os.Getpid(),
		Started: time.Now(),
		Repos:   repos,
	}}
	w.beat()
	return w
}

func (w *heartbeatWriter) beat() {
	if w == nil {
		return
	}
	w.hb.Updated = time.Now()
	b, err := json.MarshalIndent(w.hb, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(Home(), 0o755); err != nil {
		return
	}
	// Written in place rather than through a temporary file: it is a hint,
	// not a record, and a half-written one is simply ignored as unreadable.
	if err := os.WriteFile(heartbeatPath(), b, 0o600); err != nil {
		Debug("could not write the watcher note: %v", err)
	}
}

// stop removes the note, so a watcher that exited tidily is reported as gone
// straight away instead of after the staleness window.
func (w *heartbeatWriter) stop() {
	if w == nil {
		return
	}
	if err := os.Remove(heartbeatPath()); err != nil && !os.IsNotExist(err) {
		Debug("could not remove the watcher note: %v", err)
	}
}

// RunningWatcher reports the watcher that is running now, or nil if none is.
func RunningWatcher() *Heartbeat {
	b, err := os.ReadFile(heartbeatPath())
	if err != nil {
		return nil
	}
	var hb Heartbeat
	if err := json.Unmarshal(b, &hb); err != nil {
		return nil
	}
	if time.Since(hb.Updated) > staleAfter {
		return nil // left behind by a watcher that was killed
	}
	return &hb
}

// NextCheck says when this repository is due to be looked at again, given
// when it was last synced. Zero means "as soon as the watcher gets to it".
func (e *Engine) NextCheck(r *Repo) time.Time {
	last, ok := e.State.LastSync[r.Name]
	if !ok || last.IsZero() {
		return time.Time{}
	}
	return last.Add(time.Duration(e.Cfg.IntervalFor(r)) * time.Minute)
}
