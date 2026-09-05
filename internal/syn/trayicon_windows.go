//go:build windows

package syn

import _ "embed"

// Windows wants a real .ico in the notification area; handed a PNG the shell
// refuses it and reports success, which is how "unable to set icon: The
// operation completed successfully" comes about.
//
//go:embed assets/tray.ico
var trayIconData []byte
