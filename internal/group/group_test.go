package group

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hocoder-agents/crush-bot/internal/roster"
)

func TestCreateAndTranscript(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "bots"), 0o700)
	_, _, err := roster.Spawn(root, roster.SpawnOpts{Slug: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = roster.Spawn(root, roster.SpawnOpts{Slug: "beta"})
	if err != nil {
		t.Fatal(err)
	}
	g, err := Create(root, "review", []string{"alpha", "beta"})
	if err != nil {
		t.Fatal(err)
	}
	if g.ID != "review" || len(g.Members) != 2 {
		t.Fatalf("%+v", g)
	}
	if err := AppendLine(root, g.ID, Line{From: "user", Kind: "line", Body: "hello @alpha"}); err != nil {
		t.Fatal(err)
	}
	lines, err := ReadTranscript(root, g.ID)
	if err != nil || len(lines) != 1 || lines[0].Seq != 1 {
		t.Fatalf("%+v %v", lines, err)
	}
	scope := InScope("hello @alpha", g.Members)
	if len(scope) != 1 || scope[0] != "alpha" {
		t.Fatalf("%v", scope)
	}
}

func TestCreateBounds(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "bots"), 0o700)
	roster.Spawn(root, roster.SpawnOpts{Slug: "a"})
	if _, err := Create(root, "x", []string{"a"}); err == nil {
		t.Fatal("need 2 members")
	}
}

func TestMentionsToleratesSpaceAfterAt(t *testing.T) {
	members := []string{"diana", "sophie"}
	if got := Mentions("@ sophie what do you think?", members); len(got) != 1 || got[0] != "sophie" {
		t.Fatalf("space mention missed: %v", got)
	}
	if got := Mentions("hey @sophie", members); len(got) != 1 || got[0] != "sophie" {
		t.Fatalf("tight mention missed: %v", got)
	}
	if got := Mentions("no mentions here", members); got != nil {
		t.Fatalf("false positive: %v", got)
	}
}
