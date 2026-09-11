package spawn

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/hocoder-agents/crush-bot/internal/config"
	"github.com/hocoder-agents/crush-bot/internal/crush"
	"github.com/hocoder-agents/crush-bot/internal/preset"
	"github.com/hocoder-agents/crush-bot/internal/protocol"
	"github.com/hocoder-agents/crush-bot/internal/roster"
	"github.com/hocoder-agents/crush-bot/internal/sandbox"
)

type Opts struct {
	Slug        string
	Title       string
	Description string
	Model       string
	Project     string
	CloneFrom   string
	Coder       bool
	Sandbox     string
	KeepAlive   bool
	// Soul seeds soul.md when set; empty means the generic seed.
	Soul string
}

type Result struct {
	Bot     roster.Bot
	Warns   []string
	BootErr error
}

func Create(root string, cfg config.Config, o Opts) (Result, error) {
	var out Result
	o.Slug = NormalizeSlug(o.Slug)
	bin, err := crush.LookPath(cfg.CrushPath)
	if err != nil {
		return out, err
	}
	if err := crush.RequireMin(bin, cfg.MinCrushVersion); err != nil {
		return out, err
	}
	if err := crush.HasProviders(bin); err != nil {
		return out, err
	}
	if o.Coder && o.Sandbox != "off" {
		if err := sandbox.Available(); err != nil {
			return out, fmt.Errorf("%w (or pass --sandbox-off)", err)
		}
	}
	if o.CloneFrom == "" {
		if entry, soulBody, ok := preset.Get(o.Slug); ok {
			if o.Soul == "" {
				o.Soul = soulBody
			}
			if o.Title == "" {
				o.Title = entry.Title
			}
			if o.Description == "" {
				o.Description = entry.Description
			}
			if !o.Coder {
				o.Coder = entry.Coder
			}
		}
	}
	bot, warns, err := roster.Spawn(root, roster.SpawnOpts{
		Slug:        o.Slug,
		Title:       o.Title,
		Description: o.Description,
		Model:       o.Model,
		Project:     o.Project,
		CloneFrom:   o.CloneFrom,
		Coder:       o.Coder,
		Sandbox:     o.Sandbox,
		KeepAlive:   o.KeepAlive,
		MaxBots:     cfg.MaxBots,
		SoulMax:     cfg.SoulMaxBytes,
		SoulBody:    o.Soul,
	})
	if err != nil {
		return out, err
	}
	out.Bot = bot
	out.Warns = warns
	exe, _ := os.Executable()
	all, _ := roster.List(root, true)
	if err := protocol.Write(protocol.Options{
		Root:         root,
		Bot:          bot,
		Teammates:    all,
		Tasks:        cfg.Experimental.Tasks,
		Groups:       cfg.Experimental.Groups,
		IncludeMCP:   true,
		CrushbotPath: exe,
		SoulMax:      cfg.SoulMaxBytes,
	}); err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.TurnLockTimeout+3*time.Minute)
	defer cancel()
	bot, err = crush.Bootstrap(ctx, crush.RunOpts{
		Bot:     bot,
		Root:    root,
		Bin:     bin,
		Timeout: cfg.TurnLockTimeout,
		Debug:   os.Getenv("CRUSHBOT_DEBUG") == "1",
		Yolo:    bot.Unattended == "yolo",
	})
	if err != nil {
		out.Bot = bot
		out.BootErr = err
		return out, nil
	}
	if bot.KeepAlive {
		if err := crush.StartServer(bin, bot, root); err != nil {
			out.Warns = append(out.Warns, "keepalive: "+err.Error())
		}
	}
	out.Bot = bot
	return out, nil
}

func FromForm(root string, cfg config.Config) (Result, error) {
	return FromFormIO(root, cfg, nil, nil)
}

func FromFormAccessible(root string, cfg config.Config, in io.Reader, out io.Writer) (Result, error) {
	return fromForm(root, cfg, in, out, true)
}

func FromFormIO(root string, cfg config.Config, in io.Reader, out io.Writer) (Result, error) {
	return fromForm(root, cfg, in, out, false)
}

func fromForm(root string, cfg config.Config, in io.Reader, out io.Writer, accessible bool) (Result, error) {
	var slug, title, desc string
	var coder bool
	var err error
	if accessible {
		err = FormAccessible(in, out, &slug, &title, &desc, &coder)
	} else {
		err = FormIO(in, out, &slug, &title, &desc, &coder)
	}
	if err != nil {
		return Result{}, err
	}
	if slug == "" {
		return Result{}, fmt.Errorf("cancelled")
	}
	if out != nil {
		fmt.Fprintf(out, "Creating @%s…\n", slug)
	}
	return Create(root, cfg, Opts{
		Slug:        slug,
		Title:       title,
		Description: desc,
		Coder:       coder,
	})
}
