//go:build linux

package syn

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

const systemUnit = "synchronize.service"
const systemUnitPath = "/etc/systemd/system/" + systemUnit

// ServiceOptions are the answers a system service needs that a desktop
// autostart never has to ask: which account runs it, and where that account
// keeps its configuration.
type ServiceOptions struct {
	User string // the account the service runs as
	Home string // SYNCHRONIZE_HOME for that account
}

// ServiceSupported says whether this build can install a system service. The
// server story is a Linux one; a Windows server already has the scheduled
// task, and macOS is a workstation here.
func ServiceSupported() bool { return true }

func serviceDefaults(opts ServiceOptions) (ServiceOptions, error) {
	if opts.User == "" {
		// Under sudo the interesting account is the one that called it, not
		// root: its home is where the repositories and the token live.
		opts.User = os.Getenv("SUDO_USER")
	}
	if opts.User == "" {
		return opts, fmt.Errorf("which account should the service run as?\n" +
			"        pass it:  syn service install --as <user>\n" +
			"        it needs read and write on the repositories, and its own git identity")
	}
	u, err := user.Lookup(opts.User)
	if err != nil {
		return opts, fmt.Errorf("no such account %q on this machine", opts.User)
	}
	if opts.Home == "" {
		opts.Home = filepath.Join(u.HomeDir, ".synchronize")
	}
	return opts, nil
}

// InstallService writes a system-wide unit, so the watcher runs from boot
// without anybody logging in - which is the whole difference between a
// server and a desktop. The user unit that `syn auto` writes only runs
// while its owner has a session, unless lingering is switched on.
func InstallService(opts ServiceOptions) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("installing a system service needs root\n" +
			"        try:  sudo syn service install")
	}
	opts, err := serviceDefaults(opts)
	if err != nil {
		return err
	}
	exe, err := ExePath()
	if err != nil {
		return err
	}

	// No --quiet: on a server the log belongs in the journal, where
	// journalctl and everything built on it can see it. No ExecStopPost
	// push either - the watcher publishes on SIGTERM by itself, and a
	// deploy target has nothing to publish anyway.
	unit := fmt.Sprintf(`[Unit]
Description=synchronize - keep git repositories in sync
Documentation=https://github.com/FalconTechDev/synchronize
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=%s
Environment=SYNCHRONIZE_HOME=%s
ExecStart=%s watch --all --no-tray
Restart=always
RestartSec=30
KillSignal=SIGTERM
TimeoutStopSec=180
StandardOutput=journal
StandardError=journal

# It only ever needs the repositories it was told about.
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=read-only
ReadWritePaths=%s

[Install]
WantedBy=multi-user.target
`, opts.User, opts.Home, exe, strings.Join(servicePaths(opts), " "))

	if err := os.WriteFile(systemUnitPath, []byte(unit), 0o644); err != nil {
		return err
	}
	Info("wrote %s", systemUnitPath)
	if out, err := run("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %s", strings.TrimSpace(out))
	}
	if out, err := run("systemctl", "enable", "--now", systemUnit); err != nil {
		return fmt.Errorf("systemctl enable: %s", strings.TrimSpace(out))
	}
	Info("%s is running as %s, and will come back after a reboot", systemUnit, opts.User)
	Info("watch it with:  journalctl -u %s -f", systemUnit)
	// The sandbox lists the repository folders by name, so a repository
	// added later is not in it and the service cannot write to it. Better
	// said here than discovered as a permission error at three in the
	// morning.
	Info("after adding a repository, run this again so the service may write to it")
	return nil
}

// servicePaths are the directories the unit is allowed to write to.
// ProtectHome=read-only would otherwise stop the service touching the very
// repositories it exists to update.
func servicePaths(opts ServiceOptions) []string {
	paths := []string{opts.Home}
	if cfg, err := LoadConfig(); err == nil {
		for i := range cfg.Repos {
			paths = append(paths, cfg.Repos[i].Path)
		}
	}
	return paths
}

func UninstallService() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("removing a system service needs root\n" +
			"        try:  sudo syn service uninstall")
	}
	_, _ = run("systemctl", "disable", "--now", systemUnit)
	if err := os.Remove(systemUnitPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	_, _ = run("systemctl", "daemon-reload")
	Info("removed %s", systemUnit)
	return nil
}

func ServiceStatus() string {
	if _, err := os.Stat(systemUnitPath); os.IsNotExist(err) {
		return "not installed"
	}
	enabled, _ := run("systemctl", "is-enabled", systemUnit)
	active, _ := run("systemctl", "is-active", systemUnit)
	return fmt.Sprintf("%s (%s)", strings.TrimSpace(enabled), strings.TrimSpace(active))
}
