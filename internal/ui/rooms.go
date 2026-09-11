package ui

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hocoder-agents/crush-bot/internal/config"
	"github.com/hocoder-agents/crush-bot/internal/crush"
	"github.com/hocoder-agents/crush-bot/internal/group"
)

type groupDoneMsg struct{ err error }

// flatTotal is the combined sidebar index space: bots, then rooms.
func (m Model) flatTotal() int {
	return len(m.rows) + len(m.groups)
}

// selectedGroup returns the room under the flat cursor, if any.
func (m Model) selectedGroup() (group.Group, bool) {
	if i := m.cursor - len(m.rows); i >= 0 && i < len(m.groups) {
		return m.groups[i], true
	}
	return group.Group{}, false
}

func padStyle(width int) lipgloss.Style {
	return lipgloss.NewStyle().PaddingLeft(1).Width(width).MaxWidth(width)
}

func (m *Model) reloadGroupChat() {
	if m.chatGroup == "" {
		return
	}
	lines, _ := group.ReadTranscript(m.home, m.chatGroup)
	body := renderRoomTranscript(lines)
	if body == "" {
		body = mutedStyle.Render("(empty room — type a line to start a round)") + "\n"
	}
	m.vp.SetContent(body)
	m.vp.GotoBottom()
}

// roomPalette gives each member a stable color so conversations read as
// speakers, not walls of text.
var roomPalette = []string{
	"205", // pink
	"212", // orchid pink
	"39",  // cyan
	"170", // violet
	"69",  // blue
	"208", // orange
	"141", // light purple
	"72",  // teal
}

func memberStyle(slug string) lipgloss.Style {
	if slug == "sophie" {
		// The orchestrator keeps the brand lavender.
		return lipgloss.NewStyle().Foreground(lavender).Bold(true)
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(slug))
	return lipgloss.NewStyle().Foreground(lipgloss.Color(roomPalette[h.Sum32()%uint32(len(roomPalette))])).Bold(true)
}

func userStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
}

// renderRoomTranscript lays out a room log: round dividers, colored
// speaker names, dimmed passes and system notes.
func renderRoomTranscript(lines []group.Line) string {
	var b strings.Builder
	prev := -1
	for _, l := range lines {
		if l.Round != prev {
			prev = l.Round
			if l.Round > 0 && b.Len() > 0 {
				fmt.Fprintln(&b, mutedStyle.Render(fmt.Sprintf("──  round %d  ──", l.Round)))
			}
		}
		switch {
		case l.Kind == "system":
			fmt.Fprintf(&b, "%s\n", mutedStyle.Render("   ⚠ "+l.Body))
		case l.Pass || l.Kind == "pass":
			note := "passed"
			if l.Body != "" && l.Body != "PASS" {
				note = l.Body
			}
			fmt.Fprintf(&b, "%s\n", mutedStyle.Render("   · "+l.From+" "+note))
		case l.From == "user":
			fmt.Fprintf(&b, "%s  %s\n", userStyle().Render("you"), l.Body)
		default:
			fmt.Fprintf(&b, "%s  %s\n", memberStyle(l.From).Render("@"+l.From), l.Body)
		}
	}
	return b.String()
}

func (m Model) groupView(width, height int) string {
	var members []string
	for _, g := range m.groups {
		if g.ID == m.chatGroup {
			members = g.Members
			break
		}
	}
	head := nameStyle.Render("@"+m.chatGroup) + "  " + mutedStyle.Render(strings.Join(members, "  "))
	if m.groupBusy {
		head += "  " + m.spinnerGroup.View() + " " + mutedStyle.Render("thinking")
	}
	return padStyle(width).Render(strings.Join([]string{head, m.vp.View(), m.in.View()}, "\n"))
}

func (m Model) sendGroup(line string) tea.Cmd {
	m.chatBusy = true
	m.groupBusy = true
	m.status = "round running in @" + m.chatGroup
	gid := m.chatGroup
	home := m.home
	return func() tea.Msg {
		cfg, err := config.Load(config.ResolvePaths())
		if err != nil {
			return groupDoneMsg{err: err}
		}
		bin, err := crush.LookPath(cfg.CrushPath)
		if err != nil {
			return groupDoneMsg{err: err}
		}
		g, err := group.Load(home, gid)
		if err != nil {
			return groupDoneMsg{err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		return groupDoneMsg{err: group.RunUntilSettle(ctx, cfg, bin, home, g, line)}
	}
}

func (m Model) disbandUnderCursor() (tea.Model, tea.Cmd) {
	g, ok := m.selectedGroup()
	if !ok {
		m.status = "select a room to disband"
		return m, nil
	}
	if err := group.Delete(m.home, g.ID); err != nil {
		m.status = "disband failed: " + err.Error()
		return m, nil
	}
	if m.chatGroup == g.ID {
		m.chatGroup = ""
	}
	m.reload()
	m.status = "disbanded @" + g.ID
	return m, nil
}
