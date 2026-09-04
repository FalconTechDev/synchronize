package syn

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Notifier struct {
	cfg *Config
}

func NewNotifier(cfg *Config) *Notifier { return &Notifier{cfg: cfg} }

// Alert carries the two shapes a notification needs: a toast has room for
// about two short lines, while the Telegram message can hold everything.
type Alert struct {
	Title string
	Short string
	Full  string
}

func (n *Notifier) Send(level string, a Alert) {
	if level == "conflict" {
		ToastNotify(a.Title, a.Short)
	}
	if n == nil || n.cfg == nil {
		return
	}
	switch n.cfg.Notify {
	case "never", "":
		if n.cfg.Notify == "never" {
			return
		}
	case "conflict":
		if level != "conflict" {
			return
		}
	}
	tg := n.cfg.Telegram
	if tg.Token == "" || tg.ChatID == "" {
		Debug("notification skipped: telegram not configured")
		return
	}
	endpoint := "https://api.telegram.org/bot" + tg.Token + "/sendMessage"
	form := url.Values{
		"chat_id":                  {tg.ChatID},
		"text":                     {a.Full},
		"disable_web_page_preview": {"true"},
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(endpoint, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		Warn("telegram notification failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		Warn("telegram notification rejected with status %s", resp.Status)
	}
}
