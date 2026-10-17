package main

import (
	"fmt"
	"strings"

	"github.com/FalconTechDev/synchronize/internal/syn"
)

// cmdConfigTelegram sets up the Telegram alerts. The sending side has worked
// all along; there was simply no way to switch it on short of editing
// config.json by hand, and the command said "coming soon" instead of saying
// that.
func cmdConfigTelegram(cfg *syn.Config, rest []string) error {
	if len(rest) == 0 {
		return showTelegram(cfg)
	}

	switch rest[0] {
	case "off", "clear", "disable":
		cfg.Telegram = syn.Telegram{}
		if err := cfg.Save(); err != nil {
			return err
		}
		syn.Info("Telegram alerts switched off")
		return nil
	}

	if len(rest) < 2 {
		return fmt.Errorf("usage: syn config telegram <bot-token> <chat-id>\n" +
			"        the token comes from @BotFather; the chat id from @userinfobot\n" +
			"        switch it off again with:  syn config telegram off")
	}
	token, chat := strings.TrimSpace(rest[0]), strings.TrimSpace(rest[1])
	if token == "" || chat == "" {
		return fmt.Errorf("both the bot token and the chat id are needed")
	}
	cfg.Telegram = syn.Telegram{Token: token, ChatID: chat}
	if err := cfg.Save(); err != nil {
		return err
	}
	// The token is a password for the bot, so it is not echoed back.
	syn.Info("Telegram alerts will go to chat %s", chat)
	syn.Info("       check it works:  syn config test")
	return nil
}

func showTelegram(cfg *syn.Config) error {
	if cfg.Telegram.Token == "" || cfg.Telegram.ChatID == "" {
		fmt.Println("Telegram alerts are off.")
		fmt.Println()
		fmt.Println("  syn config telegram <bot-token> <chat-id>")
		fmt.Println()
		fmt.Println("The token comes from @BotFather, the chat id from @userinfobot.")
		fmt.Println("Alerts follow the `notify` setting - conflicts only, by default.")
		return nil
	}
	fmt.Printf("Telegram alerts go to chat %s\n", cfg.Telegram.ChatID)
	fmt.Printf("  sending on:  %s\n", orDefaultText(cfg.Notify, "conflict"))
	fmt.Println("  test it:     syn config test")
	fmt.Println("  switch off:  syn config telegram off")
	return nil
}

// cmdConfigTest sends one alert through whatever is configured, so you find
// out that notifications are broken now rather than during a conflict.
func cmdConfigTest(cfg *syn.Config) error {
	if cfg.Notify == "never" {
		return fmt.Errorf("notifications are switched off (notify = never)\n" +
			"        turn them on:  syn config set notify conflict")
	}
	syn.Info("sending a test alert...")
	syn.NewNotifier(cfg).Send("conflict", syn.Alert{
		Title: "synchronize",
		Short: "Test alert - everything is wired up.",
		Full: "synchronize: test alert\n\nIf you can read this, conflict alerts will reach you.\n" +
			"Sent by `syn config test`.",
	})
	fmt.Println()
	fmt.Println("  Sent. A desktop notification should have appeared just now.")
	if cfg.Telegram.Token != "" && cfg.Telegram.ChatID != "" {
		fmt.Printf("  Telegram: check chat %s.\n", cfg.Telegram.ChatID)
		fmt.Println("  If nothing arrived, the log says why:  syn log 20")
	} else {
		fmt.Println("  Telegram is not configured:  syn config telegram <token> <chat-id>")
	}
	return nil
}

func orDefaultText(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
