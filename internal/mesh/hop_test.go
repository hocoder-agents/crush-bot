package mesh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hocoder-agents/crush-bot/internal/crush"
	"github.com/hocoder-agents/crush-bot/internal/protocol"
	"github.com/hocoder-agents/crush-bot/internal/roster"
)

func setupPair(t *testing.T) (root string, a, b roster.Bot, idA Identity) {
	t.Helper()
	root = t.TempDir()
	os.MkdirAll(filepath.Join(root, "bots"), 0o700)
	var err error
	a, _, err = roster.Spawn(root, roster.SpawnOpts{Slug: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	b, _, err = roster.Spawn(root, roster.SpawnOpts{Slug: "beta"})
	if err != nil {
		t.Fatal(err)
	}
	if err := protocol.Write(protocol.Options{Root: root, Bot: a, Teammates: []roster.Bot{a, b}}); err != nil {
		t.Fatal(err)
	}
	if err := protocol.Write(protocol.Options{Root: root, Bot: b, Teammates: []roster.Bot{a, b}}); err != nil {
		t.Fatal(err)
	}
	tok, _ := os.ReadFile(filepath.Join(roster.Home(root, "alpha"), ".mcp_token"))
	idA = Identity{
		Root:    root,
		Bot:     "alpha",
		DataDir: filepath.Join(roster.Home(root, "alpha"), ".crush"),
		Cwd:     roster.Home(root, "alpha"),
		Token:   strings.TrimSpace(string(tok)),
	}
	os.MkdirAll(idA.DataDir, 0o700)
	os.MkdirAll(filepath.Join(roster.Home(root, "beta"), ".crush"), 0o700)
	return root, a, b, idA
}

func writeTurn(t *testing.T, root, slug, kind string, inboundHop, maxSends, maxHops, sends int, trace []string) {
	t.Helper()
	home := roster.Home(root, slug)
	tr := crush.Turn{
		Bot:        slug,
		Kind:       kind,
		StartedAt:  time.Now().UTC(),
		InboundHop: inboundHop,
		MaxSends:   maxSends,
		MaxHops:    maxHops,
		Sends:      sends,
		Trace:      trace,
	}
	if err := crush.WriteTurn(home, tr); err != nil {
		t.Fatal(err)
	}
}

func TestReplyABAok(t *testing.T) {
	root, _, _, id := setupPair(t)
	writeTurn(t, root, "alpha", "wake", 1, 4, 8, 0, []string{"user", "beta"})
	r := MessageBot(id, "beta", "hello back")
	if r.Reason != "" {
		t.Fatalf("%+v", r)
	}
	if r.Status != "queued" {
		t.Fatalf("want queued (daemon down), got %s", r.Status)
	}
	if r.ID == "" {
		t.Fatal("missing id")
	}
}

func TestHopLimit(t *testing.T) {
	root, _, _, id := setupPair(t)
	writeTurn(t, root, "alpha", "wake", 8, 4, 8, 0, []string{"user"})
	r := MessageBot(id, "beta", "too far")
	if r.Reason != "hop_limit" {
		t.Fatalf("%+v", r)
	}
}

func TestUnknownBot(t *testing.T) {
	root, _, _, id := setupPair(t)
	writeTurn(t, root, "alpha", "human_say", 0, 4, 8, 0, []string{"user"})
	r := MessageBot(id, "nope", "hi")
	if r.Reason != "unknown_bot" {
		t.Fatalf("%+v", r)
	}
}

func TestSelfMessage(t *testing.T) {
	root, _, _, id := setupPair(t)
	writeTurn(t, root, "alpha", "human_say", 0, 4, 8, 0, []string{"user"})
	r := MessageBot(id, "alpha", "hi")
	if r.Reason != "self_message" {
		t.Fatalf("%+v", r)
	}
}

func TestFanout(t *testing.T) {
	root, _, _, id := setupPair(t)
	writeTurn(t, root, "alpha", "human_say", 0, 4, 8, 4, []string{"user"})
	r := MessageBot(id, "beta", "hi")
	if r.Reason != "fanout_limit" {
		t.Fatalf("%+v", r)
	}
}

func TestHumanHop0(t *testing.T) {
	root, _, _, id := setupPair(t)
	writeTurn(t, root, "alpha", "human_say", 0, 4, 8, 0, []string{"user"})
	r := MessageBot(id, "beta", "hi")
	if r.Reason != "" {
		t.Fatal(r)
	}
	raw, err := os.ReadFile(filepath.Join(roster.Home(root, "beta"), "inbox", "pending", r.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"hop": 1`) {
		t.Fatalf("%s", raw)
	}
}

func TestHumanChatCap32(t *testing.T) {
	root, _, _, id := setupPair(t)
	writeTurn(t, root, "alpha", "human_chat", 0, 32, 8, 31, []string{"user"})
	r := MessageBot(id, "beta", "still ok")
	if r.Reason != "" {
		t.Fatalf("31/32 should pass: %+v", r)
	}
	r = MessageBot(id, "beta", "nope")
	if r.Reason != "fanout_limit" {
		t.Fatalf("32/32: %+v", r)
	}
}

func TestNoTurn(t *testing.T) {
	_, _, _, id := setupPair(t)
	r := MessageBot(id, "beta", "hi")
	if r.Reason != "missing_config" {
		t.Fatalf("%+v", r)
	}
}

func TestMessageBotBlockedDuringRound(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "bots"), 0o700)
	a, _, err := roster.Spawn(root, roster.SpawnOpts{Slug: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	home := roster.Home(root, "alpha")
	_ = os.WriteFile(filepath.Join(home, "turn.json"), []byte(`{"kind":"group_round","group_id":"review"}`), 0o600)
	var b roster.Bot
	if _, _, err := roster.Spawn(root, roster.SpawnOpts{Slug: "beta"}); err != nil {
		t.Fatal(err)
	}
	protocol.Write(protocol.Options{Root: root, Bot: a, Teammates: []roster.Bot{a, b}})
	tok, _ := os.ReadFile(filepath.Join(home, ".mcp_token"))
	id := Identity{
		Root:    root,
		Bot:     "alpha",
		DataDir: filepath.Join(home, ".crush"),
		Cwd:     home,
		Token:   strings.TrimSpace(string(tok)),
	}
	os.MkdirAll(id.DataDir, 0o700)
	res := MessageBot(id, "beta", "psst")
	if res.Reason != "room_dm_blocked" {
		t.Fatalf("want room_dm_blocked, got %+v", res)
	}
	if res.Error == "" || !strings.Contains(res.Error, "group_say") {
		t.Fatalf("error should nudge to group_say: %+v", res)
	}
}
