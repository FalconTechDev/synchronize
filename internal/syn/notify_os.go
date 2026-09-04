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

	// The values travel in the environment: -Command ignores param() blocks,
	// and this way no quote in a repo name or commit subject can break the script.
	const script = `
$t = $env:SYN_TOAST_TITLE
$b = $env:SYN_TOAST_BODY
$img = $env:SYN_TOAST_IMAGE
$null = [Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType=WindowsRuntime]
$null = [Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom, ContentType=WindowsRuntime]
if ($img) {
    $template = [Windows.UI.Notifications.ToastTemplateType]::ToastImageAndText02
} else {
    $template = [Windows.UI.Notifications.ToastTemplateType]::ToastText02
}
$xml = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent($template)
$nodes = $xml.GetElementsByTagName("text")
$nodes.Item(0).AppendChild($xml.CreateTextNode($t)) | Out-Null
$nodes.Item(1).AppendChild($xml.CreateTextNode($b)) | Out-Null
if ($img) {
    $imgNode = $xml.GetElementsByTagName("image").Item(0)
    $imgNode.SetAttribute("src", $img)
    $imgNode.SetAttribute("alt", "synchronize")
}
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("synchronize").Show($toast)
`

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Env = append(os.Environ(),
		"SYN_TOAST_TITLE="+title,
		"SYN_TOAST_BODY="+body,
		"SYN_TOAST_IMAGE="+imgURI,
	)
	hideConsole(cmd)
	cmd.Start()
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
