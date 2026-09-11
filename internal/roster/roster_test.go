package roster

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hocoder-agents/crush-bot/internal/soul"
)

func TestValidSlug(t *testing.T) {
	ok := []string{"a", "researcher", "bot-2"}
	bad := []string{"", "A", "1bot", "has_underscore", "has space", "x/"}
	for _, s := range ok {
		if !ValidSlug(s) {
			t.Fatalf("want valid: %q", s)
		}
	}
	for _, s := range bad {
		if ValidSlug(s) {
			t.Fatalf("want invalid: %q", s)
		}
	}
}

func TestSpawnSeedOnce(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bots"), 0o700); err != nil {
		t.Fatal(err)
	}
	b, _, err := Spawn(root, SpawnOpts{Slug: "coder", Title: "Coder"})
	if err != nil {
		t.Fatal(err)
	}
	if b.Slug != "coder" || b.CanonicalSessionTitle != "bot:coder" {
		t.Fatalf("%+v", b)
	}
	path := SoulPath(root, "coder")
	if err := os.WriteFile(path, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = soul.WriteSeed(path, "coder")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "keep me" {
		t.Fatalf("soul overwritten: %q", got)
	}
	user := filepath.Join(Home(root, "coder"), "crushrc.d", "90-user.crushrc")
	if _, err := os.Stat(user); err != nil {
		t.Fatal(err)
	}
}

func TestSpawnSoulBody(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "bots"), 0o700)
	_, _, err := Spawn(root, SpawnOpts{Slug: "a", SoulBody: "custom soul"})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := soul.Read(SoulPath(root, "a"), 0)
	if body != "custom soul" {
		t.Fatalf("got %q", body)
	}
}

func TestSpawnDuplicate(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "bots"), 0o700)
	if _, _, err := Spawn(root, SpawnOpts{Slug: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Spawn(root, SpawnOpts{Slug: "a"}); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestCoderTools(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "bots"), 0o700)
	b, _, err := Spawn(root, SpawnOpts{Slug: "dev", Coder: true})
	if err != nil {
		t.Fatal(err)
	}
	if !b.Tools.Bash || !b.Tools.Edit {
		t.Fatalf("coder tools: %+v", b.Tools)
	}
}

func TestHideListCloneDelete(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "bots"), 0o700)
	if _, _, err := Spawn(root, SpawnOpts{Slug: "alpha", Title: "Alpha"}); err != nil {
		t.Fatal(err)
	}
	if _, err := SetHidden(root, "alpha", true); err != nil {
		t.Fatal(err)
	}
	vis, _ := List(root, false)
	if len(vis) != 0 {
		t.Fatalf("hidden still listed: %v", vis)
	}
	all, _ := List(root, true)
	if len(all) != 1 {
		t.Fatalf("all: %d", len(all))
	}
	c, _, err := Clone(root, "alpha", "beta", 32, 32768)
	if err != nil {
		t.Fatal(err)
	}
	if c.Slug != "beta" || c.CloneFrom == nil || *c.CloneFrom != "alpha" {
		t.Fatalf("%+v", c)
	}
	if err := Delete(root, "alpha"); err != nil {
		t.Fatal(err)
	}
	if Exists(root, "alpha") {
		t.Fatal("alpha still there")
	}
}

func TestSharedProjectWarn(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "bots"), 0o700)
	proj := filepath.Join(root, "proj")
	os.MkdirAll(proj, 0o700)
	if _, _, err := Spawn(root, SpawnOpts{Slug: "one", Project: proj}); err != nil {
		t.Fatal(err)
	}
	_, warns, err := Spawn(root, SpawnOpts{Slug: "two", Project: proj})
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) == 0 {
		t.Fatal("expected shared project warning")
	}
}
