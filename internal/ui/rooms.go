package ui

import (
	"context"
	"fmt"
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
	var b strings.Builder
	lines, _ := group.ReadTranscript(m.home, m.chatGroup)
	for _, l := range lines {
		kind := l.Kind
		if l.Pass {
			kind = "pass"
		}
		fmt.Fprintf(&b, "%s  %s  %s\n", kind, l.From, l.Body)
	}
	if b.Len() == 0 {
		b.WriteString("(empty room — type a line to start a round)\n")
	}
	m.vp.SetContent(b.String())
	m.vp.GotoBottom()
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
