package group

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hocoder-agents/crush-bot/internal/crush"
)

// Settle rounds are executed by the daemon, which outlives TUIs: a
// client enqueues a request file in the room dir and the daemon picks
// it up on its next sweep. If the daemon dies mid-round, recovery (see
// FlushStaleSays) salvages what the bots said.

type RoundRequest struct {
	Line string `json:"line"`
}

func RequestPath(home, id string) string {
	return filepath.Join(Dir(home, id), "request.json")
}

// EnqueueRequest atomically drops a round request for the daemon.
func EnqueueRequest(home, id, line string) error {
	if strings.TrimSpace(line) == "" {
		return fmt.Errorf("empty round request")
	}
	if _, err := Load(home, id); err != nil {
		return err
	}
	b, err := json.Marshal(RoundRequest{Line: line})
	if err != nil {
		return err
	}
	tmp := RequestPath(home, id) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, RequestPath(home, id))
}

// RoomBusy reports whether any room member is mid group_round turn.
func RoomBusy(root, id string, members []string) bool {
	for _, m := range members {
		turn, err := crush.ReadTurn(filepath.Join(root, "bots", m))
		if err != nil {
			continue
		}
		if turn.Kind != "group_round" || turn.GroupID == nil || *turn.GroupID != id {
			continue
		}
		if crush.PIDAlive(turn.CrushPID) {
			return true
		}
	}
	return false
}

// FlushStaleSays collects says files whose bot is no longer running a
// round (orphaned turns, daemon restarts) into the room transcript, so
// interrupted rounds still deliver what bots said. Returns lines moved.
func FlushStaleSays(root, id string) int {
	entries, err := os.ReadDir(Dir(root, id))
	if err != nil {
		return 0
	}
	round := 0
	if existing, err := ReadTranscript(root, id); err == nil {
		for _, l := range existing {
			if l.Round > round {
				round = l.Round
			}
		}
	}
	n := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "says-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		slug := strings.TrimSuffix(strings.TrimPrefix(name, "says-"), ".jsonl")
		if liveGroupRound(root, id, slug) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(Dir(root, id), name))
		if err != nil {
			continue
		}
		_ = os.Remove(filepath.Join(Dir(root, id), name))
		for _, l := range strings.Split(string(raw), "\n") {
			body := strings.TrimSpace(l)
			if body == "" {
				continue
			}
			if AppendLine(root, id, Line{Round: round, From: slug, Kind: "line", Body: body}) == nil {
				n++
			}
		}
	}
	return n
}

// liveGroupRound reports whether slug has a live group_round turn for id.
func liveGroupRound(root, id, slug string) bool {
	turn, err := crush.ReadTurn(filepath.Join(root, "bots", slug))
	if err != nil {
		return false
	}
	if turn.Kind != "group_round" || turn.GroupID == nil || *turn.GroupID != id {
		return false
	}
	return crush.PIDAlive(turn.CrushPID)
}
