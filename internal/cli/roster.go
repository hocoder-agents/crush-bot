package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/hocoder-agents/crush-bot/internal/config"
	"github.com/hocoder-agents/crush-bot/internal/crush"
	"github.com/hocoder-agents/crush-bot/internal/preset"
	"github.com/hocoder-agents/crush-bot/internal/roster"
	"github.com/hocoder-agents/crush-bot/internal/soul"
	"github.com/hocoder-agents/crush-bot/internal/spawn"
)

func cmdSpawn(io IO, args []string) int {
	fs := flag.NewFlagSet("spawn", flag.ContinueOnError)
	fs.SetOutput(io.Err)
	title := fs.String("title", "", "display title")
	desc := fs.String("description", "", "one-line role")
	model := fs.String("model", "", "Crush model id")
	project := fs.String("project", "", "absolute project path (advisory, not Crush cwd)")
	cloneFrom := fs.String("clone-from", "", "copy soul and settings from slug")
	coder := fs.Bool("coder", false, "enable bash and edit tools")
	sandboxOff := fs.Bool("sandbox-off", false, "disable bwrap sandbox (dangerous)")
	keepAlive := fs.Bool("keepalive", false, "keep a crush server warm for this bot")
	var slug string
	var flagArgs []string
	if len(args) == 0 {
		if !interactive() {
			fmt.Fprintln(io.Err, errStyle.Render("usage: crushbot spawn <slug> [flags]"))
			return 2
		}
	} else {
		var err error
		slug, flagArgs, err = slugThenFlags(args)
		if err != nil {
			fmt.Fprintln(io.Err, errStyle.Render("usage: crushbot spawn <slug> [flags]"))
			return 2
		}
		if err := fs.Parse(flagArgs); err != nil {
			return 2
		}
		if fs.NArg() != 0 {
			fmt.Fprintln(io.Err, errStyle.Render("usage: crushbot spawn <slug> [flags]"))
			return 2
		}
	}
	presetSoul := ""
	var presetTools *roster.Tools
	if *cloneFrom == "" && slug != "" {
		if entry, soulBody, ok := preset.Get(spawn.NormalizeSlug(slug)); ok {
			if *title == "" {
				*title = entry.Title
			}
			if *desc == "" {
				*desc = entry.Description
			}
			if !*coder {
				*coder = entry.Coder
				if entry.Coder || entry.Bash || entry.Edit {
					presetTools = &roster.Tools{Bash: entry.Coder || entry.Bash, Edit: entry.Coder || entry.Edit}
				}
			}
			presetSoul = soulBody
		}
	}
	if slug == "" || (*title == "" && *desc == "" && !*coder && interactive()) {
		s, t, d, c := slug, *title, *desc, *coder
		if err := spawn.Form(&s, &t, &d, &c); err != nil {
			return fail(io, err)
		}
		slug, *title, *desc, *coder = s, t, d, c
	}
	slug = spawn.NormalizeSlug(slug)
	if slug == "" {
		fmt.Fprintln(io.Err, errStyle.Render("usage: crushbot spawn <slug> [flags]"))
		return 2
	}
	p := config.ResolvePaths()
	cfg, err := config.Load(p)
	if err != nil {
		return fail(io, err)
	}
	if err := config.EnsureHome(p); err != nil {
		return fail(io, err)
	}
	sandboxMode := ""
	if *sandboxOff {
		sandboxMode = "off"
	}
	res, err := spawn.Create(p.Home, cfg, spawn.Opts{
		Slug:        slug,
		Title:       *title,
		Description: *desc,
		Model:       *model,
		Project:     *project,
		CloneFrom:   *cloneFrom,
		Coder:       *coder,
		Sandbox:     sandboxMode,
		KeepAlive:   *keepAlive,
		Soul:        presetSoul,
		Tools:       presetTools,
	})
	if err != nil {
		return fail(io, err)
	}
	for _, w := range res.Warns {
		fmt.Fprintln(io.Err, mutedStyle.Render("warning: "+w))
	}
	if res.BootErr != nil {
		fmt.Fprintln(io.Err, mutedStyle.Render("warning: crush bootstrap: "+res.BootErr.Error()))
		fmt.Fprintln(io.Err, mutedStyle.Render("bot files exist; crushbot doctor "+res.Bot.Slug))
	}
	fmt.Fprintln(io.Out, okStyle.Render("spawned "+res.Bot.Slug))
	fmt.Fprintln(io.Out, "  home", roster.Home(p.Home, res.Bot.Slug))
	fmt.Fprintln(io.Out, "  soul", roster.SoulPath(p.Home, res.Bot.Slug))
	if res.Bot.CanonicalSessionID != "" {
		fmt.Fprintln(io.Out, "  session", res.Bot.CanonicalSessionID)
	}
	if res.Bot.KeepAlive {
		fmt.Fprintln(io.Out, "  keepalive", crush.HostURL(roster.Home(p.Home, res.Bot.Slug)))
	}
	return 0
}

func cmdList(io IO, args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(io.Err)
	asJSON := fs.Bool("json", false, "JSON output")
	all := fs.Bool("all", false, "include hidden")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	p := config.ResolvePaths()
	bots, err := roster.List(p.Home, *all)
	if err != nil {
		return fail(io, err)
	}
	if *asJSON {
		enc := json.NewEncoder(io.Out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(bots); err != nil {
			return fail(io, err)
		}
		return 0
	}
	if len(bots) == 0 {
		fmt.Fprintln(io.Out, mutedStyle.Render("no bots"))
		return 0
	}
	w := tabwriter.NewWriter(io.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SLUG\tTITLE\tMODEL\tHIDDEN\tPROJECT")
	for _, b := range bots {
		model := b.Model
		if model == "" {
			model = "-"
		}
		proj := b.Project
		if proj == "" {
			proj = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%v\t%s\n", b.Slug, b.Title, model, b.Hidden, proj)
	}
	if err := w.Flush(); err != nil {
		return fail(io, err)
	}
	return 0
}

func cmdShow(io IO, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(io.Err, errStyle.Render("usage: crushbot show <slug>"))
		return 2
	}
	p := config.ResolvePaths()
	bot, err := roster.Load(p.Home, args[0])
	if err != nil {
		return fail(io, err)
	}
	fmt.Fprintf(io.Out, "%s  %s\n", cmdStyle.Render(bot.Slug), bot.Title)
	if bot.Description != "" {
		fmt.Fprintln(io.Out, bot.Description)
	}
	fmt.Fprintln(io.Out, mutedStyle.Render("soul     "+roster.SoulPath(p.Home, bot.Slug)))
	fmt.Fprintln(io.Out, mutedStyle.Render("model    "+emptyDash(bot.Model)))
	fmt.Fprintln(io.Out, mutedStyle.Render("project  "+emptyDash(bot.Project)))
	fmt.Fprintln(io.Out, mutedStyle.Render("hidden   "+fmt.Sprint(bot.Hidden)))
	fmt.Fprintln(io.Out, mutedStyle.Render("coder    "+fmt.Sprintf("bash=%v edit=%v", bot.Tools.Bash, bot.Tools.Edit)))
	return 0
}

func cmdSoul(io IO, args []string) int {
	fs := flag.NewFlagSet("soul", flag.ContinueOnError)
	fs.SetOutput(io.Err)
	edit := fs.Bool("edit", false, "open in $EDITOR")
	slug, flagArgs, err := slugThenFlags(args)
	if err != nil {
		fmt.Fprintln(io.Err, errStyle.Render("usage: crushbot soul <slug> [--edit]"))
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	p := config.ResolvePaths()
	cfg, err := config.Load(p)
	if err != nil {
		return fail(io, err)
	}
	path := roster.SoulPath(p.Home, slug)
	if _, err := roster.Load(p.Home, slug); err != nil {
		return fail(io, err)
	}
	if *edit {
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		cmd := exec.Command(editor, path)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fail(io, err)
		}
		bot, warns, err := roster.RefreshSoulHash(p.Home, slug, cfg.SoulMaxBytes)
		if err != nil {
			return fail(io, err)
		}
		if err := writeProtocol(p, cfg, bot); err != nil {
			return fail(io, err)
		}
		for _, w := range warns {
			fmt.Fprintln(io.Err, mutedStyle.Render("warning: "+w))
		}
		fmt.Fprintln(io.Out, okStyle.Render("updated soul "+bot.Slug))
		return 0
	}
	body, err := soul.Read(path, cfg.SoulMaxBytes)
	if err != nil {
		return fail(io, err)
	}
	fmt.Fprint(io.Out, body)
	if !strings.HasSuffix(body, "\n") {
		fmt.Fprintln(io.Out)
	}
	return 0
}

func cmdHide(io IO, args []string, hidden bool) int {
	if len(args) != 1 {
		verb := "hide"
		if !hidden {
			verb = "unhide"
		}
		fmt.Fprintf(io.Err, "%s\n", errStyle.Render("usage: crushbot "+verb+" <slug>"))
		return 2
	}
	p := config.ResolvePaths()
	bot, err := roster.SetHidden(p.Home, args[0], hidden)
	if err != nil {
		return fail(io, err)
	}
	state := "hidden"
	if !hidden {
		state = "visible"
	}
	fmt.Fprintln(io.Out, okStyle.Render(bot.Slug+" "+state))
	return 0
}

func cmdClone(io IO, args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(io.Err, errStyle.Render("usage: crushbot clone <src> <dst>"))
		return 2
	}
	p := config.ResolvePaths()
	cfg, err := config.Load(p)
	if err != nil {
		return fail(io, err)
	}
	bot, warns, err := roster.Clone(p.Home, args[0], args[1], cfg.MaxBots, cfg.SoulMaxBytes)
	if err != nil {
		return fail(io, err)
	}
	for _, w := range warns {
		fmt.Fprintln(io.Err, mutedStyle.Render("warning: "+w))
	}
	fmt.Fprintln(io.Out, okStyle.Render("cloned "+args[0]+" → "+bot.Slug))
	return 0
}

func cmdDelete(io IO, args []string) int {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(io.Err)
	yes := fs.Bool("yes", false, "do not prompt")
	slug, flagArgs, err := slugThenFlags(args)
	if err != nil {
		fmt.Fprintln(io.Err, errStyle.Render("usage: crushbot delete <slug> [--yes]"))
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	p := config.ResolvePaths()
	if !roster.Exists(p.Home, slug) {
		return fail(io, fmt.Errorf("unknown bot %s", slug))
	}
	if !*yes {
		if interactive() {
			ok, err := confirmForm("Delete bot @" + slug + "?")
			if err != nil || !ok {
				fmt.Fprintln(io.Err, errStyle.Render("aborted"))
				return 1
			}
		} else {
			fmt.Fprintf(io.Out, "type %s to confirm: ", slug)
			var got string
			if _, err := fmt.Fscanln(io.In, &got); err != nil || got != slug {
				fmt.Fprintln(io.Err, errStyle.Render("aborted"))
				return 1
			}
		}
	}
	if err := roster.Delete(p.Home, slug); err != nil {
		return fail(io, err)
	}
	fmt.Fprintln(io.Out, okStyle.Render("deleted "+slug))
	return 0
}

func fail(io IO, err error) int {
	fmt.Fprintln(io.Err, errStyle.Render(err.Error()))
	return 1
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// slugThenFlags supports `cmd <slug> --flag` (stdlib flag stops at the first non-flag).
func slugThenFlags(args []string) (slug string, flags []string, err error) {
	if len(args) == 0 {
		return "", nil, fmt.Errorf("missing slug")
	}
	if !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:], nil
	}
	slug = args[len(args)-1]
	if strings.HasPrefix(slug, "-") {
		return "", nil, fmt.Errorf("missing slug")
	}
	return slug, args[:len(args)-1], nil
}
