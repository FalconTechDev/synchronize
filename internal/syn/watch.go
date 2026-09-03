package syn

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func (e *Engine) Watch(repos []*Repo) {
	e.watchLoop(repos, make(chan struct{}))
}

func (e *Engine) WatchWithTray(repos []*Repo, cfg *Config) {
	stop := make(chan struct{})
	go e.watchLoop(repos, stop)
	
	runTrayWithStop(cfg, e, stop)
}

func (e *Engine) watchLoop(repos []*Repo, externalStop <-chan struct{}) {
	Info("watcher started: %d repo(s)", len(repos))

	sigStop := make(chan os.Signal, 1)
	signal.Notify(sigStop, os.Interrupt, syscall.SIGTERM)

	
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
