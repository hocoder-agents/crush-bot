package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hocoder-agents/crush-bot/internal/group"
	"github.com/hocoder-agents/crush-bot/internal/roster"
)

func TestSidebarTitleLayout(t *testing.T) {
	m := Model{
		rows: []row{
			{bot: roster.Bot{Slug: "sophie", Title: "Orchestrator"}},
			{bot: roster.Bot{Slug: "diana", Title: "Coder"}},
		},
		cursor: 0,
		focus:  focusSide,
	}
	out := m.sidebarView(24, 12)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	sel := -1
	for i, l := range lines {
		if strings.Contains(l, "@sophie") {
			sel = i
			break
		}
	}
	if sel == -1 {
		t.Fatalf("no sophie row:\n%s", out)
	}
	if sel+1 >= len(lines) || !strings.Contains(lines[sel+1], "Orchestrator") {
		t.Fatalf("selected bot title not on second line:\n%s", out)
	}
	diana := -1
	for i, l := range lines {
		if strings.Contains(l, "@diana") {
			diana = i
			break
		}
	}
	if diana == -1 || diana+1 >= len(lines) || !strings.Contains(lines[diana+1], "Coder") {
		t.Fatalf("unselected bot title not on its own line:\n%s", out)
	}
	if strings.Contains(lines[diana], "Coder") {
		t.Fatalf("unselected title should not share the name line:\n%s", out)
	}
}

func TestSidebarSelectionIsBackground(t *testing.T) {
	m := Model{
		rows:  []row{{bot: roster.Bot{Slug: "diana", Title: "Coder"}}},
		focus: focusSide,
	}
	out := m.sidebarView(24, 10)
	if !strings.Contains(out, "\x1b[48;2;58;52;85m") && !strings.Contains(out, "\x1b[48;5;") {
		t.Fatalf("selection has no background wash:\n%q", out)
	}
}

func TestSidebarOrchestratorFirst(t *testing.T) {
	home := t.TempDir()
	for _, s := range []string{"diana", "sophie", "andreea"} {
		if err := roster.Save(home, roster.Bot{Slug: s}); err != nil {
			t.Fatal(err)
		}
	}
	m := Model{home: home}
	m.reload()
	if len(m.rows) == 0 || m.rows[0].bot.Slug != "sophie" {
		t.Fatalf("sophie not first: %+v", m.rows)
	}
}

func TestDividerIsLavender(t *testing.T) {
	m := Model{}
	out := m.divider(5)
	if !strings.Contains(out, "\x1b[38;2;196;181;253m") {
		t.Fatalf("divider not lavender: %q", out)
	}
}

func TestCrushViewPaddedFromDivider(t *testing.T) {
	m := Model{rows: []row{{bot: roster.Bot{Slug: "diana", Title: "Coder"}}}, chatSlug: "diana"}
	out := m.crushView(40, 10)
	lines := strings.Split(out, "\n")
	for _, l := range lines {
		if l == "" {
			continue
		}
		if !strings.HasPrefix(l, " ") {
			t.Fatalf("transcript line not padded: %q", l)
		}
	}
}

func TestSidebarRoomsSection(t *testing.T) {
	home := t.TempDir()
	if err := roster.Save(home, roster.Bot{Slug: "diana", Title: "Coder"}); err != nil {
		t.Fatal(err)
	}
	if err := group.Save(home, group.Group{ID: "review", Members: []string{"diana", "natasha"}}); err != nil {
		t.Fatal(err)
	}
	m := Model{home: home, groups: []group.Group{{ID: "review", Members: []string{"diana", "natasha"}}}}
	out := m.sidebarView(24, 20)
	if !strings.Contains(out, "groups") || !strings.Contains(out, "@review") {
		t.Fatalf("rooms missing from sidebar:\n%s", out)
	}
}

func TestPaletteFilter(t *testing.T) {
	m := Model{rows: []row{{bot: roster.Bot{Slug: "diana", Title: "Coder"}}}}
	m.paletteQuery = "doc"
	acts := m.paletteFiltered()
	if len(acts) != 1 || acts[0].label != "doctor --check" {
		t.Fatalf("filter 'doc' wrong: %+v", acts)
	}
	m.paletteQuery = "mail"
	acts = m.paletteFiltered()
	if len(acts) != 1 || acts[0].label != "open mailbox" {
		t.Fatalf("filter 'mail' wrong: %+v", acts)
	}
	m.paletteQuery = "zzz"
	if len(m.paletteFiltered()) != 0 {
		t.Fatal("no filter match expected")
	}
	m.paletteQuery = ""
	if len(m.paletteFiltered()) != len(m.paletteActions()) {
		t.Fatal("empty query should show all")
	}
}

func TestPaletteKeyDownThenRun(t *testing.T) {
	m := Model{rows: []row{{bot: roster.Bot{Slug: "diana", Title: "Coder"}}}, paletteOpen: true, focus: focusSide}
	km := tea.KeyPressMsg{Code: 'j'}
	m2i, _ := m.updatePalette(km)
	m2 := m2i.(Model)
	if m2.paletteIdx != 1 {
		t.Fatalf("idx %d want 1", m2.paletteIdx)
	}
	m2.paletteQuery = "refresh"
	acts := m2.paletteFiltered()
	if len(acts) != 1 || acts[0].label != "refresh roster" {
		t.Fatalf("filter refresh wrong: %+v", acts)
	}
	m3i, _ := m2.updatePalette(tea.KeyPressMsg{Code: tea.KeyEnter})
	m3 := m3i.(Model)
	if m3.paletteOpen {
		t.Fatal("palette should be closed after run")
	}
}

func TestDisbandUnderCursor(t *testing.T) {
	home := t.TempDir()
	for _, s := range []string{"diana", "natasha"} {
		if err := roster.Save(home, roster.Bot{Slug: s}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := group.Create(home, "review", []string{"diana", "natasha"}); err != nil {
		t.Fatal(err)
	}
	m := Model{home: home}
	m.reload()
	m.cursor = len(m.rows) // first room
	m2i, _ := m.disbandUnderCursor()
	m2 := m2i.(Model)
	if _, err := os.Stat(group.Dir(home, "review")); !os.IsNotExist(err) {
		t.Fatalf("room still exists: %v", err)
	}
	if !strings.Contains(m2.status, "disbanded") {
		t.Fatalf("status %q", m2.status)
	}
}

func TestSpawnFormValidation(t *testing.T) {
	home := t.TempDir()
	if err := roster.Save(home, roster.Bot{Slug: "diana", Title: "Coder"}); err != nil {
		t.Fatal(err)
	}
	base := Model{home: home}
	mi, _ := base.openSpawnForm()
	m := mi.(Model)
	if !m.spawnForm.active {
		t.Fatal("form not active")
	}
	// bad slug
	m.spawnForm.slug.SetValue("1bot")
	m2i, _ := m.updateSpawnForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m2i, _ = m2i.(Model).updateSpawnForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m2i, _ = m2i.(Model).updateSpawnForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m2i, _ = m2i.(Model).updateSpawnForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m2 := m2i.(Model)
	if m2.spawnForm.err == "" {
		t.Fatal("invalid slug should set form error")
	}
	// existing slug
	m3i, _ := m.openSpawnForm()
	m3 := m3i.(Model)
	m3.spawnForm.slug.SetValue("diana")
	for i := 0; i < 4; i++ {
		m3i, _ = m3.updateSpawnForm(tea.KeyPressMsg{Code: tea.KeyEnter})
		m3 = m3i.(Model)
	}
	if m3.spawnForm.err == "" {
		t.Fatal("duplicate slug should set form error")
	}
}

func TestGroupFormTogglesAndGuard(t *testing.T) {
	home := t.TempDir()
	for _, s := range []string{"diana", "natasha"} {
		if err := roster.Save(home, roster.Bot{Slug: s}); err != nil {
			t.Fatal(err)
		}
	}
	base := Model{home: home}
	mi, _ := base.openGroupForm()
	m := mi.(Model)
	m.groupForm.id.SetValue("review")
	// advance to member picker
	mi, _ = m.updateGroupForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = mi.(Model)
	// submit with nothing chosen -> error, form stays
	mi, _ = m.updateGroupForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = mi.(Model)
	if m.groupForm.err == "" {
		t.Fatal("empty roster pick should error")
	}
	if !m.groupForm.active {
		t.Fatal("form should stay open on error")
	}
	// toggle first two bots, submit
	mi, _ = m.updateGroupForm(tea.KeyPressMsg{Code: ' '})
	m = mi.(Model)
	mi, _ = m.updateGroupForm(tea.KeyPressMsg{Code: 'j'})
	m = mi.(Model)
	mi, _ = m.updateGroupForm(tea.KeyPressMsg{Code: ' '})
	m = mi.(Model)
	mi, _ = m.updateGroupForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = mi.(Model)
	if m.groupForm.active {
		t.Fatal("form should close after create")
	}
	if _, err := os.Stat(group.Dir(home, "review")); err != nil {
		t.Fatalf("room not created: %v", err)
	}
	if m.rows[0].bot.Slug != "diana" {
		t.Fatalf("cursor not on new room: %+v", m.rows)
	}
}

func TestFormInputsFocusable(t *testing.T) {
	home := t.TempDir()
	if err := roster.Save(home, roster.Bot{Slug: "diana", Title: "Coder"}); err != nil {
		t.Fatal(err)
	}
	base := Model{home: home}
	mi, _ := base.openGroupForm()
	m := mi.(Model)
	mi, _ = m.updateGroupForm(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = mi.(Model)
	if m.groupForm.id.Value() != "r" {
		t.Fatalf("typing into room id failed: %q", m.groupForm.id.Value())
	}
	mi, _ = base.openSpawnForm()
	m = mi.(Model)
	mi, _ = m.updateSpawnForm(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = mi.(Model)
	if m.spawnForm.slug.Value() != "x" {
		t.Fatalf("typing into slug failed: %q", m.spawnForm.slug.Value())
	}
}

func TestSpinnerRendersInViews(t *testing.T) {
	m := Model{
		home:       t.TempDir(),
		spinnerBot: spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(lavender))),
		rows:       []row{{bot: roster.Bot{Slug: "diana", Title: "Coder"}, busy: true}},
		chatSlug:   "diana",
		chatBusy:   true,
	}
	out := m.sidebarView(24, 10)
	if !strings.ContainsAny(out, "⣾⣽⣻⡿⡿⡾⡟⡾⡷⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏") {
		t.Fatalf("no braille spinner in busy bot row:\n%s", out)
	}
	m.chatGroup = "review"
	out2 := m.crushView(40, 10)
	if !strings.Contains(out2, "thinking") {
		t.Fatalf("chat head missing thinking:\n%s", out2)
	}
}

func TestRoomTranscriptRendering(t *testing.T) {
	lines := []group.Line{
		{Round: 0, From: "user", Kind: "line", Body: "hash it out"},
		{Round: 1, From: "diana", Kind: "line", Body: "my take: slice it"},
		{Round: 1, From: "sophie", Kind: "pass", Body: "PASS", Pass: true},
		{Round: 1, From: "diana", Kind: "system", Body: "wake failed: boom"},
	}
	out := renderRoomTranscript(lines)
	if !strings.Contains(out, "you") {
		t.Fatalf("user line missing: %s", out)
	}
	if !strings.Contains(out, "@diana") || !strings.Contains(out, "my take") && !strings.Contains(out, "slice it") {
		t.Fatalf("bot line missing: %s", out)
	}
	if !strings.Contains(out, "round 1") {
		t.Fatalf("round divider missing: %s", out)
	}
	if !strings.Contains(out, "sophie passed") {
		t.Fatalf("pass line missing: %s", out)
	}
	if !strings.Contains(out, "wake failed") {
		t.Fatalf("system note missing: %s", out)
	}
	if !strings.Contains(out, "38;5;205m") {
		t.Fatalf("no ANSI color in speaker names: %s", out)
	}
}

func TestGitProjectsDiscovery(t *testing.T) {
	home := t.TempDir()
	mk := func(rel string, git bool) {
		dir := filepath.Join(home, rel)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if git {
			if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o700); err != nil {
				t.Fatal(err)
			}
		}
	}
	mk("repos/wrapmind/wrapmind", true)        // repo
	mk("repos/hocoder-agents/crush-bot", true) // repo
	mk("repos/wrapmind/notarepo", false)       // plain dir
	got := gitProjects([]string{home + "/repos"})
	if len(got) != 2 {
		t.Fatalf("want 2 repos, got %v", got)
	}
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, filepath.Join("wrapmind", "wrapmind")) {
		t.Fatalf("missing wrapmind: %v", got)
	}
	if !strings.Contains(joined, "crush-bot") {
		t.Fatalf("missing nested repo: %v", got)
	}
}

func TestApplyProjectUpdatesCrew(t *testing.T) {
	home := t.TempDir()
	for _, s := range []string{"diana", "natasha"} {
		if err := roster.Save(home, roster.Bot{Slug: s, Title: strings.ToUpper(s[:1]) + s[1:]}); err != nil {
			t.Fatal(err)
		}
	}
	m := Model{home: home}
	m.reload()
	dir := filepath.Join(home, "repos", "wrapmind", "wrapmind")
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	mi, _ := m.applyProject(dir)
	m = mi.(Model)
	for _, r := range m.rows {
		if r.bot.Project != dir {
			t.Fatalf("@%s project = %q", r.bot.Slug, r.bot.Project)
		}
	}
	bot, err := roster.Load(home, "diana")
	if err != nil {
		t.Fatal(err)
	}
	if bot.Project != dir {
		t.Fatalf("bot.yaml not updated: %+v", bot)
	}
	if _, err := os.Stat(filepath.Join(roster.Home(home, "diana"), "protocol.md")); err != nil {
		t.Fatalf("protocol not regenerated: %v", err)
	}
}

func TestProjectModalCapturesArrows(t *testing.T) {
	home := t.TempDir()
	for _, s := range []string{"diana", "natasha"} {
		if err := roster.Save(home, roster.Bot{Slug: s}); err != nil {
			t.Fatal(err)
		}
	}
	base := Model{home: home}
	base.reload()
	base.cursor = 0
	mi, _ := base.openProjectForm()
	m := mi.(Model)
	mi, _ = m.updateProjectForm(tea.KeyPressMsg{Code: tea.KeyDown})
	m = mi.(Model)
	if m.projectForm.cursor != 1 {
		t.Fatalf("modal cursor did not move: %d", m.projectForm.cursor)
	}
	if m.cursor != 0 {
		t.Fatalf("roster cursor moved behind modal: %d", m.cursor)
	}
	// through the full Update path too
	mi, cmd := m.Update(tea.KeyPressMsg{Code: 'j'})
	if mm := mi.(Model); mm.projectForm.cursor != 2 {
		t.Fatalf("j via Update did not move modal cursor: %d", mm.projectForm.cursor)
	}
	mi, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = mi.(Model)
	if m.projectForm.cursor != 0 {
		t.Fatalf("up arrow did not move modal cursor: %d", m.projectForm.cursor)
	}
	if m.cursor != 0 {
		t.Fatalf("roster cursor moved via Update: %d", m.cursor)
	}
	_ = cmd
}

func TestProjectModalKeysAndFind(t *testing.T) {
	home := t.TempDir()
	if err := roster.Save(home, roster.Bot{Slug: "diana"}); err != nil {
		t.Fatal(err)
	}
	// two repos under home, one outside project_roots
	mk := func(rel string) {
		if err := os.MkdirAll(filepath.Join(home, rel, ".git"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	mk("other/wrapmind/wrapmind")
	mk("repos/dogfood/app")
	mk("repos/dogfood/app2")
	// parent dirs of repos shouldn't count
	base := Model{home: home}
	base.reload()
	t.Setenv("HOME", home) // find mode scans ~; keep it hermetic
	mi, _ := base.openProjectForm()
	m := mi.(Model)
	if !m.projectForm.active {
		t.Fatal("modal not open")
	}
	// arrows captured via the full Update path
	mi, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = mi.(Model)
	if m.projectForm.cursor != 1 {
		t.Fatalf("down did not move modal cursor: %d", m.projectForm.cursor)
	}
	if m.cursor != 0 {
		t.Fatalf("roster cursor moved behind modal: %d", m.cursor)
	}
	// find mode: / + query reaches the repo outside project_roots
	mi, _ = m.updateProjectForm(tea.KeyPressMsg{Code: '/'})
	m = mi.(Model)
	if !m.projectForm.find {
		t.Fatal("/ did not enter find mode")
	}
	for _, r := range []rune("wrapmind") {
		mi, _ = m.updateProjectForm(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = mi.(Model)
	}
	if len(m.projectForm.items) != 1 || m.projectForm.items[0] != filepath.Join(home, "other", "wrapmind", "wrapmind") {
		t.Fatalf("find results wrong: %v", m.projectForm.items)
	}
	mi, _ = m.updateProjectForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = mi.(Model)
	if m.projectForm.active {
		t.Fatal("modal should close after apply")
	}
	if got := m.rows[0].bot.Project; got != filepath.Join(home, "other", "wrapmind", "wrapmind") {
		t.Fatalf("project not applied: %q", got)
	}
	// esc in find mode steps back to the list
	mi, _ = m.openProjectForm()
	m = mi.(Model)
	mi, _ = m.updateProjectForm(tea.KeyPressMsg{Code: '/'})
	m = mi.(Model)
	mi, _ = m.updateProjectForm(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = mi.(Model)
	if m.projectForm.find || !m.projectForm.active {
		t.Fatalf("esc should leave find mode but keep the modal open")
	}
}
