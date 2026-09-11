package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hocoder-agents/crush-bot/internal/config"
	"github.com/hocoder-agents/crush-bot/internal/protocol"
	"github.com/hocoder-agents/crush-bot/internal/roster"
)

type projectFormState struct {
	active bool
	items  []string // absolute repo paths
	cursor int
	err    string
}

// gitProjects walks the configured project roots (depth 2) collecting
// directories that look like git checkouts.
func gitProjects(roots []string) []string {
	home, _ := os.UserHomeDir()
	var out []string
	seen := map[string]bool{}
	expand := func(p string) string {
		if strings.HasPrefix(p, "~/") && home != "" {
			return filepath.Join(home, p[2:])
		}
		return p
	}
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > 2 {
			return
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil && !seen[dir] {
			seen[dir] = true
			out = append(out, dir)
		}
		if depth == 2 {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				walk(filepath.Join(dir, e.Name()), depth+1)
			}
		}
	}
	for _, r := range roots {
		walk(expand(r), 0)
	}
	return out
}

func projectDisplay(dir string) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(dir, home) {
		return "~" + strings.TrimPrefix(dir, home)
	}
	return dir
}

func (m Model) openProjectForm() (tea.Model, tea.Cmd) {
	cfg, err := config.Load(config.ResolvePaths())
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	items := gitProjects(cfg.ProjectRoots)
	if len(items) == 0 {
		m.status = "no git repos found under project roots (" + strings.Join(cfg.ProjectRoots, ", ") + ")"
		return m, nil
	}
	m.projectForm = projectFormState{active: true, items: items}
	m.status = "pick a project for the crew"
	return m, nil
}

func (m Model) updateProjectForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	f := &m.projectForm
	switch msg.String() {
	case "esc":
		f.active = false
		m.status = "cancelled"
		return m, nil
	case "enter":
		return m.applyProject(f.items[f.cursor])
	case "j", "down":
		if f.cursor < len(f.items)-1 {
			f.cursor++
		}
		return m, nil
	case "k", "up":
		if f.cursor > 0 {
			f.cursor--
		}
		return m, nil
	}
	return m, nil
}

func (m Model) projectView(width, height int) string {
	f := m.projectForm
	var b strings.Builder
	fmt.Fprintln(&b, gradientText("project")+"  "+mutedStyle.Render("aim the crew at a repo"))
	fmt.Fprintln(&b)
	for i, dir := range f.items {
		mark := " "
		if i == f.cursor {
			mark = "▸"
		}
		line := fmt.Sprintf("%s %s", mark, nameStyle.Render(projectDisplay(dir)))
		cur := m.currentProject()
		if cur == dir {
			line += "  " + userStyle().Render("*current")
		}
		if i == f.cursor {
			line = selStyle.Width(width - 4).Render(line)
		}
		fmt.Fprintln(&b, line)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, mutedStyle.Render("enter select · esc cancel"))
	box := modalStyle.Render(b.String())
	return lipgloss.Place(width, height,
		lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Background(lipgloss.Color("#191622"))),
	)
}

// currentProject returns the project of the bot under the cursor, if any.
func (m Model) currentProject() string {
	if m.cursor < len(m.rows) {
		return m.rows[m.cursor].bot.Project
	}
	return ""
}

// applyProject points every visible bot at dir and regenerates their
// protocol files so the advisory context lands right away.
func (m Model) applyProject(dir string) (tea.Model, tea.Cmd) {
	m.projectForm = projectFormState{}
	cfg, err := config.Load(config.ResolvePaths())
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	exe, _ := os.Executable()
	bots, err := roster.List(m.home, false)
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	n := 0
	for _, b := range bots {
		if b.Project == dir {
			continue
		}
		updated, err := roster.SetProject(m.home, b.Slug, dir)
		if err != nil {
			m.status = "@" + b.Slug + ": " + err.Error()
			return m, nil
		}
		if err := protocol.Write(protocol.Options{
			Root:         m.home,
			Bot:          updated,
			Teammates:    bots,
			Tasks:        cfg.Experimental.Tasks,
			Groups:       true,
			IncludeMCP:   true,
			CrushbotPath: exe,
			SoulMax:      cfg.SoulMaxBytes,
		}); err != nil {
			m.status = "protocol: " + err.Error()
			return m, nil
		}
		n++
	}
	m.reload()
	m.status = fmt.Sprintf("project *%s for %d bots", projectDisplay(dir), n)
	return m, nil
}
