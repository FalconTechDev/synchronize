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

func InstallAutostart() error {
	exe, err := ExePath()
	if err != nil {
		return err
	}
	if err := registerTask(taskWatch,
		"synchronize: keep repositories in sync with their remotes",
		logonTrigger, exe, "watch --all --quiet"); err != nil {
		return err
	}
	Info("registered scheduled task %q (runs at logon)", taskWatch)

	if err := registerTask(taskShutdown,
		"synchronize: push local commits when Windows shuts down",
		shutdownTrigger, exe, "push --all --quiet"); err != nil {
		return err
	}
	Info("registered scheduled task %q (runs on shutdown)", taskShutdown)
	return nil
}

func UninstallAutostart() error {
	var firstErr error
	for _, name := range []string{taskWatch, taskShutdown} {
		if out, err := run("schtasks", "/Delete", "/TN", name, "/F"); err != nil {
			if !strings.Contains(out, "cannot find") && !strings.Contains(out, "не найден") {
				Warn("could not remove task %q: %s", name, strings.TrimSpace(out))
				if firstErr == nil {
					firstErr = err
				}
			}
			continue
		}
		Info("removed scheduled task %q", name)
	}
	return firstErr
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
