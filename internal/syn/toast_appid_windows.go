//go:build windows

package syn

import (
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/windows/registry"
)

const toastAppID = "FalconTechDev.synchronize"

var registerOnce sync.Once

// ensureToastAppID teaches Windows who we are. Without this registration a
// toast from an unpackaged program shows no icon and a placeholder name -
// the notification still appears, it just looks like it came from nowhere.
func ensureToastAppID() string {
	registerOnce.Do(func() {
		key, _, err := registry.CreateKey(registry.CURRENT_USER,
			`Software\Classes\AppUserModelId\`+toastAppID, registry.SET_VALUE)
		if err != nil {
			Debug("toast: could not register app id: %v", err)
			return
		}
		defer key.Close()

		if err := key.SetStringValue("DisplayName", "synchronize"); err != nil {
			Debug("toast: could not set display name: %v", err)
		}
		if icon := toastIconPath(); icon != "" {
			if err := key.SetStringValue("IconUri", icon); err != nil {
				Debug("toast: could not set icon: %v", err)
			}
		}
	})
	return toastAppID
}

func toastIconPath() string {
	for _, name := range []string{"logo.png", "logo.ico"} {
		p := filepath.Join(Home(), name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
