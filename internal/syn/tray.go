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

func TrayMarkSyncing(v bool) { traySyncing.Store(v) }

func TrayMarkError() { trayErrors.Add(1) }

func TrayClearErrors() { trayErrors.Store(0) }

func RunTray(cfg *Config, engine *Engine) {
	runTrayWithStop(cfg, engine, make(chan struct{}))
}

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

