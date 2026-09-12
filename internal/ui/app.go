package ui

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hocoder-agents/crush-bot/internal/crush"
	"github.com/hocoder-agents/crush-bot/internal/daemon"
	"github.com/hocoder-agents/crush-bot/internal/envelope"
	"github.com/hocoder-agents/crush-bot/internal/group"
	"github.com/hocoder-agents/crush-bot/internal/roster"
	"github.com/hocoder-agents/crush-bot/internal/spawn"
)

const (
	helpRows         = 1
	dividerW         = 1
	minSideW         = 16
	maxSideW         = 28
	focusSide        = 0
	glyphPlaceholder = "  "
	focusChat        = 1
	focusInbox       = 2
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	keyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	nameStyle  = lipgloss.NewStyle().Bold(true)
	// Selection is a background wash, not a font color change.
	selStyle    = lipgloss.NewStyle().Background(lipgloss.Color("#3A3455"))
	sideStyle   = lipgloss.NewStyle().Padding(0, 1)
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Padding(0, 1)
	divStyle    = lipgloss.NewStyle().Foreground(lavender)
	divHotStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	boxStyle    = lipgloss.NewStyle().Padding(1, 2)
)

type row struct {
	bot     roster.Bot
	pending int
	busy    bool
}

type Model struct {
	width, height  int
	home           string
	rows           []row
	cursor         int
	status         string
	focus          int
	chatSlug       string
	chatBusy       bool
	in             textinput.Model
	vp             viewport.Model
	showInbox      bool
	inbox          inboxState
	groups         []group.Group
	chatGroup      string
	groupBusy      bool
	paletteOpen    bool
	paletteQuery   string
	paletteIdx     int
	disbandPending string
	spawnForm      spawnFormState
	groupForm      groupFormState
	projectForm    projectFormState
	spinnerBot     spinner.Model
	spinnerGroup   spinner.Model
}

func New(home string) Model {
	m := Model{
		home:         home,
		focus:        focusSide,
		in:           newChatInput(),
		vp:           viewport.New(viewport.WithWidth(40), viewport.WithHeight(10)),
		spinnerBot:   spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(lavender))),
		spinnerGroup: spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("205")))),
	}
	m.reload()
	return m
}

func (m *Model) reload() {
	bots, err := roster.List(m.home, false)
	if err != nil {
		m.status = err.Error()
		m.rows = nil
		return
	}
	// The orchestrator leads the roster.
	sort.Slice(bots, func(i, j int) bool {
		pi, pj := bots[i].Slug == "sophie", bots[j].Slug == "sophie"
		if pi != pj {
			return pi
		}
		return bots[i].Slug < bots[j].Slug
	})
	m.rows = m.rows[:0]
	for _, b := range bots {
		home := roster.Home(m.home, b.Slug)
		envs, _, _ := envelope.List(envelope.PendingDir(home))
		busy := false
		if t, err := crush.ReadTurn(home); err == nil {
			busy = crush.PIDAlive(t.CrushPID)
		}
		m.rows = append(m.rows, row{bot: b, pending: len(envs), busy: busy})
	}
	m.groups, _ = group.List(m.home)
	if m.cursor >= m.flatTotal() {
		m.cursor = 0
	}
	if daemon.Live(m.home) {
		m.status = "daemon up"
	} else {
		m.status = "daemon down — crushbot daemon start"
	}
	if m.showInbox {
		m.reloadInbox()
	}
}

func (m *Model) reloadInbox() {
	if len(m.rows) == 0 {
		m.inbox = inboxState{}
		return
	}
	slug := m.rows[m.cursor].bot.Slug
	folder, cursor := m.inbox.folder, m.inbox.cursor
	m.inbox = loadInbox(m.home, slug)
	m.inbox.folder = folder
	m.inbox.cursor = cursor
	m.inbox.clamp()
}

type refreshTickMsg struct{}

func refreshTick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return refreshTickMsg{} })
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinnerBot.Tick, m.spinnerGroup.Tick, refreshTick())
}

type spawnDoneMsg struct {
	err  error
	slug string
}

func layout(width, height int) (sideW, crushW, bodyH int) {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	bodyH = height - helpRows
	if bodyH < 4 {
		bodyH = 4
	}
	sideW = 24
	if width < 70 {
		sideW = minSideW
	}
	if sideW > maxSideW {
		sideW = maxSideW
	}
	if sideW > width/2 {
		sideW = width / 2
	}
	if sideW < minSideW && width > minSideW+10 {
		sideW = minSideW
	}
	crushW = width - sideW - dividerW
	if crushW < 10 {
		crushW = 10
		sideW = width - crushW - dividerW
		if sideW < 8 {
			sideW = 8
		}
	}
	return sideW, crushW, bodyH
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshTickMsg:
		// Roster busy flags and pending counts go stale without polling;
		// this is what makes the thinking spinners appear for daemon wakes.
		m.reload()
		if m.chatGroup != "" {
			// Rounds run in the daemon; the transcript file is the feed.
			m.reloadGroupChat()
		}
		return m, refreshTick()
	case spinner.TickMsg:
		var c1, c2 tea.Cmd
		m.spinnerBot, c1 = m.spinnerBot.Update(msg)
		m.spinnerGroup, c2 = m.spinnerGroup.Update(msg)
		return m, tea.Batch(c1, c2)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.sizeChat()
		if m.chatSlug != "" {
			m.reloadChat()
		}
		if m.chatGroup != "" {
			m.reloadGroupChat()
		}
	case groupQueuedMsg:
		m.chatBusy = false
		m.reloadGroupChat()
		return m, m.in.Focus()
	case groupDoneMsg:
		m.chatBusy = false
		m.groupBusy = false
		m.reloadGroupChat()
		if msg.err != nil {
			m.status = "round enqueue failed: " + msg.err.Error()
		} else {
			m.status = "round queued @" + m.chatGroup
		}
		return m, m.in.Focus()
	case sayDoneMsg:
		m.chatBusy = false
		m.reload()
		m.reloadChat()
		if msg.err != nil {
			m.status = "say failed: " + msg.err.Error()
		} else {
			m.status = "said @" + msg.slug
		}
		return m, m.in.Focus()
	case spawnDoneMsg:
		m.reload()
		if msg.slug != "" {
			m.status = "spawned @" + msg.slug
			for i, r := range m.rows {
				if r.bot.Slug == msg.slug {
					m.cursor = i
					break
				}
			}
			break
		}
		switch {
		case msg.err != nil && (errors.Is(msg.err, spawn.ErrAborted) || msg.err.Error() == "cancelled"):
			m.status = "spawn cancelled"
		case msg.err != nil:
			m.status = "spawn failed: " + msg.err.Error()
		default:
			m.status = "spawned"
		}
	case tea.MouseClickMsg:
		return m.handleMouse("click", msg.Mouse())
	case tea.MouseReleaseMsg:
		return m.handleMouse("release", msg.Mouse())
	case tea.MouseWheelMsg:
		return m.handleMouse("wheel", msg.Mouse())
	case tea.MouseMotionMsg:
		return m.handleMouse("motion", msg.Mouse())
	case tea.InterruptMsg:
		return m.quitHost()
	case tea.KeyPressMsg:
		if isCtrl(msg, 'q') {
			return m.quitHost()
		}
		if m.formActive() {
			if m.spawnForm.active {
				return m.updateSpawnForm(msg)
			}
			if m.groupForm.active {
				return m.updateGroupForm(msg)
			}
			return m.updateProjectForm(msg)
		}
		if m.paletteOpen {
			return m.updatePalette(msg)
		}
		if m.focus == focusSide && (msg.String() == ":" || msg.String() == "?") {
			m.paletteOpen = true
			m.paletteQuery = ""
			m.paletteIdx = 0
			m.status = "quicklaunch"
			return m, nil
		}
		if isCtrl(msg, 'g') || isCtrl(msg, 'b') {
			if m.focus == focusChat || m.focus == focusInbox {
				m.focus = focusSide
				m.status = "list focused — q quits"
				return m, nil
			}
			if m.chatSlug != "" {
				m.showInbox = false
				m.focus = focusChat
				m.reloadChat()
			}
			if m.chatGroup != "" {
				m.showInbox = false
				m.focus = focusChat
				m.reloadGroupChat()
			}
			return m, nil
		}
		if m.focus == focusChat {
			switch msg.String() {
			case "esc":
				m.focus = focusSide
				m.in.Blur()
				return m, nil
			case "pgup":
				m.vp.ScrollUp(5)
				return m, nil
			case "pgdown":
				m.vp.ScrollDown(5)
				return m, nil
			case "up":
				m.vp.ScrollUp(1)
				return m, nil
			case "down":
				m.vp.ScrollDown(1)
				return m, nil
			case "k":
				if m.in.Value() == "" {
					m.vp.ScrollUp(1)
					return m, nil
				}
			case "j":
				if m.in.Value() == "" {
					m.vp.ScrollDown(1)
					return m, nil
				}
			case "enter":
				return m.sendChat()
			}
			var cmd tea.Cmd
			m.in, cmd = m.in.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m.quitHost()
		case "esc":
			if m.focus == focusInbox {
				m.focus = focusSide
				return m, nil
			}
			return m.quitHost()
		case "j", "down":
			if m.focus == focusInbox {
				m.inbox.move(1)
			} else if m.flatTotal() > 0 {
				m.disbandPending = ""
				m.cursor = (m.cursor + 1) % m.flatTotal()
				if m.showInbox {
					m.reloadInbox()
				}
			}
		case "k", "up":
			if m.focus == focusInbox {
				m.inbox.move(-1)
			} else if m.flatTotal() > 0 {
				m.disbandPending = ""
				m.cursor = (m.cursor - 1 + m.flatTotal()) % m.flatTotal()
				if m.showInbox {
					m.reloadInbox()
				}
			}
		case "tab":
			if m.focus == focusInbox || m.showInbox {
				m.inbox.nextFolder()
			}
		case "i":
			return m.openInbox()
		case "R":
			if m.focus == focusInbox && inboxFolders[m.inbox.folder] == "failed" {
				if env, ok := m.inbox.selected(); ok {
					if err := retryFailed(m.home, m.inbox.slug, env.ID); err != nil {
						m.status = "retry failed: " + err.Error()
					} else {
						m.status = "retry " + env.ID + " → pending"
						m.reload()
					}
				}
			}
		case "D":
			if m.disbandPending != "" {
				m2, cmd := m.disbandUnderCursor()
				mm := m2.(Model)
				mm.disbandPending = ""
				return mm, cmd
			}
		case "r":
			m.disbandPending = ""
			m.reload()
			if m.chatSlug != "" {
				m.reloadChat()
			}
			if m.chatGroup != "" {
				m.reloadGroupChat()
			}
		case "n":
			return m.openSpawnForm()
		case "g":
			return m.openGroupForm()
		case "p":
			return m.openProjectForm()
		case "enter":
			return m.openSelected()
		}
	}
	return m, nil
}

func (m Model) handleMouse(kind string, mouse tea.Mouse) (tea.Model, tea.Cmd) {
	sideW, _, bodyH := layout(m.width, m.height)
	if mouse.Y >= bodyH {
		return m, nil
	}
	if mouse.X < sideW {
		m.focus = focusSide
		if kind == "click" {
			idx := mouse.Y - 4 // title, subtitle, status, blank
			if idx >= 1 {      // idx 0 is the agents heading
				if idx <= 2*len(m.rows) {
					m.cursor = (idx - 1) / 2 // two lines per bot: name + title
				} else if len(m.groups) > 0 {
					off := idx - 2*len(m.rows) - 1 // 0 = groups heading, then two lines per room
					if off >= 1 {
						if k := (off - 1) / 2; k < len(m.groups) {
							m.cursor = len(m.rows) + k
						}
					}
				}
			}
		}
		return m, nil
	}
	if m.showInbox {
		m.focus = focusInbox
		return m, nil
	}
	if m.chatSlug != "" {
		m.focus = focusChat
		if kind == "wheel" {
			switch mouse.Button {
			case tea.MouseWheelUp:
				m.vp.ScrollUp(3)
			case tea.MouseWheelDown:
				m.vp.ScrollDown(3)
			}
		}
	}
	return m, nil
}

func (m Model) openInbox() (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.rows) {
		return m, nil
	}
	if len(m.rows) == 0 {
		return m, nil
	}
	m.showInbox = true
	m.focus = focusInbox
	m.reloadInbox()
	m.status = "inbox @" + m.inbox.slug
	return m, nil
}

func (m Model) quitHost() (tea.Model, tea.Cmd) {
	return m, tea.Quit
}

func (m *Model) sizeChat() {
	_, w, h := layout(m.width, m.height)
	th := h - 2
	if th < 3 {
		th = 3
	}
	m.vp.SetWidth(w - 1)
	m.vp.SetHeight(th)
	m.in.SetWidth(max(8, w-2))
}

func (m *Model) reloadChat() {
	if m.chatSlug == "" || len(m.rows) == 0 {
		return
	}
	var bot roster.Bot
	found := false
	for _, r := range m.rows {
		if r.bot.Slug == m.chatSlug {
			bot = r.bot
			found = true
			break
		}
	}
	if !found {
		m.chatSlug = ""
		m.chatBusy = false
		return
	}
	body, err := loadTranscript(m.home, bot)
	if err != nil {
		body = mutedStyle.Render(err.Error())
	}
	m.vp.SetContent(body)
	m.vp.GotoBottom()
}

func (m Model) openSelected() (tea.Model, tea.Cmd) {
	if g, ok := m.selectedGroup(); ok {
		m.showInbox = false
		m.chatSlug = ""
		m.chatGroup = g.ID
		m.focus = focusChat
		m.sizeChat()
		m.reloadGroupChat()
		m.status = "chat @" + g.ID
		return m, m.in.Focus()
	}
	if len(m.rows) == 0 {
		return m, nil
	}
	bot := m.rows[m.cursor].bot
	m.showInbox = false
	m.chatGroup = ""
	m.chatSlug = bot.Slug
	m.focus = focusChat
	m.sizeChat()
	m.reloadChat()
	m.status = "chat @" + bot.Slug
	cmd := m.in.Focus()
	return m, cmd
}

func (m Model) sendChat() (tea.Model, tea.Cmd) {
	if m.chatBusy {
		return m, nil
	}
	line := strings.TrimSpace(m.in.Value())
	if line == "" {
		return m, nil
	}
	if m.chatGroup != "" {
		m.in.SetValue("")
		m.chatBusy = true
		return m, m.sendGroup(line)
	}
	if m.chatSlug == "" {
		return m, nil
	}
	var bot roster.Bot
	for _, r := range m.rows {
		if r.bot.Slug == m.chatSlug {
			bot = r.bot
			break
		}
	}
	if bot.Slug == "" {
		return m, nil
	}
	m.in.SetValue("")
	m.in.Blur()
	m.chatBusy = true
	m.status = "running @" + bot.Slug
	root := m.home
	return m, func() tea.Msg {
		err := runSay(root, bot, line)
		return sayDoneMsg{slug: bot.Slug, err: err}
	}
}

func (m Model) View() tea.View {
	sideW, crushW, bodyH := layout(m.width, m.height)
	sidebar := m.sidebarView(sideW, bodyH)
	right := m.rightView(crushW, bodyH)
	div := m.divider(bodyH)
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, div, right)
	help := m.helpView(m.width)
	frame := lipgloss.JoinVertical(lipgloss.Left, body, help)
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m Model) sidebarView(width, height int) string {
	var b strings.Builder
	fmt.Fprintln(&b, brandLine())
	fmt.Fprintln(&b, mutedStyle.Render("Charm Crush Powered Bot Mesh"))
	fmt.Fprintln(&b, mutedStyle.Render(m.status))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, mutedStyle.Render("agents"))
	if len(m.rows) == 0 {
		fmt.Fprintln(&b, "No bots yet.")
		fmt.Fprintln(&b, mutedStyle.Render("press n to spawn"))
	}
	inner := width - 2 // sideStyle horizontal padding
	if inner < 1 {
		inner = 1
	}
	sel := selStyle.Width(inner)
	for i, r := range m.rows {
		mark := " "
		if i == m.cursor {
			mark = "▸"
		}
		open := ""
		if m.chatSlug == r.bot.Slug {
			open = " ●"
		}
		busy := ""
		if r.busy || (m.chatBusy && m.chatSlug == r.bot.Slug) {
			busy = " " + m.spinnerBot.View()
		}
		glyph := "  "
		if r.bot.Slug == "sophie" {
			glyph = "💜"
		}
		selected := i == m.cursor && m.focus == focusSide
		line := fmt.Sprintf("%s %s %s%s%s", mark, glyph, nameStyle.Render("@"+r.bot.Slug), open, busy)
		if r.pending > 0 {
			line += fmt.Sprintf("  %d", r.pending)
		}
		if selected {
			line = sel.Render(line)
		}
		fmt.Fprintln(&b, line)
		if r.bot.Title != "" {
			title := mutedStyle.Render("     " + r.bot.Title)
			if selected {
				title = sel.Render(title)
			}
			fmt.Fprintln(&b, title)
		}
	}
	if len(m.groups) > 0 {
		fmt.Fprintln(&b, mutedStyle.Render("groups"))
		for gi, g := range m.groups {
			i := len(m.rows) + gi
			mark := " "
			if i == m.cursor {
				mark = "▸"
			}
			selected := i == m.cursor && m.focus == focusSide
			line := fmt.Sprintf("%s %s %s", mark, glyphPlaceholder, nameStyle.Render("@"+g.ID))
			if m.groupBusy && m.chatGroup == g.ID {
				line += " " + m.spinnerGroup.View()
			}
			if selected {
				line = sel.Render(line)
			}
			fmt.Fprintln(&b, line)
			title := mutedStyle.Render(fmt.Sprintf("     %d members", len(g.Members)))
			if selected {
				title = sel.Render(title)
			}
			fmt.Fprintln(&b, title)
		}
	}
	return sideStyle.Width(width).Height(height).MaxHeight(height).MaxWidth(width).Render(b.String())
}

func (m Model) rightView(width, height int) string {
	if m.projectForm.active {
		return m.projectView(width, height)
	}
	if m.formActive() {
		return m.formView(width, height)
	}
	if m.paletteOpen {
		return m.paletteView(width, height)
	}
	if m.showInbox && m.inbox.slug != "" {
		return renderInbox(m.inbox, width, height)
	}
	if m.chatGroup != "" {
		return m.groupView(width, height)
	}
	return m.crushView(width, height)
}

func (m Model) crushView(width, height int) string {
	pad := lipgloss.NewStyle().PaddingLeft(1).Width(width).MaxWidth(width)
	if m.chatSlug == "" {
		hint := mutedStyle.Render("press enter to open a bot transcript")
		if len(m.rows) == 0 {
			hint = mutedStyle.Render("spawn a bot, then press enter")
		}
		lines := []string{hint}
		for len(lines) < height {
			lines = append(lines, "")
		}
		return pad.Render(strings.Join(lines[:height], "\n"))
	}
	head := nameStyle.Render("@"+m.chatSlug) + "  " + mutedStyle.Render("session")
	if m.chatBusy {
		head += "  " + m.spinnerBot.View() + " " + mutedStyle.Render("thinking")
	}
	return pad.Render(strings.Join([]string{head, m.vp.View(), m.in.View()}, "\n"))
}

func (m Model) divider(height int) string {
	st := divStyle
	if m.focus == focusChat || m.focus == focusInbox {
		st = divHotStyle
	}
	col := strings.Repeat("│\n", height)
	col = strings.TrimSuffix(col, "\n")
	return st.Width(dividerW).Height(height).Render(col)
}

func (m Model) helpView(width int) string {
	var s string
	switch {
	case m.focus == focusChat:
		s = fmt.Sprintf("%s send  %s scroll  %s list  %s quit",
			keyStyle.Render("enter"), keyStyle.Render("pgup/pgdn j/k"),
			keyStyle.Render("esc"), keyStyle.Render("ctrl+q"))
	case m.focus == focusInbox:
		s = fmt.Sprintf("%s move  %s folder  %s chat  %s retry  %s list  %s quit",
			keyStyle.Render("j/k"), keyStyle.Render("tab"), keyStyle.Render("enter"),
			keyStyle.Render("R"), keyStyle.Render("esc"), keyStyle.Render("q"))
	default:
		proj := ""
		if p := m.currentProject(); p != "" {
			proj = userStyle().Render("*" + projectDisplay(p))
		}
		s = fmt.Sprintf("%s move  %s chat  %s inbox  %s bot  %s group  %s project %s  %s refresh  %s quicklaunch  %s quit",
			keyStyle.Render("j/k"), keyStyle.Render("enter"), keyStyle.Render("i"),
			keyStyle.Render("n"), keyStyle.Render("g"), keyStyle.Render("p"), proj,
			keyStyle.Render("r"), keyStyle.Render(":"), keyStyle.Render("q"))
	}
	return helpStyle.Width(width).MaxWidth(width).Render(s)
}

func Run(home string) error {
	p := tea.NewProgram(New(home), tea.WithFilter(func(_ tea.Model, msg tea.Msg) tea.Msg {
		if _, ok := msg.(tea.InterruptMsg); ok {
			return tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'q'}
		}
		return msg
	}))
	_, err := p.Run()
	return err
}
