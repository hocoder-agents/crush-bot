package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hocoder-agents/crush-bot/internal/config"
	"github.com/hocoder-agents/crush-bot/internal/group"
	"github.com/hocoder-agents/crush-bot/internal/protocol"
	"github.com/hocoder-agents/crush-bot/internal/roster"
)

func TestDaemonRunsRoundRequests(t *testing.T) {
	bin := fakeCrush(t)
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "bots"), 0o700)
	alpha, _, err := roster.Spawn(root, roster.SpawnOpts{Slug: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	beta, _, err := roster.Spawn(root, roster.SpawnOpts{Slug: "beta"})
	if err != nil {
		t.Fatal(err)
	}
	_ = protocol.Write(protocol.Options{Root: root, Bot: alpha})
	_ = protocol.Write(protocol.Options{Root: root, Bot: beta})
	g, err := group.Create(root, "review", []string{"alpha", "beta"})
	if err != nil {
		t.Fatal(err)
	}
	if err := group.EnqueueRequest(root, g.ID, "hello @beta"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	o := Options{Root: root, Bin: bin, Cfg: config.Default()}
	deadline := time.Now().Add(15 * time.Second)
	for {
		_, _ = Once(ctx, o)
		lines, _ := group.ReadTranscript(root, g.ID)
		sawBot := false
		for _, l := range lines {
			if l.From == "beta" && l.Kind == "line" {
				sawBot = true
			}
		}
		if sawBot {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("round never produced a bot line; transcript:\n%v", lines)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if _, err := os.Stat(group.RequestPath(root, g.ID)); !os.IsNotExist(err) {
		t.Fatalf("request file not consumed: %v", err)
	}
}

func TestFlushStaleSays(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "bots"), 0o700)
	alpha, _, err := roster.Spawn(root, roster.SpawnOpts{Slug: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	_ = protocol.Write(protocol.Options{Root: root, Bot: alpha})
	beta, _, err := roster.Spawn(root, roster.SpawnOpts{Slug: "beta"})
	if err != nil {
		t.Fatal(err)
	}
	_ = protocol.Write(protocol.Options{Root: root, Bot: beta})
	g, err := group.Create(root, "review", []string{"alpha", "beta"})
	if err != nil {
		t.Fatal(err)
	}
	// simulate an orphaned turn: says file without a live turn.json
	says := "the answer the orphan left behind"
	if err := os.WriteFile(group.SaysPath(root, g.ID, "beta"), []byte(says+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if n := group.FlushStaleSays(root, g.ID); n != 1 {
		t.Fatalf("recovered %d lines, want 1", n)
	}
	if _, err := os.Stat(group.SaysPath(root, g.ID, "beta")); !os.IsNotExist(err) {
		t.Fatalf("says file not removed: %v", err)
	}
	lines, _ := group.ReadTranscript(root, g.ID)
	if len(lines) != 1 || lines[0].From != "beta" || !strings.Contains(lines[0].Body, "orphan") {
		t.Fatalf("transcript missing recovered line: %+v", lines)
	}
	// a LIVE turn must not be flushed
	home := roster.Home(root, "alpha")
	if err := os.WriteFile(filepath.Join(home, "turn.json"), []byte(`{"kind":"group_round","group_id":"review","crush_pid":`+fmt.Sprintf("%d", os.Getpid())+`}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(group.SaysPath(root, g.ID, "alpha"), []byte("live line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if n := group.FlushStaleSays(root, g.ID); n != 0 {
		t.Fatalf("flushed lines from a live turn: %d", n)
	}
	if _, err := os.Stat(group.SaysPath(root, g.ID, "alpha")); err != nil {
		t.Fatal("live says file was removed")
	}
	os.Remove(filepath.Join(home, "turn.json"))
	_ = beta
}
