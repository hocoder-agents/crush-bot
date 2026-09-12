package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/hocoder-agents/crush-bot/internal/config"
	"github.com/hocoder-agents/crush-bot/internal/group"
)

type tickMsg time.Time
type enqueueErr struct{ err error }

// Settle rounds are executed by the daemon (which outlives this TUI);
// sending just enqueues a request and the 200ms tick replays the
// transcript the daemon writes.
type groupModel struct {
	home   string
	bin    string
	cfg    config.Config
	g      group.Group
	vp     viewport.Model
	in     textinput.Model
	width  int
	height int
	status string
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
		switch {
		case group.RoomBusy(m.home, m.g.ID, m.g.Members):
			m.status = "round running…"
		case m.status == "round running…":
			m.status = "idle"
		}
		cmds = append(cmds, tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) }))
	case enqueueErr:
		if msg.err != nil {
			m.status = "enqueue failed: " + msg.err.Error()
		} else {
			m.status = "round queued — the daemon runs it"
		}
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "enter":
			line := strings.TrimSpace(m.in.Value())
			if line == "" {
				break
			}
			m.in.SetValue("")
			home := m.home
			g := m.g
			cmds = append(cmds, func() tea.Msg {
				return enqueueErr{err: group.EnqueueRequest(home, g.ID, line)}
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
	fmt.Fprintln(&b, mutedStyle.Render("enter send  ·  esc quit  (the daemon runs rounds — you can leave anytime)"))
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
