// Package tui is the interactive chat client rendered to an SSH PTY via
// bubbletea. It shows a scrolling message pane and an input line. Pastes
// are created with the /paste command and shared back into the room.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/profullstack/fiche-agentic/internal/chat"
)

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213")).Padding(0, 1)
	sysStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	agentStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)
	nameStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	pasteStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("156"))
	timeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

// IncomingMsg wraps a hub message delivered into the bubbletea event loop.
type IncomingMsg chat.Message

// PasteFunc stores the given content and returns its public URL.
type PasteFunc func(content []byte) (url string, err error)

// Model is the chat UI state for one SSH session.
type Model struct {
	hub    *chat.Hub
	client *chat.Client
	paste  PasteFunc

	room  string
	vp    viewport.Model
	input textinput.Model
	lines []string

	width  int
	height int
	ready  bool
}

// New builds a chat Model for a session already joined to roomName.
func New(hub *chat.Hub, client *chat.Client, room string, paste PasteFunc) Model {
	in := textinput.New()
	in.Placeholder = "message — /help for commands"
	in.Prompt = "› "
	in.CharLimit = 4000
	in.Focus()

	return Model{
		hub:    hub,
		client: client,
		paste:  paste,
		room:   room,
		input:  in,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return textinput.Blink }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		inputHeight := 3
		vpHeight := msg.Height - inputHeight - 1
		if vpHeight < 1 {
			vpHeight = 1
		}
		if !m.ready {
			m.vp = viewport.New(msg.Width, vpHeight)
			m.ready = true
		} else {
			m.vp.Width = msg.Width
			m.vp.Height = vpHeight
		}
		m.input.Width = msg.Width - 4
		m.refresh()

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "enter":
			cmd := m.submit()
			if cmd != nil {
				return m, cmd
			}
		}

	case IncomingMsg:
		m.append(chat.Message(msg))
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)
	m.vp, cmd = m.vp.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

// submit handles the current input line: a slash command or a chat message.
func (m *Model) submit() tea.Cmd {
	text := strings.TrimSpace(m.input.Value())
	m.input.Reset()
	if text == "" {
		return nil
	}

	if strings.HasPrefix(text, "/") {
		return m.command(text)
	}

	m.hub.Publish(chat.Message{
		Room:  m.room,
		From:  m.client.Name,
		Text:  text,
		Kind:  chat.KindChat,
		Agent: m.client.Agent,
		Time:  time.Now(),
	})
	return nil
}

func (m *Model) command(text string) tea.Cmd {
	fields := strings.Fields(text)
	switch fields[0] {
	case "/help":
		m.system("commands: /paste <text…> · /join <room> · /who · /quit")
	case "/quit":
		return tea.Quit
	case "/who":
		var names []string
		for _, r := range m.hub.Rooms() {
			if r.Name == m.room {
				m.system(fmt.Sprintf("room %q has %d member(s)", m.room, r.Members))
			}
			names = append(names, fmt.Sprintf("%s(%d)", r.Name, r.Members))
		}
		m.system("rooms: " + strings.Join(names, " "))
	case "/join":
		if len(fields) < 2 {
			m.system("usage: /join <room>")
			return nil
		}
		m.hub.Leave(m.client)
		m.room = fields[1]
		m.lines = nil
		m.hub.Join(m.client, m.room)
	case "/paste":
		body := strings.TrimSpace(strings.TrimPrefix(text, "/paste"))
		if body == "" {
			m.system("usage: /paste <text to share>")
			return nil
		}
		url, err := m.paste([]byte(body + "\n"))
		if err != nil {
			m.system("paste failed: " + err.Error())
			return nil
		}
		m.hub.Publish(chat.Message{
			Room:  m.room,
			From:  m.client.Name,
			Text:  url,
			Kind:  chat.KindPaste,
			Agent: m.client.Agent,
			Time:  time.Now(),
		})
	default:
		m.system("unknown command: " + fields[0])
	}
	return nil
}

// system shows a local-only notice (not broadcast to the room).
func (m *Model) system(text string) {
	m.append(chat.Message{From: "server", Text: text, Kind: chat.KindSystem, Time: time.Now()})
}

func (m *Model) append(msg chat.Message) {
	m.lines = append(m.lines, renderLine(msg))
	m.refresh()
}

func (m *Model) refresh() {
	if !m.ready {
		return
	}
	m.vp.SetContent(strings.Join(m.lines, "\n"))
	m.vp.GotoBottom()
}

func renderLine(msg chat.Message) string {
	ts := timeStyle.Render(msg.Time.Format("15:04"))
	switch msg.Kind {
	case chat.KindSystem:
		return fmt.Sprintf("%s %s", ts, sysStyle.Render("• "+msg.Text))
	case chat.KindPaste:
		who := nameStyle.Render(msg.From)
		return fmt.Sprintf("%s %s shared %s", ts, who, pasteStyle.Render(msg.Text))
	default:
		who := nameStyle.Render(msg.From)
		if msg.Agent {
			who = agentStyle.Render(msg.From + " ⚙")
		}
		return fmt.Sprintf("%s %s: %s", ts, who, msg.Text)
	}
}

// View implements tea.Model.
func (m Model) View() string {
	if !m.ready {
		return "starting…"
	}
	header := headerStyle.Render(fmt.Sprintf(" fiche-agentic · #%s · %s ", m.room, m.client.Name))
	help := helpStyle.Render("enter send · /help · ctrl+c quit")
	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		m.vp.View(),
		m.input.View(),
		help,
	)
}
