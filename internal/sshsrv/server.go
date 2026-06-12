// Package sshsrv exposes fiche-agentic over SSH.
//
// Routing is by how you connect, which keeps it friendly to both humans
// and agents/scripts:
//
//   - PTY session (normal `ssh host`)            -> interactive chat TUI
//   - piped to user "paste" (cat f | ssh paste@) -> create a paste, print URL
//   - piped to any other user (echo | ssh host)  -> post one line to a room
//
// The SSH username becomes the chat handle. Connect as `agent-*` (or set
// the user to anything starting with "agent") to be flagged as an AI agent
// in the room. The room is taken from the ssh command, e.g. `ssh host general`.
package sshsrv

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/logging"
	"github.com/charmbracelet/wish/recover"
	gossh "golang.org/x/crypto/ssh"

	"github.com/profullstack/fiche-agentic/internal/chat"
	"github.com/profullstack/fiche-agentic/internal/config"
	"github.com/profullstack/fiche-agentic/internal/paste"
	"github.com/profullstack/fiche-agentic/internal/tui"
)

// New builds the wish SSH server.
func New(cfg config.Config, hub *chat.Hub, store *paste.Store) (*ssh.Server, error) {
	return wish.NewServer(
		wish.WithAddress(cfg.SSHAddr),
		wish.WithHostKeyPath(cfg.HostKeyPath),
		// Open server: anyone can connect. Public keys are accepted so
		// agents can use key auth; keyboard-interactive lets humans in
		// without a configured key.
		wish.WithPublicKeyAuth(func(ssh.Context, ssh.PublicKey) bool { return true }),
		wish.WithKeyboardInteractiveAuth(func(ssh.Context, gossh.KeyboardInteractiveChallenge) bool { return true }),
		wish.WithMiddleware(
			handler(cfg, hub, store),
			logging.Middleware(),
			recover.Middleware(),
		),
	)
}

func handler(cfg config.Config, hub *chat.Hub, store *paste.Store) wish.Middleware {
	return func(next ssh.Handler) ssh.Handler {
		return func(s ssh.Session) {
			defer next(s)

			user := sanitizeName(s.User())
			room := roomFromCommand(s.Command(), cfg.DefaultRoom)
			_, _, hasPty := s.Pty()

			switch {
			case user == "paste" || firstArg(s.Command()) == "paste":
				handlePaste(s, store)
			case !hasPty:
				handlePipeChat(s, hub, user, room)
			default:
				handleTUI(s, hub, store, cfg, user, room)
			}
		}
	}
}

// handlePaste reads stdin and stores it as a paste (SSH-native termbin).
func handlePaste(s ssh.Session, store *paste.Store) {
	data, err := io.ReadAll(io.LimitReader(s, int64(store.BufferLen)+1))
	if err != nil {
		fmt.Fprintln(s, "error reading input:", err)
		return
	}
	slug, err := store.Create(data)
	if err != nil {
		fmt.Fprintln(s, "error:", err)
		return
	}
	fmt.Fprintln(s, store.URL(slug))
}

// handlePipeChat posts piped stdin (one message per line, or the whole
// blob as one message) into a room, then exits. Ideal for agents/scripts.
func handlePipeChat(s ssh.Session, hub *chat.Hub, user, room string) {
	client := &chat.Client{Name: user, Agent: isAgent(user), Out: make(chan chat.Message, 1)}
	hub.Join(client, room)
	defer hub.Leave(client)

	data, _ := io.ReadAll(io.LimitReader(s, 64*1024))
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		hub.Publish(chat.Message{
			Room:  room,
			From:  user,
			Text:  line,
			Kind:  chat.KindChat,
			Agent: client.Agent,
			Time:  time.Now(),
		})
	}
	fmt.Fprintf(s, "posted to #%s as %s\n", room, user)
}

// handleTUI runs the interactive bubbletea chat client over the PTY.
func handleTUI(s ssh.Session, hub *chat.Hub, store *paste.Store, cfg config.Config, user, room string) {
	client := &chat.Client{Name: user, Agent: isAgent(user), Out: make(chan chat.Message, 64)}
	hub.Join(client, room)
	defer hub.Leave(client)

	pasteFn := func(content []byte) (string, error) {
		slug, err := store.Create(content)
		if err != nil {
			return "", err
		}
		return store.URL(slug), nil
	}

	model := tui.New(hub, client, room, pasteFn)
	p := tea.NewProgram(model, tea.WithInput(s), tea.WithOutput(s), tea.WithAltScreen())

	// Pump hub -> program.
	ctx, cancel := context.WithCancel(s.Context())
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-client.Out:
				if !ok {
					return
				}
				p.Send(tui.IncomingMsg(msg))
			}
		}
	}()

	// Pump window resizes -> program. The initial size must be sent from a
	// goroutine: p.Send blocks until the program loop is running, and the
	// loop only starts once p.Run() is called below.
	pty, winCh, _ := s.Pty()
	go func() {
		p.Send(tea.WindowSizeMsg{Width: pty.Window.Width, Height: pty.Window.Height})
		for {
			select {
			case <-ctx.Done():
				return
			case w, ok := <-winCh:
				if !ok {
					return
				}
				p.Send(tea.WindowSizeMsg{Width: w.Width, Height: w.Height})
			}
		}
	}()

	if _, err := p.Run(); err != nil {
		wish.Errorln(s, err)
	}
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "anon"
	}
	if len(name) > 32 {
		name = name[:32]
	}
	return name
}

func isAgent(user string) bool {
	u := strings.ToLower(user)
	return strings.HasPrefix(u, "agent") || strings.HasPrefix(u, "bot") || strings.HasPrefix(u, "ai-")
}

// roomFromCommand picks a room name from the ssh command args, ignoring a
// leading "paste" keyword. Falls back to the default room.
func roomFromCommand(cmd []string, def string) string {
	for _, a := range cmd {
		a = sanitizeName(a)
		if a == "paste" || a == "" {
			continue
		}
		return a
	}
	return def
}

func firstArg(cmd []string) string {
	if len(cmd) == 0 {
		return ""
	}
	return strings.TrimSpace(cmd[0])
}
