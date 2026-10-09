package spawn

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"

	"github.com/hocoder-agents/crush-bot/internal/roster"
)

// ErrAborted is huh's user-cancelled error.
var ErrAborted = huh.ErrUserAborted

func NormalizeSlug(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "@")
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	return s
}

func Form(slug, title, desc *string, coder *bool) error {
	return FormIO(nil, nil, slug, title, desc, coder)
}

// FormAccessible is line-based (no nested Bubble Tea). Use it from the mesh
// TUI: tea.Exec's stdin is a cancelreader, which aborts Huh's TUI immediately.
func FormAccessible(in io.Reader, out io.Writer, slug, title, desc *string, coder *bool) error {
	return formIO(in, out, true, slug, title, desc, coder)
}

func FormIO(in io.Reader, out io.Writer, slug, title, desc *string, coder *bool) error {
	return formIO(in, out, false, slug, title, desc, coder)
}

func formIO(in io.Reader, out io.Writer, accessible bool, slug, title, desc *string, coder *bool) error {
	if slug == nil || title == nil || desc == nil || coder == nil {
		return fmt.Errorf("form: nil field")
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Slug").
				Description("lowercase handle, e.g. researcher").
				Value(slug).
				Validate(func(s string) error {
					s = NormalizeSlug(s)
					if !roster.ValidSlug(s) {
						return fmt.Errorf("use a lowercase slug like researcher (a-z, 0-9, hyphens)")
					}
					return nil
				}),
			huh.NewInput().Title("Title").Value(title),
			huh.NewText().Title("Description").Value(desc),
			huh.NewConfirm().Title("Coder bot?").Description("bash/edit, sandboxed on Linux").Value(coder),
		),
	).WithAccessible(accessible)
	if in != nil {
		form = form.WithInput(in)
	}
	if out != nil {
		form = form.WithOutput(out)
	}
	if err := form.Run(); err != nil {
		return err
	}
	*slug = NormalizeSlug(*slug)
	if *title == "" && *slug != "" {
		*title = strings.ToUpper((*slug)[:1]) + (*slug)[1:]
	}
	return nil
}

// EditValues holds the live field pointers for an edit form.
type EditValues struct {
	Title, Description, Model, Project   string
	Coder, Bash, Edit, KeepAlive, Hidden bool
}

// NewEditForm builds a huh form bound to the given values. Use it with
// EditResult below to run the form as a modal inside a Bubble Tea program
// (state == StateCompleted saves, StateAborted cancels).
func NewEditForm(v *EditValues) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Title").Value(&v.Title),
			huh.NewInput().Title("Model").Description("Crush model id, empty = default").Value(&v.Model),
		),
		huh.NewGroup(
			huh.NewText().Title("Description").Value(&v.Description),
			huh.NewInput().Title("Project").Description("absolute path, empty = none").Value(&v.Project).
				Validate(func(s string) error {
					if strings.TrimSpace(s) != "" && !filepath.IsAbs(strings.TrimSpace(s)) {
						return fmt.Errorf("project must be an absolute path")
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewConfirm().Title("Coder bot (bash + edit)?").Description("sets both tool flags").Value(&v.Coder),
			huh.NewConfirm().Title("Allow bash?").Value(&v.Bash),
			huh.NewConfirm().Title("Allow edit?").Value(&v.Edit),
			huh.NewConfirm().Title("Keepalive crush server?").Value(&v.KeepAlive),
			huh.NewConfirm().Title("Hidden?").Value(&v.Hidden),
		),
	)
}

// EditResult is a bot patch derived from an EditValues snapshot; each Set
// field is written back to bot.yaml by roster.Update.
type EditResult struct {
	Title       roster.Opt[string]
	Description roster.Opt[string]
	Model       roster.Opt[string]
	Project     roster.Opt[string]
	KeepAlive   roster.Opt[bool]
	Coder       roster.Opt[bool]
	Bash        roster.Opt[bool]
	Edit        roster.Opt[bool]
	Hidden      roster.Opt[bool]
}

// EditResultFrom converts an EditValues snapshot into the patch set.
func EditResultFrom(v *EditValues) EditResult {
	return EditResult{
		Title:       roster.StrOpt(strings.TrimSpace(v.Title)),
		Description: roster.StrOpt(strings.TrimSpace(v.Description)),
		Model:       roster.StrOpt(strings.TrimSpace(v.Model)),
		Project:     roster.StrOpt(strings.TrimSpace(v.Project)),
		KeepAlive:   roster.BoolOpt(v.KeepAlive),
		Coder:       roster.BoolOpt(v.Coder),
		Bash:        roster.BoolOpt(v.Bash),
		Edit:        roster.BoolOpt(v.Edit),
		Hidden:      roster.BoolOpt(v.Hidden),
	}
}

// EditValuesFrom seeds an EditValues snapshot from a bot.
func EditValuesFrom(b roster.Bot) EditValues {
	return EditValues{
		Title:       b.Title,
		Model:       b.Model,
		Description: b.Description,
		Project:     b.Project,
		Coder:       b.Tools.Bash && b.Tools.Edit,
		Bash:        b.Tools.Bash,
		Edit:        b.Tools.Edit,
		KeepAlive:   b.KeepAlive,
		Hidden:      b.Hidden,
	}
}

// EditFormIO runs the edit form standalone (blocking). The mesh TUI uses
// NewEditForm to embed the same fields as a modal instead.
func EditFormIO(in io.Reader, out io.Writer, b roster.Bot) (EditResult, error) {
	v := EditValuesFrom(b)
	form := NewEditForm(&v).WithAccessible(false)
	if in != nil {
		form = form.WithInput(in)
	}
	if out != nil {
		form = form.WithOutput(out)
	}
	if err := form.Run(); err != nil {
		return EditResult{}, err
	}
	return EditResultFrom(&v), nil
}

// EditFormAccessible is the line-based variant for scripts and pipes.
func EditFormAccessible(in io.Reader, out io.Writer, b roster.Bot) (EditResult, error) {
	v := EditValuesFrom(b)
	form := NewEditForm(&v).WithAccessible(true)
	if in != nil {
		form = form.WithInput(in)
	}
	if out != nil {
		form = form.WithOutput(out)
	}
	if err := form.Run(); err != nil {
		return EditResult{}, err
	}
	return EditResultFrom(&v), nil
}

func OpenTTY() (*os.File, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}
