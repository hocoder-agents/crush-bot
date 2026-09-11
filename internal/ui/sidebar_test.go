package ui

import (
	"strings"
	"testing"

	"github.com/hocoder-agents/crush-bot/internal/roster"
)

func TestSidebarTitleLayout(t *testing.T) {
	m := Model{
		rows: []row{
			{bot: roster.Bot{Slug: "sophie", Title: "Orchestrator"}},
			{bot: roster.Bot{Slug: "diana", Title: "Coder"}},
		},
		cursor: 0,
		focus:  focusSide,
	}
	out := m.sidebarView(24, 12)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	sel := -1
	for i, l := range lines {
		if strings.Contains(l, "@sophie") {
			sel = i
			break
		}
	}
	if sel == -1 {
		t.Fatalf("no sophie row:\n%s", out)
	}
	if sel+1 >= len(lines) || !strings.Contains(lines[sel+1], "Orchestrator") {
		t.Fatalf("selected bot title not on second line:\n%s", out)
	}
	diana := -1
	for i, l := range lines {
		if strings.Contains(l, "@diana") {
			diana = i
			break
		}
	}
	if diana == -1 || diana+1 >= len(lines) || !strings.Contains(lines[diana+1], "Coder") {
		t.Fatalf("unselected bot title not on its own line:\n%s", out)
	}
	if strings.Contains(lines[diana], "Coder") {
		t.Fatalf("unselected title should not share the name line:\n%s", out)
	}
}

func TestSidebarSelectionIsBackground(t *testing.T) {
	m := Model{
		rows:  []row{{bot: roster.Bot{Slug: "diana", Title: "Coder"}}},
		focus: focusSide,
	}
	out := m.sidebarView(24, 10)
	if !strings.Contains(out, "\x1b[48;2;58;52;85m") && !strings.Contains(out, "\x1b[48;5;") {
		t.Fatalf("selection has no background wash:\n%q", out)
	}
}

func TestSidebarOrchestratorFirst(t *testing.T) {
	home := t.TempDir()
	for _, s := range []string{"diana", "sophie", "andreea"} {
		if err := roster.Save(home, roster.Bot{Slug: s}); err != nil {
			t.Fatal(err)
		}
	}
	m := Model{home: home}
	m.reload()
	if len(m.rows) == 0 || m.rows[0].bot.Slug != "sophie" {
		t.Fatalf("sophie not first: %+v", m.rows)
	}
}

func TestDividerIsLavender(t *testing.T) {
	m := Model{}
	out := m.divider(5)
	if !strings.Contains(out, "\x1b[38;2;196;181;253m") {
		t.Fatalf("divider not lavender: %q", out)
	}
}

func TestCrushViewPaddedFromDivider(t *testing.T) {
	m := Model{rows: []row{{bot: roster.Bot{Slug: "diana", Title: "Coder"}}}, chatSlug: "diana"}
	out := m.crushView(40, 10)
	lines := strings.Split(out, "\n")
	for _, l := range lines {
		if l == "" {
			continue
		}
		if !strings.HasPrefix(l, " ") {
			t.Fatalf("transcript line not padded: %q", l)
		}
	}
}
