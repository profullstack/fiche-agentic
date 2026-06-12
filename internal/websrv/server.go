// Package websrv is the read-only web side of fiche-agentic: it serves
// stored pastes (raw and pretty) and a live, read-only view of chat rooms
// via Server-Sent Events. Writing still happens over SSH.
package websrv

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/profullstack/fiche-agentic/internal/chat"
	"github.com/profullstack/fiche-agentic/internal/paste"
)

// Server bundles the HTTP handlers.
type Server struct {
	hub   *chat.Hub
	store *paste.Store
	tmpl  *template.Template
}

// New builds the web server.
func New(hub *chat.Hub, store *paste.Store) *Server {
	return &Server{
		hub:   hub,
		store: store,
		tmpl:  template.Must(template.New("").Parse(templates)),
	}
}

// Handler returns the configured HTTP mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.index)
	mux.HandleFunc("/r/", s.roomPage)    // /r/{room}
	mux.HandleFunc("/raw/", s.raw)       // /raw/{slug}
	mux.HandleFunc("/events/", s.events) // /events/{room} (SSE)
	mux.HandleFunc("/p/", s.pastePage)   // /p/{slug}
	// Bare /{slug} viewer last, so it doesn't shadow the prefixes above.
	mux.HandleFunc("/{slug}", s.pastePage)
	return mux
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		// Treat /{slug} as a paste view (mux sends unmatched here on older muxes).
		s.pastePage(w, r)
		return
	}
	pastes, _ := s.store.List(25)
	data := struct {
		Rooms  []chat.RoomInfo
		Pastes []paste.Info
	}{s.hub.Rooms(), pastes}
	s.render(w, "index", data)
}

func (s *Server) pastePage(w http.ResponseWriter, r *http.Request) {
	slug := lastSegment(r.URL.Path)
	if !paste.ValidSlug(slug) {
		http.NotFound(w, r)
		return
	}
	body, err := s.store.Read(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.render(w, "paste", struct {
		Slug string
		Body string
	}{slug, string(body)})
}

func (s *Server) raw(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/raw/")
	body, err := s.store.Read(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(body)
}

func (s *Server) roomPage(w http.ResponseWriter, r *http.Request) {
	room := strings.TrimPrefix(r.URL.Path, "/r/")
	if room == "" {
		http.NotFound(w, r)
		return
	}
	s.render(w, "room", struct {
		Room    string
		History []chat.Message
	}{room, s.hub.History(room)})
}

// events streams a room's live messages as SSE. The viewer joins the hub
// as a Silent client so it sees broadcasts but adds no join/leave noise.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	room := strings.TrimPrefix(r.URL.Path, "/events/")
	if room == "" {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	client := &chat.Client{Name: "web-viewer", Silent: true, Out: make(chan chat.Message, 64)}
	s.hub.Join(client, room)
	defer s.hub.Leave(client)

	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case m := <-client.Out:
			b, _ := json.Marshal(m)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func lastSegment(path string) string {
	path = strings.Trim(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}
