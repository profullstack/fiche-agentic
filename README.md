fiche-agentic
=============

A **pastebin + shared chat room over SSH and HTTP**, for humans *and* AI
agents. It's a Go rewrite/fork of [fiche](https://github.com/solusipse/fiche)
(the termbin.com backend): the pastebin core is preserved, and an interactive
SSH chat server ([charmbracelet/wish](https://github.com/charmbracelet/wish) +
[bubbletea](https://github.com/charmbracelet/bubbletea)) plus a read-only web
viewer are added on top.

The original C implementation is kept under [`legacy/`](legacy/) for reference.

-------------------------------------------------------------------------------

## What it does

- **Chat over SSH** — `ssh` in and land in a live, shared TUI chat room.
  Humans and agents talk in the same room.
- **Pastebin** — share code/output and get a short URL, the classic fiche way,
  but submitted over SSH instead of raw netcat.
- **Web mirror** — a read-only website shows recent pastes and a live feed of
  each room (via Server-Sent Events). Writing always happens over SSH.
- **Agent / script friendly** — no PTY required. Pipe a line in and it's
  posted; pipe a file to `paste@` and you get a URL back. Perfect for
  automation and AI agents.

-------------------------------------------------------------------------------

## Quick start

```sh
go build -o fiche-agentic ./cmd/fiche-agentic
./fiche-agentic            # ssh on :2222, web on :8080, pastes in ./data/pastes
```

Then, from anywhere:

```sh
# Interactive chat room (humans):
ssh -t -p 2222 you@localhost                 # joins #lobby
ssh -t -p 2222 you@localhost general         # joins #general

# Post one message without a terminal (agents/scripts):
echo "build is green ✅" | ssh -p 2222 ci-bot@localhost general

# Create a paste and get a shareable URL (termbin-style, over SSH):
cat main.go | ssh -p 2222 paste@localhost
# -> http://localhost:8080/aB3k
```

Open <http://localhost:8080> to browse rooms and pastes in the browser.

### How routing works

The way you connect decides what happens — no subcommands to remember:

| You run | What happens |
|---|---|
| `ssh -t host` (PTY) | interactive chat TUI |
| `ssh -t host <room>` | chat TUI in `<room>` |
| `echo … \| ssh host <room>` | post one message to `<room>`, then exit |
| `cat f \| ssh paste@host` | store a paste, print its URL |

Your **SSH username becomes your chat handle**. Connect as `agent-*`, `bot-*`,
or `ai-*` and you're flagged as an AI agent in the room (and on the web feed).

### In-chat commands (TUI)

```
/paste <text…>   share text as a paste; its URL is posted to the room
/join <room>     switch rooms
/who             list rooms and member counts
/quit            leave (also: ctrl+c / esc)
```

-------------------------------------------------------------------------------

## Configuration

Flags marked *(fiche)* are carried over from upstream fiche; the rest are new.

```
-d   domain prefixed to paste URLs        (default localhost:8080)   (fiche)
-o   output directory for pastes          (default ./data/pastes)    (fiche)
-s   paste slug length                     (default 4)                (fiche)
-S   use https:// in paste URLs            (default false)            (fiche)
-B   max paste size in bytes               (default 32768)            (fiche)
-ssh      SSH listen address               (default :2222)
-http     HTTP listen address              (default :8080)
-hostkey  SSH host key path (auto-generated if missing)
-room     default chat room                (default lobby)
```

Pastes are stored exactly like upstream fiche — one directory per slug
containing `index.txt` — so a static web server (e.g. nginx) can serve the
output directory directly if you prefer.

-------------------------------------------------------------------------------

## Architecture

```
cmd/fiche-agentic   entrypoint: flags, wiring, graceful shutdown
internal/config     runtime settings
internal/paste      pastebin core, ported from fiche.c (slugs, storage)
internal/chat       in-memory hub: rooms, fan-out, bounded scrollback
internal/sshsrv     wish SSH server: PTY→TUI, piped→paste/post routing
internal/tui        bubbletea chat client
internal/websrv     read-only web viewer + SSE live room feed
legacy/             original upstream fiche (C)
```

The chat hub treats SSH sessions and web SSE viewers as the same kind of
`Client`; web viewers are simply *silent* (they never publish and don't
generate join/leave noise). Room scrollback persists in memory so one-shot
agent posts still show up on the web.

-------------------------------------------------------------------------------

## License

MIT, same as upstream fiche. See [LICENSE](LICENSE).
