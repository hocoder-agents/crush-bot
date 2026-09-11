# Mesh

Bots talk through crushbot, not by shelling Crush.

## Delivery

`message_bot` writes `inbox/pending/<id>.json` and returns `queued` or `sent`.

- `sent` — file on disk **and** `crushbot daemon` is running
- `queued` — file on disk, daemon down. **Do not retry** either status.

The daemon moves pending → processing, runs `crush run` under the same `turn.lock` as `say`/`chat`, then archives. After a successful **DM** wake it may enqueue a FYI `kind: receipt` (last assistant text, 4096 chars) to the sender. Receipts never generate more receipts.

```bash
crushbot daemon start
crushbot mention researcher coder "ask who they are"
crushbot inbox
```

## Hop and fan-out

Hop is **not** a model argument. The host writes `$BOT_HOME/turn.json` before every Crush process. MCP increments hop, appends the current bot to `trace`, and rejects `hop > 8`, self-DMs, and more than 4 mesh sends per unattended turn (32 in `crushbot chat`).

Replies to the sender are allowed. Path-cycle (`to already in trace`) is **not** a reject.

## Tasks

```bash
# from a bot turn: assign_task
crushbot tasks
crushbot task show <id>
crushbot task retry <id>
```

New inits have `experimental.tasks: true`. Completing a task notifies the assigner with a `kind: receipt`.

## Groups

Off until you opt in:

```bash
crushbot group create review natasha diana andreea
crushbot group chat review --plain
```

Public lines are `group_say`. `message_bot` in a group round is a private DM and does not hit the transcript.

## Human routing

Keep a Crush server warm (skips spawn tax on the next `say`/wake):

```bash
crushbot spawn coder --keepalive
crushbot keepalive start coder
```

`crush run` then uses `--host unix://$BOT_HOME/crush.sock`. The daemon still holds `turn.lock` so prompts stay serial.

Install the mesh courier as a user service:

```bash
crushbot daemon install    # ~/.config/systemd/user/crushbot.service
crushbot daemon uninstall
```

Crush’s TUI has no roster `@`. Use crushbot:

| Command | Effect |
| --- | --- |
| `say` / `chat` | Talk to one bot under `turn.lock` |
| `mention <bot> <target> <text>` | Wake `<bot>` and ask it to `message_bot` the target |
| `broadcast <text>` | Hop-0 user DM to every visible bot |
| `crushbot` | Mesh TUI (`j`/`k` scroll transcript, `enter` opens chat + prompt, `esc` returns to list) |
| `crushbot chat <slug>` | Full Crush TUI under `turn.lock` |
