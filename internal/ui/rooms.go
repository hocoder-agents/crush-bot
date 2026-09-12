package ui

import (
	"fmt"
	"hash/fnv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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
	for _, g := range m.groups {
		if g.ID == m.chatGroup {
			m.groupBusy = group.RoomBusy(m.home, g.ID, g.Members)
			break
		}
	}
	lines, _ := group.ReadTranscript(m.home, m.chatGroup)
	body := renderRoomTranscript(lines, m.vp.Width())
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
// speaker names, dimmed passes and system notes. Lines wrap to the
// pane width with continuation lines indented under the speaker.
func renderRoomTranscript(lines []group.Line, width int) string {
	var b strings.Builder
	prev := -1
	prevTurn := false
	for _, l := range lines {
		isTurn := !l.Pass && l.Kind != "pass" && l.Kind != "system"
		if l.Round != prev {
			prev = l.Round
			if l.Round > 0 && b.Len() > 0 {
				fmt.Fprintln(&b, mutedStyle.Render(fmt.Sprintf("──  round %d  ──", l.Round)))
			}
			prevTurn = false
		} else if isTurn && prevTurn && b.Len() > 0 {
			// One blank line between speaker turns within a round.
			fmt.Fprintln(&b)
		}
		prevTurn = isTurn
		switch {
		case l.Kind == "system":
			fmt.Fprintln(&b, wrapRoomLine(mutedStyle.Render("   ⚠ "+l.Body), "", width))
		case l.Pass || l.Kind == "pass":
			note := "passed"
			if l.Body != "" && l.Body != "PASS" {
				note = l.Body
			}
			fmt.Fprintln(&b, wrapRoomLine(mutedStyle.Render("   · "+l.From+" "+note), "", width))
		case l.From == "user":
			fmt.Fprintln(&b, wrapRoomLine(userStyle().Render("you"), l.Body, width))
		default:
			fmt.Fprintln(&b, wrapRoomLine(memberStyle(l.From).Render("@"+l.From), l.Body, width))
		}
	}
	return b.String()
}

// wrapRoomLine word-wraps a transcript line to the pane width, indenting
// continuation lines under the message so a paragraph still reads as one
// speaker turn. ANSI styles in the prefix survive the wrap.
func wrapRoomLine(prefix, body string, width int) string {
	if width <= 0 || body == "" {
		if body == "" {
			return prefix
		}
		return prefix + "  " + body
	}
	limit := width - 2 - lipgloss.Width(prefix)
	if limit < 12 {
		limit = 12
	}
	wrapped := ansi.Wordwrap(body, limit, " \t-")
	parts := strings.Split(wrapped, "\n")
	indent := strings.Repeat(" ", lipgloss.Width(prefix)+2)
	out := prefix + "  " + parts[0]
	for _, l := range parts[1:] {
		out += "\n" + indent + l
	}
	return out
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
	gid := m.chatGroup
	home := m.home
	m.status = "round queued in @" + gid + " (daemon runs it)"
	return func() tea.Msg {
		if err := group.EnqueueRequest(home, gid, line); err != nil {
			return groupDoneMsg{err: err}
		}
		return groupQueuedMsg{gid: gid}
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

type groupQueuedMsg struct{ gid string }
