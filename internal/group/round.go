package group

import (
	"context"
	"os"
	"strings"

	"github.com/hocoder-agents/crush-bot/internal/config"
	"github.com/hocoder-agents/crush-bot/internal/crush"
	"github.com/hocoder-agents/crush-bot/internal/lock"
	"github.com/hocoder-agents/crush-bot/internal/roster"
)

func TailPrompt(home, id string, n int) string {
	lines, _ := ReadTranscript(home, id)
	if n <= 0 {
		n = 50
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	var b strings.Builder
	b.WriteString("You are in a crushbot group room. Room discussion belongs in public group_say lines - deliberate here so the operator sees it; message_bot is disabled while the room is in session and anything a room member must see has to be a group_say line. group_pass to skip.\n\nTranscript:\n")
	for _, l := range lines {
		b.WriteString(l.From + " [" + l.Kind + "]: " + l.Body + "\n")
	}
	return b.String()
}

func ensureSession(ctx context.Context, cfg config.Config, bin, root string, bot roster.Bot, gid string, held *lock.Lock) (string, error) {
	if bot.GroupSessions == nil {
		bot.GroupSessions = map[string]string{}
	}
	if id := bot.GroupSessions[gid]; id != "" {
		return id, nil
	}
	home := roster.Home(root, bot.Slug)
	_, err := crush.Run(ctx, crush.RunOpts{
		Bot: bot, Root: root, Bin: bin, Timeout: cfg.TurnLockTimeout,
		Kind: "group_round", NoSession: true, HeldLock: held,
		Prompt: "You are joining group " + gid + ". Stay in character.",
	})
	if err != nil {
		return "", err
	}
	meta, err := crush.SessionLast(bin, home, home+"/.crush")
	if err != nil {
		return "", err
	}
	_ = crush.SessionRename(bin, home, home+"/.crush", meta.UUID, "group:"+gid)
	bot.GroupSessions[gid] = meta.UUID
	if err := roster.Save(root, bot); err != nil {
		return "", err
	}
	return meta.UUID, nil
}

// Round runs one serial round for in-scope members. Returns whether everyone passed.
func Round(ctx context.Context, cfg config.Config, bin, root string, g Group, round int, scope []string) (allPass bool, err error) {
	allPass = true
	for _, slug := range scope {
		bot, err := roster.Load(root, slug)
		if err != nil {
			return false, err
		}
		home := roster.Home(root, slug)
		// Wait for the lock instead of instant-skipping: daemon DM wakes and
		// group rounds legitimately contend, and a skipped member looks like
		// the room ignoring them. The timeout bounds the wait.
		l, err := lock.Acquire(crush.LockPath(home), cfg.TurnLockTimeout, false)
		if err != nil {
			_ = AppendLine(root, g.ID, Line{Round: round, From: slug, Kind: "pass", Body: "skipped: target_busy", Pass: true})
			continue
		}
		sid, err := ensureSession(ctx, cfg, bin, root, bot, g.ID, l)
		if err != nil {
			l.Unlock()
			return false, err
		}
		_ = os.Remove(PassFlag(home))
		_ = os.Remove(SaysPath(root, g.ID, slug))
		_, runErr := crush.Run(ctx, crush.RunOpts{
			Bot: bot, Root: root, Bin: bin, Kind: "group_round",
			Prompt: TailPrompt(root, g.ID, 50), Timeout: cfg.TurnLockTimeout,
			SessionID: sid, HeldLock: l, MaxHops: cfg.MaxHops, GroupID: g.ID,
		})
		l.Unlock()
		if runErr != nil {
			allPass = false
			_ = AppendLine(root, g.ID, Line{Round: round, From: slug, Kind: "system", Body: "wake failed: " + runErr.Error()})
			continue
		}
		if _, err := os.Stat(PassFlag(home)); err == nil {
			_ = AppendLine(root, g.ID, Line{Round: round, From: slug, Kind: "pass", Body: "PASS", Pass: true})
			_ = os.Remove(PassFlag(home))
			continue
		}
		says := readSays(root, g.ID, slug)
		if len(says) > 0 {
			allPass = false
			for _, s := range says {
				_ = AppendLine(root, g.ID, Line{Round: round, From: slug, Kind: "line", Body: s, Mentions: Mentions(s, g.Members)})
			}
			_ = os.Remove(SaysPath(root, g.ID, slug))
			continue
		}
		text, _ := crush.LastAssistant(bin, home, home+"/.crush", sid, 4096)
		if strings.TrimSpace(text) == "" {
			_ = AppendLine(root, g.ID, Line{Round: round, From: slug, Kind: "pass", Body: "PASS", Pass: true})
			continue
		}
		allPass = false
		_ = AppendLine(root, g.ID, Line{Round: round, From: slug, Kind: "line", Body: text, Mentions: Mentions(text, g.Members)})
	}
	return allPass, nil
}

func RunUntilSettle(ctx context.Context, cfg config.Config, bin, root string, g Group, userLine string) error {
	mentions := Mentions(userLine, g.Members)
	scope := g.Members
	if len(mentions) > 0 {
		scope = mentions
	}
	_ = AppendLine(root, g.ID, Line{Round: 0, From: "user", Kind: "line", Body: userLine, Mentions: mentions})
	for r := 1; r <= g.MaxRounds; r++ {
		all, err := Round(ctx, cfg, bin, root, g, r, scope)
		if err != nil {
			return err
		}
		if all {
			return nil
		}
		lines, _ := ReadTranscript(root, g.ID)
		cascade := 0
		for _, l := range lines {
			if l.Round == r && l.Kind == "line" {
				cascade++
			}
		}
		if cascade >= g.MaxMsgsPerSend {
			return nil
		}
	}
	return nil
}

func readSays(home, gid, slug string) []string {
	b, err := os.ReadFile(SaysPath(home, gid, slug))
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
