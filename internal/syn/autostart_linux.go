//go:build linux

package syn

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const unitName = "synchronize.service"

func unitPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user", unitName), nil
}

// InstallAutostart writes a systemd --user unit. systemd starts it at login
// and sends SIGTERM at shutdown, which makes the watcher push before the
// machine goes down.
func InstallAutostart() error {
	exe, err := ExePath()
	if err != nil {
		return err
	}
	path, err := unitPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	unit := fmt.Sprintf(`[Unit]
Description=synchronize - keep git repositories in sync
After=network-online.target

[Service]
Type=simple
ExecStart=%s watch --all --quiet
ExecStopPost=%s push --all --quiet
Restart=on-failure
RestartSec=30
KillSignal=SIGTERM
TimeoutStopSec=120

[Install]
WantedBy=default.target
`, exe, exe)
	if err := os.WriteFile(path, []byte(unit), 0o644); err != nil {
		return err
	}
	Info("wrote %s", path)
	if out, err := run("systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %s", strings.TrimSpace(out))
	}
	if out, err := run("systemctl", "--user", "enable", "--now", unitName); err != nil {
		return fmt.Errorf("systemctl enable: %s", strings.TrimSpace(out))
	}
	Info("enabled %s (run `loginctl enable-linger $USER` if you want it without being logged in)", unitName)
	return nil
}

// UninstallAutostart disables and removes the unit.
func UninstallAutostart() error {
	_, _ = run("systemctl", "--user", "disable", "--now", unitName)
	path, err := unitPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	_, _ = run("systemctl", "--user", "daemon-reload")
	Info("removed %s", unitName)
	return nil
}

// AutostartStatus reports the unit state.
func AutostartStatus() string {
	out, _ := run("systemctl", "--user", "is-enabled", unitName)
	state := strings.TrimSpace(out)
	if state == "" {
		state = "not installed"
	}
	active, _ := run("systemctl", "--user", "is-active", unitName)
	return fmt.Sprintf("  %-28s %s (%s)\n", unitName, state, strings.TrimSpace(active))
}

// StartWatcherNow starts the unit.
func StartWatcherNow() error {
	if out, err := run("systemctl", "--user", "start", unitName); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(out))
	}
	return nil
}
