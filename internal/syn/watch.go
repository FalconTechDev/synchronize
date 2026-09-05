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
			e.RunAll(repos, e.Push)
			Info("everything published - letting the shutdown continue")
		})
	})

	TrayMarkSyncing(true)
	e.RunAll(repos, e.Pull)
	TrayMarkSyncing(false)

	// Apps come up after that first pull, so a machine that has been off
	// starts the code that is current rather than the code it went down on.
	e.StartApps(repos)
	defer e.StopApps()

	type entry struct {
		repo    *Repo
		ticker  *time.Ticker
		channel <-chan time.Time
	}
	entries := make([]entry, len(repos))
	for i, r := range repos {
		interval := time.Duration(e.Cfg.IntervalFor(r)) * time.Minute
		t := time.NewTicker(interval)
		entries[i] = entry{r, t, t.C}
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
			e.RunAll(repos, e.Push)
			TrayMarkSyncing(false)
			Info("watcher stopped")
			return
		case <-externalStop:
			Info("watcher stopping (tray quit): pushing local work first")
			TrayMarkSyncing(true)
			e.RunAll(repos, e.Push)
			TrayMarkSyncing(false)
			Info("watcher stopped")
			return
		default:
		}

		fired := false
		for _, en := range entries {
			select {
			case <-en.channel:
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
		if !fired {
			time.Sleep(5 * time.Second)
		}
	}
}
