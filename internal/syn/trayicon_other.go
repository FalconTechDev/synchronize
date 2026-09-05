//go:build !windows

package syn

import _ "embed"

//go:embed assets/tray.png
var trayIconData []byte
