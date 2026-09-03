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

func (n *Notifier) Send(level, msg string) {
	
	if level == "conflict" {
		lines := strings.SplitN(msg, "\n", 3)
		title := "synchronize: конфликт"
		body := lines[0]
		if len(lines) > 1 {
			body = strings.Join(lines[1:], " ")
		}
		ToastNotify(title, body)
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
		"text":                     {msg},
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
