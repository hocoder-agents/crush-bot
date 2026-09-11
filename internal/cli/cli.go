package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/hocoder-agents/crush-bot/internal/config"
	"github.com/hocoder-agents/crush-bot/internal/ui"
	"github.com/hocoder-agents/crush-bot/internal/version"
)

const Version = version.Version

// IO is stdin/stdout/stderr for tests.
type IO struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

func stdio() IO {
	return IO{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}
}

// Main is the argv router. No cobra.
func Main(args []string) int {
	return run(stdio(), args)
}

func run(io IO, args []string) int {
	if len(args) == 0 {
		return cmdDefault(io)
	}
	verb := args[0]
	rest := args[1:]
	switch verb {
	case "help", "-h", "--help":
		printHelp(io.Out)
		return 0
	case "version", "-v", "--version":
		fmt.Fprintln(io.Out, "crushbot", Version)
		return 0
	case "init":
		return cmdInit(io, rest)
	case "mesh":
		return cmdMesh(io, rest)
	case "spawn":
		return cmdSpawn(io, rest)
	case "presets":
		return cmdPresets(io, rest)
	case "crew":
		return cmdCrew(io, rest)
	case "list":
		return cmdList(io, rest)
	case "show":
		return cmdShow(io, rest)
	case "soul":
		return cmdSoul(io, rest)
	case "hide":
		return cmdHide(io, rest, true)
	case "unhide":
		return cmdHide(io, rest, false)
	case "clone":
		return cmdClone(io, rest)
	case "delete":
		return cmdDelete(io, rest)
	case "say":
		return cmdSay(io, rest)
	case "chat":
		return cmdChat(io, rest)
	case "stop":
		return cmdStop(io, rest)
	case "doctor":
		return cmdDoctor(io, rest)
	case "mcp":
		return cmdMCP(io, rest)
	case "daemon":
		return cmdDaemon(io, rest)
	case "inbox":
		return cmdInbox(io, rest)
	case "tasks":
		return cmdTasks(io, rest)
	case "task":
		return cmdTask(io, rest)
	case "mention":
		return cmdMention(io, rest)
	case "broadcast":
		return cmdBroadcast(io, rest)
	case "group":
		return cmdGroup(io, rest)
	case "keepalive":
		return cmdKeepalive(io, rest)
	case "sandbox-exec":
		return cmdSandboxExec(io, rest)
	default:
		if strings.HasPrefix(verb, "-") {
			fmt.Fprintln(io.Err, errStyle.Render("unknown flag: "+verb))
			fmt.Fprintln(io.Err, mutedStyle.Render("try crushbot --help"))
			return 2
		}
		fmt.Fprintln(io.Err, errStyle.Render("unknown command: "+verb))
		fmt.Fprintln(io.Err, mutedStyle.Render("try crushbot --help"))
		return 2
	}
}

func cmdDefault(io IO) int {
	if !isTTY(os.Stdout) {
		printHelp(io.Out)
		return 0
	}
	p := config.ResolvePaths()
	if err := ui.Run(p.Home); err != nil {
		fmt.Fprintln(io.Err, errStyle.Render(err.Error()))
		return 1
	}
	return 0
}

func cmdMesh(io IO, args []string) int {
	plain := false
	for _, a := range args {
		switch a {
		case "--plain", "-p":
			plain = true
		case "-h", "--help":
			fmt.Fprintln(io.Out, "Usage: crushbot mesh [--plain]")
			return 0
		default:
			fmt.Fprintln(io.Err, errStyle.Render("unknown flag: "+a))
			return 2
		}
	}
	if plain || !isTTY(os.Stdout) {
		return cmdList(io, nil)
	}
	return cmdDefault(io)
}

func cmdInit(io IO, _ []string) int {
	p := config.ResolvePaths()
	if err := config.EnsureHome(p); err != nil {
		fmt.Fprintln(io.Err, errStyle.Render(err.Error()))
		return 1
	}
	cfg, err := config.Load(p)
	if err != nil {
		fmt.Fprintln(io.Err, errStyle.Render(err.Error()))
		return 1
	}
	if err := config.Save(p, cfg); err != nil {
		fmt.Fprintln(io.Err, errStyle.Render(err.Error()))
		return 1
	}
	fmt.Fprintln(io.Out, okStyle.Render("initialized"))
	fmt.Fprintln(io.Out, "  home   ", p.Home)
	fmt.Fprintln(io.Out, "  config ", p.ConfigFile)
	fmt.Fprintln(io.Out, mutedStyle.Render("default crew: crushbot crew (crushbot presets to list)"))
	return 0
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, headStyle.Render("crushbot")+"  "+mutedStyle.Render(Version))
	fmt.Fprintln(w, mutedStyle.Render("Charm Crush Powered Bot Mesh"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  crushbot                 "+mutedStyle.Render("open the mesh TUI"))
	fmt.Fprintln(w, "  crushbot <command> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	row := func(name, desc string) {
		fmt.Fprintf(w, "  %s  %s\n", cmdStyle.Render(fmt.Sprintf("%-10s", name)), desc)
	}
	row("init", "create CRUSHBOT_HOME and config")
	row("spawn", "create a bot (required soul.md)")
	row("presets", "list shipped default bots")
	row("crew", "spawn every missing default bot")
	row("list", "roster table; --json --all")
	row("show", "inspect one bot")
	row("soul", "print or --edit soul.md")
	row("hide", "hide a bot from default list")
	row("unhide", "unhide a bot")
	row("clone", "copy a bot to a new slug")
	row("delete", "remove a bot home (--yes to skip confirm)")
	row("say", "one-shot crush run under turn.lock")
	row("chat", "attach Crush TUI under turn.lock")
	row("stop", "SIGINT in-flight Crush")
	row("doctor", "check crush, soul, session, hooks; --check")
	row("daemon", "start|stop|status|logs|install|uninstall")
	row("keepalive", "start|stop|status [slug|--all] crush server")
	row("inbox", "pending/archive/failed; retry <id>")
	row("tasks", "list tasks for a bot")
	row("task", "show|retry|unblock <id>")
	row("mention", "ask <bot> to message @target")
	row("broadcast", "queue a user DM to every visible bot")
	row("group", "create|list|chat|disband")
	row("mesh", "mesh TUI (sidebar + session transcript); --plain for a table")
	row("help", "show this help")
	row("version", "print version")
	fmt.Fprintln(w)
	fmt.Fprintln(w, mutedStyle.Render("Crush ≥ 0.91.2 · daemon install · keepalive"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Env:")
	fmt.Fprintln(w, "  CRUSHBOT_HOME     "+mutedStyle.Render("data dir (default: $XDG_DATA_HOME/crushbot)"))
	fmt.Fprintln(w, "  XDG_CONFIG_HOME   "+mutedStyle.Render("config parent (…/crushbot/config.yaml)"))
}

func isTTY(f *os.File) bool {
	if f == nil {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
