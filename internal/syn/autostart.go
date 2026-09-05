package syn

import (
	"os"
	"os/exec"
	"path/filepath"
)

func ExePath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return filepath.Abs(p)
}

// run is for the small OS tools that set autostart up - schtasks, launchctl,
// systemctl. Their output is quoted straight back at the user when something
// goes wrong, and on Windows they answer in the console code page, so it goes
// through the same decoding as a hook's output.
func run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return decodeConsole(out), err
}

// AutostartInstalled reports whether anything at all runs on its own.
func AutostartInstalled() bool { return LoginEnabled() || ShutdownEnabled() }

// InstallAutostart and UninstallAutostart are the older all-or-nothing names.
func InstallAutostart() error {
	if err := EnableLogin(true); err != nil {
		return err
	}
	return EnableShutdown()
}

func UninstallAutostart() error {
	if err := DisableLogin(); err != nil {
		return err
	}
	return DisableShutdown()
}
