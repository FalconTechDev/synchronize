//go:build darwin

package syn

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const agentLabel = "dev.synchronize.watch"

func plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", agentLabel+".plist"), nil
}

func InstallAutostart() error {
	exe, err := ExePath()
	if err != nil {
		return err
	}
	path, err := plistPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>watch</string>
    <string>--all</string>
    <string>--quiet</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ExitTimeOut</key><integer>120</integer>
  <key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, agentLabel, exe, filepath.Join(Home(), "launchd.err.log"))
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return err
	}
	Info("wrote %s", path)
	_, _ = run("launchctl", "unload", path)
	if out, err := run("launchctl", "load", "-w", path); err != nil {
		return fmt.Errorf("launchctl load: %s", strings.TrimSpace(out))
	}
	Info("loaded LaunchAgent %s", agentLabel)
	return nil
}

func UninstallAutostart() error {
	path, err := plistPath()
	if err != nil {
		return err
	}
	_, _ = run("launchctl", "unload", "-w", path)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	Info("removed LaunchAgent %s", agentLabel)
	return nil
}

func AutostartStatus() string {
	out, err := run("launchctl", "list", agentLabel)
	if err != nil || strings.TrimSpace(out) == "" {
		return fmt.Sprintf("  %-28s not installed\n", agentLabel)
	}
	return fmt.Sprintf("  %-28s installed\n", agentLabel)
}

func StartWatcherNow() error {
	if out, err := run("launchctl", "start", agentLabel); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(out))
	}
	return nil
}
