// Package chat is the in-memory chat hub: named rooms, members, message
// fan-out, and a bounded scrollback. Both SSH sessions and web SSE viewers
// participate as Clients — the only difference is that viewers never publish
// and are joined Silently so they don't generate join/leave noise.
package chat

import (
	"sort"
	"sync"
	"time"
)

// Kind distinguishes message rendering/semantics.
type Kind string

const (
	KindChat   Kind = "chat"   // a user/agent message
	KindSystem Kind = "system" // join/leave/server notices
	KindPaste  Kind = "paste"  // a paste was shared to the room
)

// Message is a single line in a room.
type Message struct {
	Room  string    `json:"room"`
	From  string    `json:"from"`
	Text  string    `json:"text"`
	Kind  Kind      `json:"kind"`
	Agent bool      `json:"agent"` // sender self-identified as an AI agent
	Time  time.Time `json:"time"`
}

// historyLimit is how many recent messages a room retains for replay.
const historyLimit = 200

// Client is a participant in exactly one room at a time. Out is a buffered
// channel the hub sends to; sends are non-blocking, so a slow/stuck client
// drops messages rather than stalling the room.
type Client struct {
	Name   string
	Agent  bool
	Silent bool // suppress join/leave announcements (web viewers)
	Out    chan Message

	room string
}

// Room is the current name of the room the client is in.
func (c *Client) Room() string { return c.room }

// Hub owns all rooms. It is safe for concurrent use.
type Hub struct {
	mu    sync.RWMutex
	rooms map[string]*room
}

type room struct {
	name    string
	clients map[*Client]struct{}
	history []Message
}

// New returns an empty hub.
func New() *Hub {
	return &Hub{rooms: map[string]*room{}}
}

func (h *Hub) getOrCreate(name string) *room {
	r := h.rooms[name]
	if r == nil {
		r = &room{name: name, clients: map[*Client]struct{}{}}
		h.rooms[name] = r
	}
	return r
}

// Join adds c to the named room, replays scrollback to c.Out, and (unless
// Silent) announces the arrival to the room.
func (h *Hub) Join(c *Client, roomName string) {
	h.mu.Lock()
	r := h.getOrCreate(roomName)
	r.clients[c] = struct{}{}
	c.room = roomName
	history := append([]Message(nil), r.history...)
	h.mu.Unlock()

	for _, m := range history {
		trySend(c, m)
	}
	if !c.Silent {
		h.Publish(Message{
			Room: roomName,
			From: "server",
			Text: c.Name + " joined",
			Kind: KindSystem,
			Time: time.Now(),
		})
	}
}

// Leave removes c from its room and (unless Silent) announces the departure.
func (h *Hub) Leave(c *Client) {
	h.mu.Lock()
	r := h.rooms[c.room]
	if r != nil {
		delete(r.clients, c)
		// Keep the room (and its scrollback) alive even when empty so the
		// read-only web view can still show recent activity, including
		// one-shot posts from piped agents/scripts.
	}
	roomName := c.room
	h.mu.Unlock()

	if !c.Silent {
		h.Publish(Message{
			Room: roomName,
			From: "server",
			Text: c.Name + " left",
			Kind: KindSystem,
			Time: time.Now(),
		})
	}
}

// Publish records m in its room's history and fans it out to every client
// currently in that room.
func (h *Hub) Publish(m Message) {
	if m.Time.IsZero() {
		m.Time = time.Now()
	}
	h.mu.Lock()
	r := h.getOrCreate(m.Room)
	r.history = append(r.history, m)
	if len(r.history) > historyLimit {
		r.history = r.history[len(r.history)-historyLimit:]
	}
	targets := make([]*Client, 0, len(r.clients))
	for c := range r.clients {
		targets = append(targets, c)
	}
	h.mu.Unlock()

	for _, c := range targets {
		trySend(c, m)
	}
}

// RoomInfo summarizes a room for listings.
type RoomInfo struct {
	Name    string
	Members int
}

// Rooms returns all non-empty rooms, sorted by name.
func (h *Hub) Rooms() []RoomInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]RoomInfo, 0, len(h.rooms))
	for name, r := range h.rooms {
		members := 0
		for c := range r.clients {
			if !c.Silent { // don't count passive web viewers
				members++
			}
		}
		out = append(out, RoomInfo{Name: name, Members: members})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// History returns a copy of the named room's scrollback.
func (h *Hub) History(roomName string) []Message {
	h.mu.RLock()
	defer h.mu.RUnlock()
	r := h.rooms[roomName]
	if r == nil {
		return nil
	}
	return append([]Message(nil), r.history...)
}

// trySend delivers m to c without blocking; if c's buffer is full the
// message is dropped (the client is too slow to keep up).
func trySend(c *Client, m Message) {
	select {
	case c.Out <- m:
	default:
	}
}
