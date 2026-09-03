package syn

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Watch runs the background sync loop. Each repo is synced at its own
// interval. An initial pull runs first; on SIGTERM/SIGINT everything is
// pushed before exit.
func (e *Engine) Watch(repos []*Repo) {
	e.watchLoop(repos, make(chan struct{}))
}

// WatchWithTray runs the sync loop in a background goroutine and starts the
// system tray on the calling (main OS) thread. The tray's Quit button
// gracefully stops the watcher.
func (e *Engine) WatchWithTray(repos []*Repo, cfg *Config) {
	stop := make(chan struct{})
	go e.watchLoop(repos, stop)
	// RunTray blocks until the user quits; it closes stop on exit.
	runTrayWithStop(cfg, e, stop)
}

func (e *Engine) watchLoop(repos []*Repo, externalStop <-chan struct{}) {
	Info("watcher started: %d repo(s)", len(repos))

	sigStop := make(chan os.Signal, 1)
	signal.Notify(sigStop, os.Interrupt, syscall.SIGTERM)

	// Initial pull so we start up to date.
	TrayMarkSyncing(true)
	e.RunAll(repos, e.Pull)
	TrayMarkSyncing(false)

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
