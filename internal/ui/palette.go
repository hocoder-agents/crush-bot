package ui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hocoder-agents/crush-bot/internal/version"
)

type action struct {
	label string
	hint  string
	run   func(m Model) (tea.Model, tea.Cmd)
}

// paletteActions is the quicklaunch surface. Labels are filter targets.
func (m Model) paletteActions() []action {
	return []action{
		{label: "open bot transcript", hint: "enter", run: func(m Model) (tea.Model, tea.Cmd) { return m.openSelected() }},
		{label: "open mailbox", hint: "i", run: func(m Model) (tea.Model, tea.Cmd) { return m.openInbox() }},
		{label: "spawn a bot", hint: "n", run: func(m Model) (tea.Model, tea.Cmd) {
			return m.openSpawnForm()
		}},
		{label: "create group", hint: "", run: func(m Model) (tea.Model, tea.Cmd) {
			return m.openGroupForm()
		}},
		{label: "refresh roster", hint: "r", run: func(m Model) (tea.Model, tea.Cmd) {
			m2 := m
			m2.reload()
			if m2.chatSlug != "" {
				m2.reloadChat()
			}
			if m2.chatGroup != "" {
				m2.reloadGroupChat()
			}
			return m2, nil
		}},

		{label: "disband room under cursor", hint: "", run: func(m Model) (tea.Model, tea.Cmd) {
			g, ok := m.selectedGroup()
			if !ok {
				m.status = "select a room to disband"
				return m, nil
			}
			m.disbandPending = g.ID
			m.status = "disband @" + g.ID + "? press D to confirm"
			return m, nil
		}},
		{label: "doctor --check", hint: "", run: func(m Model) (tea.Model, tea.Cmd) {
			return m, tea.Exec(&doctorWizard{}, func(err error) tea.Msg {
				return doctorDoneMsg{err: err}
			})
		}},
		{label: "quit", hint: "q", run: func(m Model) (tea.Model, tea.Cmd) { return m.quitHost() }},
	}
}

func (m Model) paletteFiltered() []action {
	acts := m.paletteActions()
	q := strings.ToLower(strings.TrimSpace(m.paletteQuery))
	if q == "" {
		return acts
	}
	var out []action
	for _, a := range acts {
		if strings.Contains(strings.ToLower(a.label), q) || strings.Contains(strings.ToLower(a.hint), q) {
			out = append(out, a)
		}
	}
	return out
}

func (m *Model) paletteClamp() {
	acts := m.paletteFiltered()
	if m.paletteIdx >= len(acts) {
		m.paletteIdx = max(0, len(acts)-1)
	}
}

func (m Model) paletteView(width, height int) string {
	acts := m.paletteFiltered()
	inner := width - 8 // border + padding margins
	if inner < 12 {
		inner = 12
	}
	if inner > 44 {
		inner = 44
	}
	var b strings.Builder
	fmt.Fprintln(&b, gradientText("quicklaunch")+"  "+mutedStyle.Render(version.Version))
	fmt.Fprintln(&b, mutedStyle.Render("/"+m.paletteQuery))
	fmt.Fprintln(&b)
	if len(acts) == 0 {
		fmt.Fprintln(&b, mutedStyle.Render("no matching action"))
	}
	for i, a := range acts {
		mark := " "
		if i == m.paletteIdx {
			mark = "▸"
		}
		line := fmt.Sprintf("%s %s", mark, nameStyle.Render(a.label))
		if a.hint != "" {
			line += "  " + keyStyle.Render(a.hint)
		}
		if i == m.paletteIdx {
			line = selStyle.Width(inner).Render(line)
		}
		fmt.Fprintln(&b, line)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, mutedStyle.Render("type to filter · enter run · esc close"))
	box := modalStyle.Render(b.String())
	return lipgloss.Place(width, height,
		lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Background(lipgloss.Color("#191622"))),
	)
}

// modalStyle frames the quicklaunch as a floating panel.
var modalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lavender).
	Padding(1, 3)

type doctorDoneMsg struct{ err error }

// doctorWizard shells the operator's own crushbot binary for doctor --check,
// same exec pattern as the spawn wizard.
type doctorWizard struct {
	in  io.Reader
	out io.Writer
	err io.Writer
}

func (w *doctorWizard) Run() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "doctor", "--check")
	cmd.Stdin = w.in
	cmd.Stdout = w.out
	cmd.Stderr = w.err
	return cmd.Run()
}

func (w *doctorWizard) SetStdin(r io.Reader)  { w.in = r }
func (w *doctorWizard) SetStdout(f io.Writer) { w.out = f }
func (w *doctorWizard) SetStderr(f io.Writer) { w.err = f }

// updatePalette handles keys while the quicklaunch panel is open.
func (m Model) updatePalette(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.paletteOpen = false
		m.paletteQuery = ""
		m.paletteIdx = 0
		m.status = "list focused — q quits"
		return m, nil
	case "enter":
		m.paletteClamp()
		acts := m.paletteFiltered()
		if len(acts) == 0 {
			return m, nil
		}
		a := acts[m.paletteIdx]
		m.paletteOpen = false
		m.paletteQuery = ""
		m.paletteIdx = 0
		return a.run(m)
	case "up", "k", "ctrl+p":
		if m.paletteIdx > 0 {
			m.paletteIdx--
		}
		return m, nil
	case "down", "j", "ctrl+n":
		m.paletteIdx++
		m.paletteClamp()
		return m, nil
	case "backspace", "delete":
		r := []rune(m.paletteQuery)
		if len(r) > 0 {
			m.paletteQuery = string(r[:len(r)-1])
		}
		m.paletteIdx = 0
		return m, nil
	}
	for _, r := range msg.String() {
		m.paletteQuery += string(r)
	}
	m.paletteIdx = 0
	return m, nil
}
