package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hocoder-agents/crush-bot/internal/config"
	"github.com/hocoder-agents/crush-bot/internal/group"
	"github.com/hocoder-agents/crush-bot/internal/ui"
)

func cmdGroup(io IO, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(io.Err, errStyle.Render("usage: crushbot group create|list|chat|disband"))
		return 2
	}
	p := config.ResolvePaths()
	cfg, err := config.Load(p)
	if err != nil {
		return fail(io, err)
	}
	switch args[0] {
	case "create":
		if len(args) < 4 {
			fmt.Fprintln(io.Err, errStyle.Render("usage: crushbot group create <name> <member> <member> [members...]"))
			return 2
		}
		g, err := group.Create(p.Home, args[1], args[2:])
		if err != nil {
			return fail(io, err)
		}
		fmt.Fprintln(io.Out, okStyle.Render("created group "+g.ID+" members "+strings.Join(g.Members, ",")))
		return 0
	case "list":
		gs, err := group.List(p.Home)
		if err != nil {
			return fail(io, err)
		}
		if len(gs) == 0 {
			fmt.Fprintln(io.Out, mutedStyle.Render("no groups"))
			return 0
		}
		for _, g := range gs {
			fmt.Fprintf(io.Out, "%s  %s  %s\n", g.ID, g.Name, strings.Join(g.Members, ","))
		}
		return 0
	case "disband":
		if len(args) != 2 {
			fmt.Fprintln(io.Err, errStyle.Render("usage: crushbot group disband <id>"))
			return 2
		}
		if err := group.Delete(p.Home, args[1]); err != nil {
			return fail(io, err)
		}
		fmt.Fprintln(io.Out, okStyle.Render("disbanded "+args[1]))
		return 0
	case "chat":
		plain := false
		id := ""
		for _, a := range args[1:] {
			if a == "--plain" {
				plain = true
				continue
			}
			if !strings.HasPrefix(a, "-") {
				id = a
			}
		}
		if id == "" {
			fmt.Fprintln(io.Err, errStyle.Render("usage: crushbot group chat <id> [--plain]"))
			return 2
		}
		g, err := group.Load(p.Home, id)
		if err != nil {
			return fail(io, err)
		}
		bin, err := crushBin(cfg)
		if err != nil {
			return fail(io, err)
		}
		if !plain && isTTY(os.Stdout) && isTTY(os.Stdin) {
			if err := ui.RunGroup(p.Home, bin, cfg, g); err != nil {
				return fail(io, err)
			}
			return 0
		}
		sc := bufio.NewScanner(io.In)
		if io.In == nil {
			sc = bufio.NewScanner(os.Stdin)
		}
		fmt.Fprintln(io.Out, mutedStyle.Render("group @"+g.ID+" — type a line, empty to skip"))
		for sc.Scan() {
			line := sc.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			err := group.RunUntilSettle(ctx, cfg, bin, p.Home, g, line)
			cancel()
			if err != nil {
				fmt.Fprintln(io.Err, errStyle.Render(err.Error()))
			}
			lines, _ := group.ReadTranscript(p.Home, g.ID)
			for _, l := range lines {
				fmt.Fprintf(io.Out, "[%s] %s: %s\n", l.Kind, l.From, l.Body)
			}
		}
		return 0
	default:
		fmt.Fprintln(io.Err, errStyle.Render("usage: crushbot group create|list|chat|disband"))
		return 2
	}
}
