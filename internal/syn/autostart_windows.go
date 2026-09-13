//go:build windows

package syn

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

const (
	taskWatch    = "Synchronize Watch"
	taskShutdown = "Synchronize Shutdown Push"
)

const logonTrigger = `<LogonTrigger><Enabled>true</Enabled></LogonTrigger>`

const shutdownTrigger = `<EventTrigger><Enabled>true</Enabled><Subscription>&lt;QueryList&gt;&lt;Query Id="0" Path="System"&gt;&lt;Select Path="System"&gt;*[System[Provider[@Name='User32'] and EventID=1074]]&lt;/Select&gt;&lt;/Query&gt;&lt;/QueryList&gt;</Subscription></EventTrigger>`

func taskXML(desc, trigger, exe, args string) string {
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>` + desc + `</Description>
    <Author>synchronize</Author>
  </RegistrationInfo>
  <Triggers>` + trigger + `</Triggers>
  <Principals>
    <Principal id="Author">
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <IdleSettings>
      <StopOnIdleEnd>false</StopOnIdleEnd>
      <RestartOnIdle>false</RestartOnIdle>
    </IdleSettings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>true</Hidden>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>7</Priority>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>` + xmlEscape(exe) + `</Command>
      <Arguments>` + xmlEscape(args) + `</Arguments>
    </Exec>
  </Actions>
</Task>`
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

func writeUTF16(path, content string) error {
	units := utf16.Encode([]rune(content))
	buf := make([]byte, 0, len(units)*2+2)
	buf = append(buf, 0xFF, 0xFE)
	var tmp [2]byte
	for _, u := range units {
		binary.LittleEndian.PutUint16(tmp[:], u)
		buf = append(buf, tmp[0], tmp[1])
	}
	return os.WriteFile(path, buf, 0o600)
}

func registerTask(name, desc, trigger, exe, args string) error {
	xmlPath := filepath.Join(os.TempDir(), sanitizeRef(name)+".xml")
	if err := writeUTF16(xmlPath, taskXML(desc, trigger, exe, args)); err != nil {
		return err
	}
	defer os.Remove(xmlPath)
	out, err := run("schtasks", "/Create", "/TN", name, "/XML", xmlPath, "/F")
	if err != nil {
		return fmt.Errorf("schtasks failed for %q: %s", name, strings.TrimSpace(out))
	}
	return nil
}

// The two scheduled tasks are separate switches: one syncs when you log in,
// the other publishes when the machine goes down. Whether the login task
// keeps watching on a timer, or syncs once and exits, is a third choice.
func EnableLogin(keepWatching bool) error {
	exe, err := autostartExe()
	if err != nil {
		return err
	}
	args, desc := "sync --all --quiet", "synchronize: sync once when you log in"
	if keepWatching {
		args = "watch --all --quiet"
		desc = "synchronize: keep repositories in sync while you are logged in"
	}
	if err := registerTask(taskWatch, desc, logonTrigger, exe, args); err != nil {
		return err
	}
	return nil
}

func DisableLogin() error { return removeTask(taskWatch) }

func EnableShutdown() error {
	exe, err := autostartExe()
	if err != nil {
		return err
	}
	return registerTask(taskShutdown,
		"synchronize: publish local commits when Windows shuts down",
		shutdownTrigger, exe, "push --all --quiet")
}

func DisableShutdown() error { return removeTask(taskShutdown) }

func LoginEnabled() bool    { return taskExists(taskWatch) }
func ShutdownEnabled() bool { return taskExists(taskShutdown) }

func taskExists(name string) bool {
	_, err := run("schtasks", "/Query", "/TN", name)
	return err == nil
}

func removeTask(name string) error {
	// Ask first: schtasks reports "not found" in the system language, so
	// matching its text would only work on an English Windows.
	if !taskExists(name) {
		return nil
	}
	if out, err := run("schtasks", "/Delete", "/TN", name, "/F"); err != nil {
		return fmt.Errorf("could not remove task %q: %s", name, strings.TrimSpace(out))
	}
	return nil
}

func AutostartStatus() string {
	var b strings.Builder
	for _, name := range []string{taskWatch, taskShutdown} {
		if _, err := run("schtasks", "/Query", "/TN", name); err != nil {
			fmt.Fprintf(&b, "    %-28s not installed\n", name)
		} else {
			fmt.Fprintf(&b, "    %-28s installed\n", name)
		}
	}
	return b.String()
}

func StartWatcherNow() error {
	out, err := run("schtasks", "/Run", "/TN", taskWatch)
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(out))
	}
	return nil
}

// autostartExe is the binary the scheduler should run.
//
// syn.exe is a console program, so Windows gives it a console before any of
// our code runs - and on Windows 11, where the default host is Windows
// Terminal, that means a window on screen that we cannot take back. The
// Hidden flag in the task definition does not help: it hides the task in the
// scheduler's list, not the window.
//
// synw.exe is the same program linked for the GUI subsystem, so no console is
// ever created for it and there is nothing to hide. It is installed beside
// syn.exe; if it is missing - a build from source, say - fall back rather
// than refuse to set autostart up at all.
func autostartExe() (string, error) {
	exe, err := ExePath()
	if err != nil {
		return "", err
	}
	windowless := filepath.Join(filepath.Dir(exe), "synw.exe")
	if _, err := os.Stat(windowless); err == nil {
		return windowless, nil
	}
	Debug("synw.exe not found beside %s; autostart will show a console window", exe)
	return exe, nil
}
