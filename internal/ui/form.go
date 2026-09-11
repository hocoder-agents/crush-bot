package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hocoder-agents/crush-bot/internal/config"
	"github.com/hocoder-agents/crush-bot/internal/group"
	"github.com/hocoder-agents/crush-bot/internal/roster"
	"github.com/hocoder-agents/crush-bot/internal/spawn"
	"github.com/hocoder-agents/crush-bot/internal/version"
)

// In-TUI form state. Forms render as centered modals in the right pane;
// no tea.Exec, no dropping out to the terminal.

type spawnFormState struct {
	active bool
	step   int // 0 slug, 1 title, 2 description, 3 coder toggle
	slug   textinput.Model
	title  textinput.Model
	desc   textinput.Model
	coder  bool
	err    string
}

type groupFormState struct {
	active bool
	step   int // 0 room id, 1 member picker
	id     textinput.Model
	bots   []roster.Bot
	cursor int
	chosen map[string]bool
	err    string
}

func newFormInput(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	return ti
}

func (m Model) openSpawnForm() (tea.Model, tea.Cmd) {
	m.spawnForm = spawnFormState{
		active: true,
		step:   0,
		slug:   newFormInput("lowercase handle, e.g. researcher"),
		title:  newFormInput("display name, e.g. Coder"),
		desc:   newFormInput("one-line role"),
	}
	cmd := m.spawnForm.slug.Focus()
	m.status = "new bot"
	return m, cmd
}

func (m Model) openGroupForm() (tea.Model, tea.Cmd) {
	bots, _ := roster.List(m.home, false)
	m.groupForm = groupFormState{
		active: true,
		step:   0,
		id:     newFormInput("lowercase id, e.g. review"),
		bots:   bots,
		chosen: map[string]bool{},
	}
	cmd := m.groupForm.id.Focus()
	m.status = "new group"
	return m, cmd
}

func (m Model) formActive() bool {
	return m.spawnForm.active || m.groupForm.active
}

func formErrView(err string) string {
	if err == "" {
		return ""
	}
	return lipgloss.NewStyle().Foreground(pink).Render(err)
}
func (m Model) formView(width, height int) string {
	inner := width - 8
	if inner < 16 {
		inner = 16
	}
	if inner > 52 {
		inner = 52
	}
	var b strings.Builder
	var hint string
	if m.spawnForm.active {
		fmt.Fprintln(&b, gradientText("new bot")+"  "+mutedStyle.Render(version.Version))
		f := func(label string, in textinput.Model, focus bool) {
			mark := "  "
			if focus {
				mark = keyStyle.Render("▸ ")
			}
			row := fmt.Sprintf("%s%s  %s", mark, mutedStyle.Render(label), in.View())
			if focus {
				row = nameStyle.Render(row)
			}
			fmt.Fprintln(&b, row)
		}
		f("slug", m.spawnForm.slug, m.spawnForm.step == 0)
		f("title", m.spawnForm.title, m.spawnForm.step == 1)
		f("role", m.spawnForm.desc, m.spawnForm.step == 2)
		box := "[ ]"
		if m.spawnForm.coder {
			box = "[x]"
		}
		mark := "  "
		if m.spawnForm.step == 3 {
			mark = keyStyle.Render("▸ ")
		}
		fmt.Fprintf(&b, "%s%s  %s coder bot\n", mark, mutedStyle.Render("tools"), box)
		fmt.Fprintln(&b, mutedStyle.Render("        bash/edit, sandboxed on Linux"))
		hint = "space toggle coder · enter next · esc cancel"
	} else {
		fmt.Fprintln(&b, gradientText("new group")+"  "+mutedStyle.Render(version.Version))
		f := func(label string, in textinput.Model, focus bool) {
			mark := "  "
			if focus {
				mark = keyStyle.Render("▸ ")
			}
			fmt.Fprintf(&b, "%s%s  %s\n", mark, mutedStyle.Render(label), in.View())
		}
		f("room", m.groupForm.id, m.groupForm.step == 0)
		fmt.Fprintln(&b, mutedStyle.Render("members — 2 to 6, space toggles"))
		for i, bot := range m.groupForm.bots {
			box := "[ ]"
			if m.groupForm.chosen[bot.Slug] {
				box = "[x]"
			}
			mark := "  "
			focus := m.groupForm.step == 1 && i == m.groupForm.cursor
			if focus {
				mark = keyStyle.Render("▸ ")
			}
			line := fmt.Sprintf("%s%s  @%s  %s", mark, box, bot.Slug, mutedStyle.Render(bot.Title))
			if focus {
				line = nameStyle.Render(line)
			}
			fmt.Fprintln(&b, line)
		}
		hint = "j/k move · space toggle · enter create · esc cancel"
	}
	if msg := formErrView(m.spawnForm.err + m.groupForm.err); msg != "" {
		fmt.Fprintln(&b, msg)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, mutedStyle.Render(hint))
	box := modalStyle.Render(b.String())
	return lipgloss.Place(width, height,
		lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Background(lipgloss.Color("#191622"))),
	)
}

func (m Model) updateSpawnForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	f := &m.spawnForm
	if msg.String() == "esc" {
		f.active = false
		m.status = "cancelled"
		return m, nil
	}
	if msg.String() == " " && f.step == 3 {
		f.coder = !f.coder
		return m, nil
	}
	if msg.String() == "enter" {
		switch f.step {
		case 0:
			if strings.TrimSpace(f.slug.Value()) == "" {
				f.err = "pick a slug"
				return m, nil
			}
			f.step = 1
			return m, f.title.Focus()
		case 1:
			f.step = 2
			return m, f.desc.Focus()
		case 2:
			f.step = 3
			return m, nil
		case 3:
			return m.submitSpawnForm()
		}
		return m, nil
	}
	var cmd tea.Cmd
	switch f.step {
	case 0:
		f.slug, cmd = f.slug.Update(msg)
	case 1:
		f.title, cmd = f.title.Update(msg)
	case 2:
		f.desc, cmd = f.desc.Update(msg)
	}
	return m, cmd
}

func (m Model) submitSpawnForm() (tea.Model, tea.Cmd) {
	f := m.spawnForm
	slug := spawn.NormalizeSlug(f.slug.Value())
	if !roster.ValidSlug(slug) {
		m.spawnForm.err = "slug must be lowercase (a-z, 0-9, hyphens)"
		return m, nil
	}
	if roster.Exists(m.home, slug) {
		m.spawnForm.err = "@" + slug + " already exists"
		return m, nil
	}
	m.spawnForm = spawnFormState{}
	m.status = "spawning @" + slug + "…"
	root := m.home
	coder := f.coder
	title := strings.TrimSpace(f.title.Value())
	desc := strings.TrimSpace(f.desc.Value())
	return m, func() tea.Msg {
		cfg, err := config.Load(config.ResolvePaths())
		if err != nil {
			return spawnDoneMsg{err: err, slug: slug}
		}
		res, err := spawn.Create(root, cfg, spawn.Opts{
			Slug:        slug,
			Title:       title,
			Description: desc,
			Coder:       coder,
		})
		if err != nil {
			return spawnDoneMsg{err: err, slug: slug}
		}
		for _, w := range res.Warns {
			_ = w
		}
		if res.BootErr != nil {
			return spawnDoneMsg{err: res.BootErr, slug: res.Bot.Slug}
		}
		return spawnDoneMsg{slug: res.Bot.Slug}
	}
}

func (m Model) updateGroupForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	g := &m.groupForm
	switch msg.String() {
	case "esc":
		g.active = false
		m.status = "cancelled"
		return m, nil
	case "enter":
		if g.step == 0 {
			g.step = 1
			return m, nil
		}
		return m.submitGroupForm()
	case "j", "down":
		if g.step == 1 && g.cursor < len(g.bots)-1 {
			g.cursor++
		}
		return m, nil
	case "k", "up":
		if g.step == 1 && g.cursor > 0 {
			g.cursor--
		}
		return m, nil
	case " ", "space":
		if g.step == 1 && g.cursor < len(g.bots) {
			slug := g.bots[g.cursor].Slug
			g.chosen[slug] = !g.chosen[slug]
			g.err = ""
		}
		return m, nil
	}
	var cmd tea.Cmd
	if g.step == 0 {
		g.id, cmd = g.id.Update(msg)
	}
	return m, cmd
}

func (m Model) submitGroupForm() (tea.Model, tea.Cmd) {
	g := m.groupForm
	id := spawn.NormalizeSlug(g.id.Value())
	var members []string
	for _, b := range g.bots {
		if g.chosen[b.Slug] {
			members = append(members, b.Slug)
		}
	}
	created, err := group.Create(m.home, id, members)
	if err != nil {
		m.groupForm.err = err.Error()
		return m, nil
	}
	m.groupForm = groupFormState{}
	m.reload()
	for i, gr := range m.groups {
		if gr.ID == created.ID {
			m.cursor = len(m.rows) + i
			break
		}
	}
	m.status = "created @" + created.ID + " — enter to open"
	return m, nil
}
