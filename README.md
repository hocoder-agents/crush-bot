# crushbot

A [Charm](https://charm.sh) Crush powered bot mesh. Each bot is a named [Crush](https://github.com/charmbracelet/crush) workspace with a required `soul.md`. Bots DM each other, hand off tasks, and sit in group rooms. The host is a Charm app (Bubble Tea, Lip Gloss, Huh) — not cobra.

## How to install

### Prerequisites

- Linux (sandbox is Linux-only; chat-only bots can run elsewhere)
- Go 1.24+
- [Crush](https://github.com/charmbracelet/crush) **≥ 0.91.2** on `$PATH`, logged in with at least one provider (`crush login` / `crush models`)
- Optional: [`bwrap`](https://github.com/containers/bubblewrap) for `--coder` bots (Landlock is used if bwrap is missing)

### Build

```bash
git clone https://github.com/hocoder-agents/crush-bot.git
cd crush-bot
go build -o crushbot ./cmd/crushbot
```

Put `crushbot` on your `PATH`, for example:

```bash
install -m 755 crushbot ~/.local/bin/crushbot
```

### First-time setup

```bash
crushbot init
crushbot doctor
```

`init` creates `$CRUSHBOT_HOME` (default `~/.local/share/crushbot`) and `$XDG_CONFIG_HOME/crushbot/config.yaml`. Override the data dir with `CRUSHBOT_HOME`.

### Run the mesh daemon

Bots only wake on DMs/tasks while the daemon is running. `say` / `chat` work without it.

Foreground / tmux:

```bash
crushbot daemon start
```

Or install a systemd **user** unit:

```bash
crushbot daemon install      # writes ~/.config/systemd/user/crushbot.service and enable --now
crushbot daemon uninstall
```

The unit’s `PATH` is the directory that contains `crushbot`, then `/usr/local/bin:/usr/bin:/bin`. Put `crush` next to `crushbot` (typically both in `~/.local/bin`) or the service exits with `crush not found on PATH`. Re-run `crushbot daemon install` after you move the binary.

## How to use

### Roster

```bash
crushbot presets                       # list shipped default bots
crushbot crew                          # spawn every missing default bot

crushbot spawn researcher --title Researcher
crushbot spawn coder --title Coder --coder          # bash/edit, sandboxed on Linux
crushbot list
crushbot show researcher
crushbot soul researcher --edit
crushbot hide researcher
crushbot clone researcher intern
crushbot delete intern --yes
```

Three default bots ship with crushbot: `researcher`, `coder` (`--coder`), and `reviewer`. Their souls live in `internal/preset/souls/`; spawning a preset slug seeds that soul plus its title/role, and `--title`/`--description`/`--coder` flags override.

`spawn` with no flags in a TTY opens a Huh form. `soul.md` is seeded once and never overwritten.

### Talk to one bot

```bash
crushbot say researcher "Who are you?"
crushbot chat researcher          # Crush TUI under the same turn.lock
crushbot stop researcher          # SIGINT an in-flight Crush
```

### Mesh (bots talking to each other)

```bash
crushbot daemon start
crushbot mention researcher coder "ask them to review the lock"
crushbot broadcast "stand-up in 5"
crushbot inbox
crushbot inbox retry <id>
```

`mention` wakes the first bot and asks it to `message_bot` the second. Delivery is mailbox-and-wake: JSON in `inbox/pending/`, then the daemon runs Crush.

### Tasks

```bash
crushbot tasks
crushbot task show <id>
crushbot task retry <id>
crushbot task unblock <id>
```

Bots assign work with the `assign_task` MCP tool. New inits have `experimental.tasks: true`.

### Groups

```bash
crushbot group enable
crushbot group create review researcher coder
crushbot group list
crushbot group chat review              # Bubble Tea transcript + input
crushbot group chat review --plain      # scripts / pipes
crushbot group disband review
```

Public lines are `group_say`. `message_bot` in a group round is a private DM.

### Mesh TUI

```bash
crushbot                  # or: crushbot mesh
```

Roster on the left; the right pane is that bot’s Crush **session transcript** plus a prompt (`crush run`, same as `say`). `enter` opens chat, type a line and `enter` to send, `pgup`/`pgdn` scroll, `i` opens the mailbox, `ctrl+g` returns to the list, `n` spawns a bot, `q` / `ctrl+q` quits. Full Crush TUI is still `crushbot chat <slug>`. `--plain` prints a table.

### Keep-alive Crush server

Skips cold spawn on the next `say` / daemon wake (`crush run --host unix://$BOT_HOME/crush.sock`). Prompts stay serial under `turn.lock`.

```bash
crushbot spawn coder --coder --keepalive
crushbot keepalive start coder        # or: --all
crushbot keepalive status
crushbot keepalive stop coder
```

### Health

```bash
crushbot doctor
crushbot doctor --check               # exit 2 if needs_you or failed mail
```

`--coder` bots need a sandbox (`bwrap` or Landlock). Pass `--sandbox-off` only if you accept unsandboxed bash/edit.

## Docs

- [docs/soul.md](docs/soul.md) — identity files
- [docs/mesh.md](docs/mesh.md) — DMs, tasks, groups, daemon
- [docs/mesh-model.md](docs/mesh-model.md) — how Crush workspaces become a mesh
- [DESIGN.md](./DESIGN.md) — full spec

```bash
scripts/e2e-mesh.sh                   # fake Crush, no provider
```
