package mesh

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/hocoder-agents/crush-bot/internal/crush"
	"github.com/hocoder-agents/crush-bot/internal/envelope"
	"github.com/hocoder-agents/crush-bot/internal/roster"
)

const MessageMaxChars = 16000

type CallResult struct {
	Status string `json:"status,omitempty"`
	ID     string `json:"id,omitempty"`
	To     string `json:"to,omitempty"`
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
}

type Identity struct {
	Root    string
	Bot     string
	DataDir string
	Cwd     string
	Token   string
}

func (id Identity) BotHome() string {
	return roster.Home(id.Root, id.Bot)
}

func (id Identity) Validate() error {
	if id.Root == "" || id.Bot == "" {
		return fmt.Errorf("missing_config: CRUSHBOT_HOME/CRUSHBOT_BOT")
	}
	if !roster.ValidSlug(id.Bot) || !roster.Exists(id.Root, id.Bot) {
		return fmt.Errorf("unknown_bot")
	}
	wantHome, err := filepath.EvalSymlinks(id.BotHome())
	if err != nil {
		wantHome = id.BotHome()
	}
	if id.Cwd != "" {
		cwd, err := filepath.EvalSymlinks(id.Cwd)
		if err != nil {
			cwd = id.Cwd
		}
		if cwd != wantHome {
			return fmt.Errorf("missing_config: cwd is not bot home")
		}
	}
	if id.DataDir != "" {
		want := filepath.Join(id.BotHome(), ".crush")
		got, err := filepath.EvalSymlinks(id.DataDir)
		if err != nil {
			got = id.DataDir
		}
		want2, err := filepath.EvalSymlinks(want)
		if err != nil {
			want2 = want
		}
		if got != want2 {
			return fmt.Errorf("missing_config: CRUSHBOT_DATA_DIR mismatch")
		}
	}
	tokenPath := filepath.Join(id.BotHome(), ".mcp_token")
	b, err := os.ReadFile(tokenPath)
	if err != nil {
		return fmt.Errorf("missing_config: token")
	}
	if id.Token == "" || strings.TrimSpace(string(b)) != strings.TrimSpace(id.Token) {
		return fmt.Errorf("missing_config: bad token")
	}
	return nil
}

func DaemonLive(root string) bool {
	b, err := os.ReadFile(filepath.Join(root, "daemon.pid"))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func MessageBot(id Identity, target, message string) CallResult {
	if err := id.Validate(); err != nil {
		return failReason(err)
	}
	target = strings.TrimPrefix(strings.TrimSpace(target), "@")
	if utf8.RuneCountInString(message) > MessageMaxChars {
		return CallResult{Reason: "message_too_long", Error: "message exceeds 16000 characters"}
	}
	if !roster.Exists(id.Root, target) {
		return CallResult{Reason: "unknown_bot", Error: "unknown bot " + target}
	}
	if target == id.Bot {
		return CallResult{Reason: "self_message", Error: "cannot message self"}
	}
	turn, err := crush.ReadTurn(id.BotHome())
	if err != nil {
		return CallResult{Reason: "missing_config", Error: "no turn context; use crushbot say/chat/daemon"}
	}
	// Room rounds are public by design: a DM mid-round continues the
	// conversation invisibly (daemon wakes the target later, outside the
	// transcript). Block it with a nudge instead of letting the room
	// silently empty out.
	if turn.Kind == "group_round" {
		return CallResult{Reason: "room_dm_blocked", Error: "you are in a group room; DMs are invisible to the room - post with group_say"}
	}
	hop := turn.InboundHop + 1
	if hop > turn.MaxHops && turn.MaxHops > 0 {
		return CallResult{Reason: "hop_limit", Error: "hop exceeds max"}
	}
	trace := append([]string{}, turn.Trace...)
	trace = append(trace, turn.Bot)
	if err := bumpSends(id.BotHome(), turn.MaxSends); err != nil {
		return failReason(err)
	}
	env := envelope.Envelope{
		Kind:        "dm",
		From:        id.Bot,
		To:          target,
		Hop:         hop,
		Trace:       trace,
		Body:        message,
		Attribution: fmt.Sprintf("Message from %s (@%s):", id.Bot, id.Bot),
	}
	if turn.ParentID != "" {
		p := turn.ParentID
		env.ParentID = &p
	}
	eid, err := envelope.WritePending(roster.Home(id.Root, target), env)
	if err != nil {
		return CallResult{Reason: "runtime_offline", Error: err.Error()}
	}
	st := "queued"
	if DaemonLive(id.Root) {
		st = "sent"
	}
	return CallResult{Status: st, ID: eid, To: target}
}

func RosterList(id Identity) (any, CallResult) {
	if err := id.Validate(); err != nil {
		return nil, failReason(err)
	}
	bots, err := roster.List(id.Root, true)
	if err != nil {
		return nil, CallResult{Reason: "unknown", Error: err.Error()}
	}
	type row struct {
		Slug        string `json:"slug"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Hidden      bool   `json:"hidden"`
	}
	var out []row
	for _, b := range bots {
		out = append(out, row{b.Slug, b.Title, b.Description, b.Hidden})
	}
	return out, CallResult{Status: "ok"}
}

func Escalate(id Identity, summary string) CallResult {
	if err := id.Validate(); err != nil {
		return failReason(err)
	}
	path := filepath.Join(id.Root, "needs_you.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return CallResult{Reason: "runtime_offline", Error: err.Error()}
	}
	defer f.Close()
	rec := map[string]any{
		"ts":      time.Now().UTC().Format(time.RFC3339),
		"bot":     id.Bot,
		"summary": summary,
	}
	b, _ := json.Marshal(rec)
	if _, err := f.Write(append(b, '\n')); err != nil {
		return CallResult{Reason: "runtime_offline", Error: err.Error()}
	}
	return CallResult{Status: "sent"}
}

func bumpSends(botHome string, max int) error {
	path := crush.TurnPath(botHome)
	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("missing_config: turn.json")
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	var t crush.Turn
	if err := json.NewDecoder(f).Decode(&t); err != nil {
		return err
	}
	if max <= 0 {
		max = t.MaxSends
	}
	if t.Sends >= max {
		return fmt.Errorf("fanout_limit")
	}
	t.Sends++
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(t)
}

func failReason(err error) CallResult {
	msg := err.Error()
	reason := "unknown"
	for _, r := range []string{
		"unknown_bot", "self_message", "hop_limit", "fanout_limit",
		"message_too_long", "missing_config", "runtime_offline",
	} {
		if strings.Contains(msg, r) {
			reason = r
			break
		}
	}
	return CallResult{Reason: reason, Error: msg}
}
