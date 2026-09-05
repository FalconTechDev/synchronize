package syn

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// AppConfig is a program syn keeps running out of a repository - a bot, a
// small service - and restarts when the code it is made of arrives.
//
// This is not a hook. A hook is run and waited for, which is right for
// `systemctl restart` and wrong for the thing being restarted: a bot never
// returns, so as a hook it would hold the sync until the timeout killed it.
type AppConfig struct {
	Command string   `json:"command"`
	Paths   []string `json:"paths,omitempty"` // empty: any change restarts it
	Dir     string   `json:"dir,omitempty"`   // within the repository
}

// Wants says whether a pull that touched these files should restart the app.
// With no paths given, anything arriving is reason enough - the usual case,
// where the repository is the program.
func (a AppConfig) Wants(changed []string) bool {
	if len(a.Paths) == 0 || len(changed) == 0 {
		return true
	}
	return Hook{Paths: a.Paths}.Wants(changed)
}

// Supervisor keeps one app alive: starts it, brings it back if it falls over,
// and stops it on the way out.
type Supervisor struct {
	repo *Repo

	mu      sync.Mutex
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	done    chan struct{}
	stopped bool

	// failures counts starts that died almost immediately, in a row. A
	// program that cannot start at all should say so once and stay quiet,
	// not fill the log forever at one line every five seconds.
	failures int
}

const (
	appRestartDelay = 5 * time.Second
	appTooQuick     = 5 * time.Second
	appMaxFailures  = 5
)

func newSupervisor(r *Repo) *Supervisor { return &Supervisor{repo: r} }

func (s *Supervisor) dir() string {
	if s.repo.App != nil && s.repo.App.Dir != "" {
		return filepath.Join(s.repo.Path, s.repo.App.Dir)
	}
	return s.repo.Path
}

// Start launches the app and watches it in the background.
func (s *Supervisor) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = false
	s.startLocked()
}

func (s *Supervisor) startLocked() {
	app := s.repo.App
	if app == nil || app.Command == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := shellCommand(ctx, app.Command)
	cmd.Dir = s.dir()
	cmd.Env = hookEnv(s.repo, hookInfo{Event: "app"})
	detachFromConsole(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		Errorf("%s: could not start %q: %v", s.repo.Name, app.Command, err)
		return
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		cancel()
		Errorf("%s: could not start %q: %v", s.repo.Name, app.Command, err)
		return
	}
	Info("%s: started %s (pid %d)", s.repo.Name, app.Command, cmd.Process.Pid)

	s.cmd, s.cancel = cmd, cancel
	done := make(chan struct{})
	s.done = done

	go s.copyOutput(stdout)
	go s.wait(cmd, done)
}

// copyOutput puts the app's own output in the log, so `journalctl -u
// synchronize` shows the bot's messages next to the sync that restarted it.
func (s *Supervisor) copyOutput(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		Info("%s | %s", s.repo.Name, decodeConsole(sc.Bytes()))
	}
}

func (s *Supervisor) wait(cmd *exec.Cmd, done chan struct{}) {
	started := time.Now()
	err := cmd.Wait()
	close(done)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || s.cmd != cmd {
		return // we asked it to stop, or it has already been replaced
	}

	lived := time.Since(started)
	if lived < appTooQuick {
		s.failures++
	} else {
		s.failures = 0
	}

	switch {
	case err != nil:
		Errorf("%s: %s exited after %s: %v", s.repo.Name, s.repo.App.Command, lived.Round(time.Second), err)
	default:
		Warn("%s: %s exited after %s", s.repo.Name, s.repo.App.Command, lived.Round(time.Second))
	}

	if s.failures >= appMaxFailures {
		Errorf("%s: %s has died %d times in a row without staying up.\n"+
			"       leaving it stopped; it will be started again by the next update,\n"+
			"       or now with:  syn app %s start",
			s.repo.Name, s.repo.App.Command, s.failures, s.repo.Name)
		return
	}

	go func() {
		time.Sleep(appRestartDelay)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.stopped || s.cmd != cmd {
			return
		}
		Info("%s: starting it again", s.repo.Name)
		s.startLocked()
	}()
}

// Restart is what an update calls: the old process goes, the new code runs.
func (s *Supervisor) Restart() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
	s.failures = 0 // new code deserves a fresh set of attempts
	s.stopped = false
	s.startLocked()
}

func (s *Supervisor) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	s.stopLocked()
}

func (s *Supervisor) stopLocked() {
	if s.cmd == nil || s.cmd.Process == nil {
		return
	}
	cmd, done := s.cmd, s.done
	Info("%s: stopping %s (pid %d)", s.repo.Name, s.repo.App.Command, cmd.Process.Pid)

	// Ask, then insist. The shell we started is not the interesting process -
	// python is its child - so the whole tree has to go, or the old bot keeps
	// running beside the new one and both answer the same messages.
	terminateTree(cmd.Process.Pid)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		Warn("%s: it did not stop when asked; killing it", s.repo.Name)
		killTree(cmd.Process.Pid)
		if s.cancel != nil {
			s.cancel()
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			Errorf("%s: could not stop pid %d", s.repo.Name, cmd.Process.Pid)
		}
	}
	s.cmd = nil
}

// Running reports whether the app is up, for status output.
func (s *Supervisor) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cmd != nil && s.cmd.Process != nil
}

// --- the engine's side ---

// StartApps brings up every app the watcher is responsible for. Only the
// watcher does this: a one-off `syn pull` should not leave a bot running
// behind it after the command has exited.
func (e *Engine) StartApps(repos []*Repo) {
	e.appMu.Lock()
	defer e.appMu.Unlock()
	if e.apps == nil {
		e.apps = map[string]*Supervisor{}
	}
	for _, r := range repos {
		if r.App == nil || r.App.Command == "" {
			continue
		}
		if _, running := e.apps[r.Name]; running {
			continue
		}
		s := newSupervisor(r)
		e.apps[r.Name] = s
		s.Start()
	}
}

// StopApps shuts them down, so the watcher does not leave orphans behind.
func (e *Engine) StopApps() {
	e.appMu.Lock()
	defer e.appMu.Unlock()
	for name, s := range e.apps {
		s.Stop()
		delete(e.apps, name)
	}
}

// restartApp is called after a pull. It does nothing unless this process is
// the one supervising that app, which keeps a manual `syn pull` from
// interfering with a watcher running elsewhere.
func (e *Engine) restartApp(r *Repo, changed []string) {
	e.appMu.Lock()
	s := e.apps[r.Name]
	e.appMu.Unlock()
	if s == nil || r.App == nil || r.App.Command == "" {
		return
	}
	if !r.App.Wants(changed) {
		Debug("%s: app left alone, nothing under %v changed", r.Name, r.App.Paths)
		return
	}
	Info("%s: the code changed - restarting %s", r.Name, r.App.Command)
	s.Restart()
}

// RunAppInForeground runs the app attached to this terminal, for trying it
// out before handing it to the watcher. It returns when the app does.
func (e *Engine) RunAppInForeground(r *Repo) error {
	if r.App == nil || r.App.Command == "" {
		return fmt.Errorf("%s has no app configured\n"+
			"        set one:  syn app %s -- python bot.py", r.Name, r.Name)
	}
	s := newSupervisor(r)
	s.Start()
	if !s.Running() {
		return fmt.Errorf("it did not start")
	}
	Info("running; press Ctrl+C to stop")
	OnShutdown(s.Stop)
	<-s.done
	return nil
}
