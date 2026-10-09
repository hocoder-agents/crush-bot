package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"

	"github.com/hocoder-agents/crush-bot/internal/config"
	"github.com/hocoder-agents/crush-bot/internal/crush"
	"github.com/hocoder-agents/crush-bot/internal/roster"
)

type sayDoneMsg struct {
	slug string
	err  error
}

func newChatInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "message this bot…"
	ti.Prompt = "> "
	return ti
}

func formatTranscript(lines []crush.Line, name string) string {
	if name == "" {
		name = "bot"
	}
	if len(lines) == 0 {
		return mutedStyle.Render("(empty session — type a line and press enter)")
	}
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		role := l.Role
		body := l.Text
		switch role {
		case "user":
			fmt.Fprintf(&b, "%s\n%s\n", keyStyle.Render("you"), body)
		case "assistant":
			fmt.Fprintf(&b, "%s\n%s\n", selStyle.Render(name), body)
		default:
			fmt.Fprintf(&b, "%s\n%s\n", mutedStyle.Render(role), body)
		}
	}
	return b.String()
}

func loadTranscript(root string, bot roster.Bot) (string, error) {
	p := config.ResolvePaths()
	cfg, err := config.Load(p)
	if err != nil {
		return "", err
	}
	bin, err := crush.LookPath(cfg.CrushPath)
	if err != nil {
		return "", err
	}
	home := roster.Home(root, bot.Slug)
	lines, err := crush.SessionTranscript(bin, home, filepath.Join(home, ".crush"), bot.CanonicalSessionID, 80)
	if err != nil {
		return "", err
	}
	return formatTranscript(lines, bot.Slug), nil
}

func runSay(root string, bot roster.Bot, prompt string) error {
	p := config.ResolvePaths()
	cfg, err := config.Load(p)
	if err != nil {
		return err
	}
	bin, err := crush.LookPath(cfg.CrushPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.TurnLockTimeout+2*time.Minute)
	defer cancel()
	_, err = crush.Run(ctx, crush.RunOpts{
		Bot:     bot,
		Root:    root,
		Bin:     bin,
		Kind:    "human_say",
		Prompt:  prompt,
		Timeout: cfg.TurnLockTimeout,
		Debug:   os.Getenv("CRUSHBOT_DEBUG") == "1",
		Yolo:    bot.Unattended == "yolo",
	})
	return err
}
