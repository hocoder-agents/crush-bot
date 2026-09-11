package cli

import (
	"fmt"

	"github.com/hocoder-agents/crush-bot/internal/config"
	"github.com/hocoder-agents/crush-bot/internal/preset"
	"github.com/hocoder-agents/crush-bot/internal/roster"
	"github.com/hocoder-agents/crush-bot/internal/spawn"
)

func cmdPresets(io IO, _ []string) int {
	entries, err := preset.List()
	if err != nil {
		return fail(io, err)
	}
	for _, e := range entries {
		desc := e.Description
		if e.Coder {
			desc += mutedStyle.Render("  (--coder)")
		}
		fmt.Fprintf(io.Out, "  %s  %s\n", cmdStyle.Render(fmt.Sprintf("%-12s", e.Slug)), desc)
	}
	return 0
}

func cmdCrew(io IO, _ []string) int {
	p := config.ResolvePaths()
	if err := config.EnsureHome(p); err != nil {
		return fail(io, err)
	}
	cfg, err := config.Load(p)
	if err != nil {
		return fail(io, err)
	}
	existing := map[string]bool{}
	if bots, err := roster.List(p.Home, true); err == nil {
		for _, b := range bots {
			existing[b.Slug] = true
		}
	}
	entries, err := preset.List()
	if err != nil {
		return fail(io, err)
	}
	missing := 0
	for _, e := range entries {
		if existing[e.Slug] {
			fmt.Fprintln(io.Out, mutedStyle.Render("already have @"+e.Slug))
			continue
		}
		missing++
		_, soulBody, ok := preset.Get(e.Slug)
		if !ok {
			fmt.Fprintln(io.Err, errStyle.Render("failed @"+e.Slug+": no soul file"))
			continue
		}
		res, err := spawn.Create(p.Home, cfg, spawn.Opts{Slug: e.Slug, Soul: soulBody})
		if err != nil {
			fmt.Fprintln(io.Err, errStyle.Render("failed @"+e.Slug+": "+err.Error()))
			continue
		}
		for _, w := range res.Warns {
			fmt.Fprintln(io.Err, mutedStyle.Render("warning: "+w))
		}
		if res.BootErr != nil {
			fmt.Fprintln(io.Err, mutedStyle.Render("warning: crush bootstrap: "+res.BootErr.Error()))
			fmt.Fprintln(io.Err, mutedStyle.Render("bot files exist; crushbot doctor "+res.Bot.Slug))
		}
		fmt.Fprintln(io.Out, okStyle.Render("spawned "+res.Bot.Slug))
	}
	if missing == 0 {
		fmt.Fprintln(io.Out, mutedStyle.Render("crew complete"))
	}
	return 0
}
