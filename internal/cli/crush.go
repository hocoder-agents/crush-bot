package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hocoder-agents/crush-bot/internal/config"
	"github.com/hocoder-agents/crush-bot/internal/crush"
	"github.com/hocoder-agents/crush-bot/internal/daemon"
	"github.com/hocoder-agents/crush-bot/internal/protocol"
	"github.com/hocoder-agents/crush-bot/internal/roster"
	"github.com/hocoder-agents/crush-bot/internal/sandbox"
	"github.com/hocoder-agents/crush-bot/internal/soul"
)

func writeProtocol(p config.Paths, cfg config.Config, bot roster.Bot) error {
	all, err := roster.List(p.Home, true)
	if err != nil {
		return err
	}
	exe, _ := os.Executable()
	return protocol.Write(protocol.Options{
		Root:         p.Home,
		Bot:          bot,
		Teammates:    all,
		Tasks:        cfg.Experimental.Tasks,
		Groups:       true,
		IncludeMCP:   true,
		CrushbotPath: exe,
		SoulMax:      cfg.SoulMaxBytes,
	})
}

func crushBin(cfg config.Config) (string, error) {
	return crush.LookPath(cfg.CrushPath)
}

func cmdSay(io IO, args []string) int {
	fsNowait := false
	var pos []string
	for _, a := range args {
		switch a {
		case "--nowait":
			fsNowait = true
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) < 1 {
		fmt.Fprintln(io.Err, errStyle.Render("usage: crushbot say <slug> [prompt|-] [--nowait]"))
		return 2
	}
	slug := pos[0]
	prompt := strings.Join(pos[1:], " ")
	p := config.ResolvePaths()
	cfg, err := config.Load(p)
	if err != nil {
		return fail(io, err)
	}
	bot, err := roster.Load(p.Home, slug)
	if err != nil {
		return fail(io, err)
	}
	if prompt == "-" || prompt == "" {
		b, err := readAll(io.In)
		if err != nil {
			return fail(io, err)
		}
		prompt = strings.TrimSpace(string(b))
	}
	if prompt == "" {
		fmt.Fprintln(io.Err, errStyle.Render("empty prompt"))
		return 2
	}
	bin, err := crushBin(cfg)
	if err != nil {
		return fail(io, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.TurnLockTimeout+2*time.Minute)
	defer cancel()
	res, err := crush.Run(ctx, crush.RunOpts{
		Bot:     bot,
		Root:    p.Home,
		Bin:     bin,
		Kind:    "human_say",
		Prompt:  prompt,
		Nowait:  fsNowait,
		Timeout: cfg.TurnLockTimeout,
		Debug:   os.Getenv("CRUSHBOT_DEBUG") == "1",
		Yolo:    bot.Unattended == "yolo",
		Stdout:  io.Out,
		Stderr:  io.Err,
	})
	if err != nil {
		return fail(io, err)
	}
	if strings.TrimSpace(res.Stdout) == "" {
		fmt.Fprintln(io.Out, mutedStyle.Render("(no output)"))
	}
	return 0
}

func cmdChat(io IO, args []string) int {
	nowait := false
	var slug string
	for _, a := range args {
		if a == "--nowait" {
			nowait = true
			continue
		}
		if slug == "" && !strings.HasPrefix(a, "-") {
			slug = a
		}
	}
	if slug == "" {
		fmt.Fprintln(io.Err, errStyle.Render("usage: crushbot chat <slug> [--nowait]"))
		return 2
	}
	p := config.ResolvePaths()
	cfg, err := config.Load(p)
	if err != nil {
		return fail(io, err)
	}
	bot, err := roster.Load(p.Home, slug)
	if err != nil {
		return fail(io, err)
	}
	if bot.CanonicalSessionID == "" {
		return fail(io, fmt.Errorf("no canonical session; crushbot doctor %s", slug))
	}
	bin, err := crushBin(cfg)
	if err != nil {
		return fail(io, err)
	}
	err = crush.Chat(context.Background(), crush.RunOpts{
		Bot:     bot,
		Root:    p.Home,
		Bin:     bin,
		Nowait:  nowait,
		Timeout: cfg.TurnLockTimeout,
		Debug:   os.Getenv("CRUSHBOT_DEBUG") == "1",
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	})
	if err != nil {
		return fail(io, err)
	}
	return 0
}

func cmdStop(io IO, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(io.Err, errStyle.Render("usage: crushbot stop <slug>"))
		return 2
	}
	p := config.ResolvePaths()
	if err := crush.Stop(roster.Home(p.Home, args[0])); err != nil {
		return fail(io, err)
	}
	fmt.Fprintln(io.Out, okStyle.Render("stopped "+args[0]))
	return 0
}

func cmdDoctor(io IO, args []string) int {
	checkOnly := false
	var pos []string
	for _, a := range args {
		if a == "--check" {
			checkOnly = true
			continue
		}
		pos = append(pos, a)
	}
	args = pos
	p := config.ResolvePaths()
	cfg, err := config.Load(p)
	if err != nil {
		return fail(io, err)
	}
	var slugs []string
	if len(args) == 1 {
		slugs = []string{args[0]}
	} else {
		bots, err := roster.List(p.Home, true)
		if err != nil {
			return fail(io, err)
		}
		for _, b := range bots {
			slugs = append(slugs, b.Slug)
		}
	}
	bin, binErr := crushBin(cfg)
	ok := true
	check := func(name string, err error) {
		if err != nil {
			ok = false
			fmt.Fprintln(io.Out, errStyle.Render("FAIL  "+name+": "+err.Error()))
			return
		}
		fmt.Fprintln(io.Out, okStyle.Render("ok    "+name))
	}
	if binErr != nil {
		check("crush binary", binErr)
	} else {
		check("crush >= "+cfg.MinCrushVersion, crush.RequireMin(bin, cfg.MinCrushVersion))
		check("providers", crush.HasProviders(bin))
	}
	if daemon.Live(p.Home) {
		check("daemon", nil)
	} else {
		fmt.Fprintln(io.Out, mutedStyle.Render("warn  daemon not running (mesh DMs stay queued)"))
	}
	if !cfg.Experimental.Tasks {
		fmt.Fprintln(io.Out, mutedStyle.Render("warn  experimental.tasks is false; task MCP tools are disabled"))
	}
	if len(slugs) == 0 {
		fmt.Fprintln(io.Out, mutedStyle.Render("no bots"))
		if !ok {
			return 1
		}
		return 0
	}
	for _, slug := range slugs {
		fmt.Fprintln(io.Out, headStyle.Render("bot "+slug))
		bot, err := roster.Load(p.Home, slug)
		if err != nil {
			check("load", err)
			continue
		}
		home := roster.Home(p.Home, slug)
		body, err := soul.Read(roster.SoulPath(p.Home, slug), cfg.SoulMaxBytes)
		if err != nil {
			check("soul.md", err)
		} else if soul.IsBlank(body) {
			fmt.Fprintln(io.Out, mutedStyle.Render("warn  soul.md is whitespace-only"))
		} else {
			check("soul.md", nil)
		}
		for _, rel := range []string{
			"crushrc", "crushrc.d/10-host.crushrc", "CRUSH.md", "protocol.md",
			"hooks/identity.sh", "hooks/deny-disabled-tools.sh", ".mcp_token",
		} {
			_, err := os.Stat(filepath.Join(home, rel))
			check(rel, err)
		}
		if bot.CanonicalSessionID == "" {
			fmt.Fprintln(io.Out, mutedStyle.Render("warn  no canonical session uuid"))
		} else {
			check("session "+bot.CanonicalSessionID, nil)
		}
		if bot.KeepAlive {
			if crush.ServerLive(home) {
				check("keepalive "+crush.HostURL(home), nil)
			} else {
				fmt.Fprintln(io.Out, mutedStyle.Render("warn  keepalive down; crushbot keepalive start "+slug))
			}
		}
		if sandbox.Required(bot) {
			check("sandbox", sandbox.Available())
		} else if bot.Sandbox == "off" && (bot.Tools.Bash || bot.Tools.Edit) {
			fmt.Fprintln(io.Out, mutedStyle.Render("warn  sandbox:off"))
		}
		if t, err := crush.ReadTurn(home); err == nil {
			if crush.PIDAlive(t.CrushPID) {
				fmt.Fprintln(io.Out, mutedStyle.Render(fmt.Sprintf("busy  crush pid %d kind %s", t.CrushPID, t.Kind)))
			} else {
				fmt.Fprintln(io.Out, mutedStyle.Render("warn  stale turn.json"))
			}
		}
		proc := filepath.Join(home, "inbox", "processing")
		if entries, err := os.ReadDir(proc); err == nil {
			for _, e := range entries {
				info, err := e.Info()
				if err != nil {
					continue
				}
				if time.Since(info.ModTime()) > cfg.TurnLockTimeout+60*time.Second {
					fmt.Fprintln(io.Out, mutedStyle.Render("warn  stale processing/"+e.Name()))
				}
			}
		}
	}
	needs, _ := os.ReadFile(filepath.Join(p.Home, "needs_you.jsonl"))
	failedN := 0
	bots, _ := roster.List(p.Home, true)
	for _, b := range bots {
		entries, _ := os.ReadDir(filepath.Join(roster.Home(p.Home, b.Slug), "inbox", "failed"))
		failedN += len(entries)
	}
	if checkOnly && (len(needs) > 0 || failedN > 0) {
		return 2
	}
	if !ok {
		return 1
	}
	return 0
}

func readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}
