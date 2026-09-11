package mesh

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/hocoder-agents/crush-bot/internal/crush"
	"github.com/hocoder-agents/crush-bot/internal/group"
)

func GroupSay(id Identity, body string) CallResult {
	if err := id.Validate(); err != nil {
		return failReason(err)
	}
	if utf8.RuneCountInString(body) > MessageMaxChars {
		return CallResult{Reason: "message_too_long", Error: "line too long"}
	}
	turn, err := crush.ReadTurn(id.BotHome())
	if err != nil || turn.Kind != "group_round" || turn.GroupID == nil || *turn.GroupID == "" {
		return CallResult{Reason: "missing_config", Error: "group_say only in a group_round"}
	}
	gid := *turn.GroupID
	g, err := group.Load(id.Root, gid)
	if err != nil {
		return CallResult{Reason: "unknown", Error: err.Error()}
	}
	max := g.MaxMsgsPerSend
	if max <= 0 {
		max = 10
	}
	if err := bumpGroupSends(id.BotHome(), max); err != nil {
		return failReason(err)
	}
	f, err := os.OpenFile(group.SaysPath(id.Root, gid, id.Bot), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return CallResult{Reason: "runtime_offline", Error: err.Error()}
	}
	defer f.Close()
	if _, err := f.WriteString(strings.TrimRight(body, "\n") + "\n"); err != nil {
		return CallResult{Reason: "runtime_offline", Error: err.Error()}
	}
	return CallResult{Status: "queued"}
}

func GroupPass(id Identity) CallResult {
	if err := id.Validate(); err != nil {
		return failReason(err)
	}
	turn, err := crush.ReadTurn(id.BotHome())
	if err != nil || turn.Kind != "group_round" {
		return CallResult{Reason: "missing_config", Error: "group_pass only in a group_round"}
	}
	if err := os.WriteFile(group.PassFlag(id.BotHome()), []byte("1\n"), 0o600); err != nil {
		return CallResult{Reason: "runtime_offline", Error: err.Error()}
	}
	return CallResult{Status: "queued"}
}

func bumpGroupSends(botHome string, max int) error {
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
	if t.GroupSends >= max {
		return fmt.Errorf("fanout_limit")
	}
	t.GroupSends++
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
