package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/hocoder-agents/crush-bot/internal/config"
	"github.com/hocoder-agents/crush-bot/internal/group"
)

type settleMsg struct{ err error }
type tickMsg time.Time

type groupModel struct {
	home, bin string
	cfg       config.Config
	g         group.Group
	vp        viewport.Model
	in        textinput.Model
	width     int
	height    int
	settling  bool
	status    string
	cancel    context.CancelFunc
}

func newGroupModel(home, bin string, cfg config.Config, g group.Group) groupModel {
	ti := textinput.New()
	ti.Placeholder = "message the room; @slug to address one member"
	_ = ti.Focus()
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(16))
	m := groupModel{home: home, bin: bin, cfg: cfg, g: g, vp: vp, in: ti, status: "idle"}
	m.reloadTranscript()
	return m
}

func (m *groupModel) reloadTranscript() {
	lines, _ := group.ReadTranscript(m.home, m.g.ID)
	body := renderRoomTranscript(lines)
	if body == "" {
		body = mutedStyle.Render("(empty room — type a line to start a round)") + "\n"
	}
	m.vp.SetContent(body)
	m.vp.GotoBottom()
}

func (m groupModel) Init() tea.Cmd {
	return tea.Batch(
		m.in.Focus(),
		tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) }),
	)
}

func (m groupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		vh := msg.Height - 8
		if vh < 5 {
			vh = 5
		}
		m.vp.SetWidth(msg.Width - 4)
		m.vp.SetHeight(vh)
		m.in.SetWidth(msg.Width - 6)
	case tickMsg:
		m.reloadTranscript()
		cmds = append(cmds, tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) }))
	case settleMsg:
		m.settling = false
		if m.cancel != nil {
			m.cancel()
			m.cancel = nil
		}
		if msg.err != nil {
			m.status = msg.err.Error()
		} else {
			m.status = "idle"
		}
		m.reloadTranscript()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "enter":
			if m.settling {
				break
			}
			line := strings.TrimSpace(m.in.Value())
			if line == "" {
				break
			}
			m.in.SetValue("")
			m.settling = true
			m.status = "round running…"
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			m.cancel = cancel
			g := m.g
			home, bin, cfg := m.home, m.bin, m.cfg
			cmds = append(cmds, func() tea.Msg {
				err := group.RunUntilSettle(ctx, cfg, bin, home, g, line)
				return settleMsg{err}
			})
		}
	}
	var cmd tea.Cmd
	m.in, cmd = m.in.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	m.vp, cmd = m.vp.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m groupModel) View() tea.View {
	var b strings.Builder
	fmt.Fprintln(&b, gradientText("group @"+m.g.ID))
	fmt.Fprintln(&b, mutedStyle.Render(strings.Join(m.g.Members, "  ")+"  ·  "+m.status))
	fmt.Fprintln(&b, m.vp.View())
	fmt.Fprintln(&b, keyStyle.Render("> ")+m.in.View())
	fmt.Fprintln(&b, mutedStyle.Render("enter send  ·  esc quit  (rounds keep going if you leave)"))
	body := b.String()
	if m.width > 0 {
		body = boxStyle.Width(m.width).Render(body)
	}
	v := tea.NewView(body)
	v.AltScreen = true
	return v
}

func RunGroup(home, bin string, cfg config.Config, g group.Group) error {
	p := tea.NewProgram(newGroupModel(home, bin, cfg, g))
	_, err := p.Run()
	return err
}
