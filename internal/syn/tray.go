package syn

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"fyne.io/systray"
)

//go:embed assets/tray.png
var trayIconData []byte

var (
	trayErrors  atomic.Int32
	traySyncing atomic.Bool
)

// TrayMarkSyncing sets the syncing flag so the tray icon reflects it.
func TrayMarkSyncing(v bool) { traySyncing.Store(v) }

// TrayMarkError increments the error counter shown in the tray tooltip.
func TrayMarkError() { trayErrors.Add(1) }

// TrayClearErrors resets the error counter.
func TrayClearErrors() { trayErrors.Store(0) }

// RunTray starts the system tray as a standalone command (syn tray).
// Blocks on the main OS thread.
func RunTray(cfg *Config, engine *Engine) {
	runTrayWithStop(cfg, engine, make(chan struct{}))
}

// runTrayWithStop starts the tray; closing stop triggers a graceful shutdown.
// Must run on the main OS thread (systray requirement).
func runTrayWithStop(cfg *Config, engine *Engine, stop chan struct{}) {
	systray.Run(func() {
		systray.SetIcon(trayIconData)
		systray.SetTitle("syn")
		systray.SetTooltip("synchronize — auto git sync")

		mStatus := systray.AddMenuItem("✓ up to date", "Current sync status")
		mStatus.Disable()
		systray.AddSeparator()
		mPull := systray.AddMenuItem("Pull all", "Pull all repos now")
		mPush := systray.AddMenuItem("Push all", "Push all repos now")
		systray.AddSeparator()
		mLog := systray.AddMenuItem("Open log", "Open the log file")
		mHome := systray.AddMenuItem("Config folder", "Open ~/.synchronize")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit synchronize", "Stop the watcher and exit")

		// Status update ticker.
		go func() {
			for range time.Tick(2 * time.Second) {
				errs := trayErrors.Load()
				syncing := traySyncing.Load()
				switch {
				case errs > 0:
					label := fmt.Sprintf("⚠ %d error(s)", errs)
					mStatus.SetTitle(label)
					systray.SetTooltip("synchronize — " + label)
				case syncing:
					mStatus.SetTitle("⟳ syncing…")
					systray.SetTooltip("synchronize — syncing")
				default:
					mStatus.SetTitle("✓ up to date")
					systray.SetTooltip("synchronize — up to date")
				}
			}
		}()

		// Button handler.
		go func() {
			for {
				select {
				case <-mPull.ClickedCh:
					repos := cfg.EnabledRepos()
					go engine.RunAll(repos, engine.Pull)
				case <-mPush.ClickedCh:
					repos := cfg.EnabledRepos()
					go engine.RunAll(repos, engine.Push)
				case <-mLog.ClickedCh:
					openPath(LogPath())
				case <-mHome.ClickedCh:
					openPath(Home())
				case <-mQuit.ClickedCh:
					close(stop)
					// Give the watcher a moment to push, then quit.
					time.Sleep(3 * time.Second)
					systray.Quit()
					os.Exit(0)
				}
			}
		}()
	}, func() {})
}

func openPath(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", filepath.ToSlash(path))
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	cmd.Start()
}

