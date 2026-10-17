package syn

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func (e *Engine) Watch(repos []*Repo) {
	e.watchLoop(repos, make(chan struct{}))
}

// WatchWithTray syncs in the background and puts an icon by the clock. The
// icon is the optional half: if there is no notification area to put it in,
// or systray cannot start in this session, the watching still has to happen,
// so the tray failing drops us to a plain watch rather than exiting.
func (e *Engine) WatchWithTray(repos []*Repo, cfg *Config) {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.watchLoop(repos, stop)
	}()

	if waitForTaskbar(3 * time.Minute) {
		runTrayWithStop(cfg, e, stop)
	} else {
		Info("no notification area appeared; watching without a tray icon")
	}

	// systray.Run comes back either because the user chose Quit - which
	// closed stop, and the loop is on its way out - or because it could not
	// start at all. Only in the first case is it time to leave.
	select {
	case <-stop:
	default:
		Info("tray unavailable; carrying on without it")
	}
	<-done
}

// WatchUntil is the watch loop for a GUI host: it never installs signal
// handlers and reports every sync through onResult.
func (e *Engine) WatchUntil(repos []*Repo, stop <-chan struct{}, onResult func(Result)) {
	report := func(res Result) {
		if onResult != nil {
			onResult(res)
		}
	}

	Info("watcher started: %d repo(s)", len(repos))
	for _, res := range e.RunAll(repos, e.Pull) {
		report(res)
	}

	tickers := make([]*time.Ticker, len(repos))
	for i, r := range repos {
		interval := time.Duration(e.Cfg.IntervalFor(r)) * time.Minute
		tickers[i] = time.NewTicker(interval)
		Info("  %s: every %s", r.Name, interval)
	}
	defer func() {
		for _, t := range tickers {
			t.Stop()
		}
	}()

	for {
		select {
		case <-stop:
			Info("watcher stopping: pushing local work first")
			for _, res := range e.RunAll(repos, e.Push) {
				report(res)
			}
			Info("watcher stopped")
			return
		case <-time.After(2 * time.Second):
		}

		for i, t := range tickers {
			select {
			case <-t.C:
				report(e.Sync(repos[i]))
			default:
			}
		}
	}
}

func (e *Engine) watchLoop(repos []*Repo, externalStop <-chan struct{}) {
	Info("watcher started: %d repo(s)", len(repos))

	sigStop := make(chan os.Signal, 1)
	signal.Notify(sigStop, os.Interrupt, syscall.SIGTERM)

	// Publishing has to finish before the machine goes down, otherwise the
	// work stays on this disk and the other machine never sees it.
	var shutdownOnce sync.Once
	OnShutdown(func() {
		shutdownOnce.Do(func() {
			HoldShutdown("synchronize is publishing your work")
			defer ReleaseShutdown()
			Info("shutdown requested: publishing before the machine goes down")
			e.RunAll(autoTargets(repos), e.Push)
			Info("everything published - letting the shutdown continue")
		})
	})

	TrayMarkSyncing(true)
	e.RunAll(autoTargets(repos), e.Pull)
	TrayMarkSyncing(false)

	// Apps come up after that first pull, so a machine that has been off
	// starts the code that is current rather than the code it went down on.
	e.StartApps(autoTargets(repos))
	defer e.StopApps()

	// Leave a note saying the watcher is here, so `syn status` can answer
	// "is it actually running" rather than only "is autostart switched on".
	beat := startHeartbeat(len(repos))
	defer beat.stop()

	type entry struct {
		repo     *Repo
		ticker   *time.Ticker
		channel  <-chan time.Time
		interval time.Duration
	}
	entries := make([]entry, len(repos))
	for i, r := range repos {
		interval := time.Duration(e.Cfg.IntervalFor(r)) * time.Minute
		t := time.NewTicker(interval)
		entries[i] = entry{r, t, t.C, interval}
		Info("  %s: every %s", r.Name, interval)
	}
	defer func() {
		for _, en := range entries {
			en.ticker.Stop()
		}
	}()

	for {
		select {
		case sig := <-sigStop:
			Info("watcher stopping (%s): pushing local work first", sig)
			TrayMarkSyncing(true)
			e.RunAll(autoTargets(repos), e.Push)
			TrayMarkSyncing(false)
			Info("watcher stopped")
			return
		case <-externalStop:
			Info("watcher stopping (tray quit): pushing local work first")
			TrayMarkSyncing(true)
			e.RunAll(autoTargets(repos), e.Push)
			TrayMarkSyncing(false)
			Info("watcher stopped")
			return
		default:
		}

		fired := false
		for i := range entries {
			en := &entries[i]
			select {
			case <-en.channel:
				// Ask the config again rather than trust the copy read at
				// startup: the pause may have been typed a minute ago.
				e.reloadAutoState([]*Repo{en.repo})
				if !en.repo.SyncsByItself() {
					Debug("%s: left alone, automatic syncing is off for it", en.repo.Name)
					continue
				}
				// Pick up an interval changed since the watcher started,
				// rather than make the change wait for a restart nobody
				// knows is needed.
				if want := time.Duration(e.Cfg.IntervalFor(en.repo)) * time.Minute; want != en.interval {
					Info("%s: now checked every %s", en.repo.Name, want)
					en.ticker.Reset(want)
					en.interval = want
				}
				TrayMarkSyncing(true)
				res := e.Sync(en.repo)
				TrayMarkSyncing(false)
				if res.Err != nil || res.Conflict != nil {
					TrayMarkError()
				} else {
					TrayClearErrors()
					if e.Cfg.NotifySync && (res.Pulled > 0 || res.Pushed > 0) {
						ToastNotify("synchronize",
							fmt.Sprintf("%s: pulled %d, pushed %d", en.repo.Name, res.Pulled, res.Pushed))
					}
				}
				fired = true
			default:
			}
		}
		beat.beat()
		if !fired {
			time.Sleep(5 * time.Second)
		}
	}
}

// autoTargets drops the repositories automatic syncing is supposed to leave
// alone. `syn pause` and `syn auto <repo> off` promise exactly this, and
// until now only the tray and the desktop app kept the promise - the watcher
// synced everything it was given.
func autoTargets(repos []*Repo) []*Repo {
	kept := make([]*Repo, 0, len(repos))
	for _, r := range repos {
		if r.SyncsByItself() {
			kept = append(kept, r)
		}
	}
	return kept
}

// reloadAutoState re-reads the switches from the config file, so pausing a
// repository takes effect on a watcher that is already running. The watcher
// lives for days; a pause that only applied after a restart would not be a
// pause at all.
//
// Only the two switches are taken. Paths, hooks and intervals are left as
// they were, because changing those under a running sync is a different and
// riskier thing than deciding not to sync at all.
func (e *Engine) reloadAutoState(repos []*Repo) {
	fresh, err := LoadConfig()
	if err != nil {
		Debug("could not re-read the config: %v", err)
		return
	}
	for _, r := range repos {
		f := fresh.Find(r.Name)
		if f == nil {
			continue // removed from the config; the next restart drops it
		}
		r.Enabled = f.Enabled
		r.PausedUntil = f.PausedUntil
		r.IntervalMinutes = f.IntervalMinutes
	}
	// The global default can move too, and a watcher that only noticed the
	// per-repository one would be half right in a confusing way.
	if e.Cfg != nil && fresh.IntervalMinutes > 0 {
		e.Cfg.IntervalMinutes = fresh.IntervalMinutes
	}
}
