package ui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/hocoder-agents/crush-bot/internal/config"
	"github.com/hocoder-agents/crush-bot/internal/crush"
	"github.com/hocoder-agents/crush-bot/internal/group"
	"github.com/hocoder-agents/crush-bot/internal/roster"
	"github.com/hocoder-agents/crush-bot/internal/spawn"
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
		head += "  " + mutedStyle.Render("round running…")
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

type groupCreatedMsg struct {
	id  string
	err error
}

// groupWizard is the tea.Exec form for creating a room: room id plus a
// member pick, line-based because tea.Exec's stdin is a cancelreader that
// breaks Huh's interactive TUI (same reason the spawn form is accessible).
type groupWizard struct {
	home string
	id   string
	in   io.Reader
	out  io.Writer
	err  io.Writer
}

func (w *groupWizard) Run() error {
	tty, err := spawn.OpenTTY()
	if err != nil {
		return fmt.Errorf("group form needs a terminal: %w", err)
	}
	defer tty.Close()
	in, out := io.Reader(tty), io.Writer(tty)
	bots, _ := roster.List(w.home, false)
	if len(bots) < 2 {
		return fmt.Errorf("need at least two bots on the roster; spawn more first")
	}
	r := bufio.NewScanner(in)
	fmt.Fprintln(out, brandLine())
	fmt.Fprintln(out, mutedStyle.Render("new group — esc/ctrl+c aborts"))
	fmt.Fprintln(out, mutedStyle.Render("room id (lowercase, e.g. review)"))
	fmt.Fprint(out, "> ")
	if !r.Scan() {
		return huh.ErrUserAborted
	}
	id := spawn.NormalizeSlug(r.Text())
	if !roster.ValidSlug(id) {
		return fmt.Errorf("invalid room id %q", id)
	}
	fmt.Fprintln(out)
	for i, b := range bots {
		fmt.Fprintf(out, "%s\n", mutedStyle.Render(fmt.Sprintf("  %2d  @%s  %s", i+1, b.Slug, b.Title)))
	}
	fmt.Fprintln(out, mutedStyle.Render("members — 2 to 6, pick by number (e.g. 1 3 4)"))
	fmt.Fprint(out, "> ")
	if !r.Scan() {
		return huh.ErrUserAborted
	}
	members, err := parseMembers(r.Text(), botSlugs(bots))
	if err != nil {
		return err
	}
	g, err := group.Create(w.home, id, members)
	if err != nil {
		return err
	}
	w.id = g.ID
	return nil
}

func (w *groupWizard) SetStdin(r io.Reader)  { w.in = r }
func (w *groupWizard) SetStdout(o io.Writer) { w.out = o }
func (w *groupWizard) SetStderr(e io.Writer) { w.err = e }

func botSlugs(bots []roster.Bot) []string {
	out := make([]string, len(bots))
	for i, b := range bots {
		out[i] = b.Slug
	}
	return out
}

// parseMembers takes "1 3 4" or "diana, masha" against the roster order.
func parseMembers(sel string, slugs []string) ([]string, error) {
	fields := strings.FieldsFunc(sel, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t'
	})
	var out []string
	for _, f := range fields {
		if n, err := strconv.Atoi(f); err == nil {
			if n < 1 || n > len(slugs) {
				return nil, fmt.Errorf("no bot %d on the roster", n)
			}
			out = append(out, slugs[n-1])
			continue
		}
		f = strings.TrimPrefix(f, "@")
		found := false
		for _, s := range slugs {
			if s == f {
				out = append(out, s)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown bot %q", f)
		}
	}
	if len(out) < 2 || len(out) > 6 {
		return nil, fmt.Errorf("pick 2 to 6 members")
	}
	return out, nil
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
