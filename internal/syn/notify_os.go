package syn

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func ToastNotify(title, body string) {
	switch runtime.GOOS {
	case "windows":
		toastWindows(title, body)
	case "darwin":
		toastDarwin(title, body)
	default:
		toastLinux(title, body)
	}
}

func toastWindows(title, body string) {
	
	
	logoPath := filepath.Join(Home(), "logo.png")
	imgURI := ""
	if _, err := os.Stat(logoPath); err == nil {
		imgURI = "file:///" + filepath.ToSlash(logoPath)
	}

	script := `
param($t,$b,$img)
$null = [Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType=WindowsRuntime]
$null = [Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom, ContentType=WindowsRuntime]
if ($img -ne '') {
    $template = [Windows.UI.Notifications.ToastTemplateType]::ToastImageAndText02
} else {
    $template = [Windows.UI.Notifications.ToastTemplateType]::ToastText02
}
$xml = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent($template)
$xml.GetElementsByTagName("text")[0].AppendChild($xml.CreateTextNode($t)) | Out-Null
$xml.GetElementsByTagName("text")[1].AppendChild($xml.CreateTextNode($b)) | Out-Null
if ($img -ne '') {
    $imgNode = $xml.GetElementsByTagName("image")[0]
    $imgNode.SetAttribute("src", $img)
    $imgNode.SetAttribute("alt", "logo")
}
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("synchronize").Show($toast)
`
	exec.Command("powershell", "-NoProfile", "-NonInteractive",
		"-Command", script, "-t", title, "-b", body, "-img", imgURI).Start()
}

func toastDarwin(title, body string) {
	safe := strings.ReplaceAll(body, `"`, `\"`)
	safeTitle := strings.ReplaceAll(title, `"`, `\"`)
	script := fmt.Sprintf(`display notification "%s" with title "%s"`, safe, safeTitle)
	exec.Command("osascript", "-e", script).Start()
}

func toastLinux(title, body string) {
	logoPath := filepath.Join(Home(), "logo.png")
	args := []string{"--app-name=synchronize", title, body}
	if _, err := os.Stat(logoPath); err == nil {
		args = append([]string{"--icon=" + logoPath}, args...)
	}
	exec.Command("notify-send", args...).Start()
}
