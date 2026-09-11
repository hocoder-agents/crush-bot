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
	var inline string
	for _, l := range lines {
		if strings.Contains(l, "@diana") {
			inline = l
			break
		}
	}
	if !strings.Contains(inline, "Coder") {
		t.Fatalf("unselected bot title not inline:\n%s", out)
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
