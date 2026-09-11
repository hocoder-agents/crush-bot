package preset

import (
	"strings"
	"testing"

	"github.com/hocoder-agents/crush-bot/internal/roster"
)

func TestManifest(t *testing.T) {
	entries, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no presets in manifest")
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if !roster.ValidSlug(e.Slug) {
			t.Fatalf("bad slug %q", e.Slug)
		}
		if seen[e.Slug] {
			t.Fatalf("duplicate slug %q", e.Slug)
		}
		seen[e.Slug] = true
		if e.Title == "" {
			t.Fatalf("preset %q has no title", e.Slug)
		}
	}
}

func TestGet(t *testing.T) {
	e, body, ok := Get("coder")
	if !ok {
		t.Fatal("missing coder preset")
	}
	if !e.Coder {
		t.Fatal("coder preset should set coder")
	}
	if strings.TrimSpace(body) == "" || !strings.Contains(body, "# Identity") {
		t.Fatalf("bad soul body: %q", body)
	}
	if _, _, ok := Get("nobody"); ok {
		t.Fatal("unknown preset should not match")
	}
}

func TestEveryPresetHasSoul(t *testing.T) {
	entries, err := List()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		_, body, ok := Get(e.Slug)
		if !ok {
			t.Fatalf("preset %q has no soul file", e.Slug)
		}
		if strings.TrimSpace(body) == "" {
			t.Fatalf("preset %q soul is blank", e.Slug)
		}
	}
}
