// Command fiche-agentic is an SSH + web pastebin and shared chat room for
// humans and AI agents. It is a Go rewrite of upstream fiche
// (https://github.com/solusipse/fiche): the pastebin core is preserved,
// and an SSH chat server (charmbracelet/wish) plus a read-only web viewer
// are added on top.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/profullstack/fiche-agentic/internal/chat"
	"github.com/profullstack/fiche-agentic/internal/config"
	"github.com/profullstack/fiche-agentic/internal/paste"
	"github.com/profullstack/fiche-agentic/internal/sshsrv"
	"github.com/profullstack/fiche-agentic/internal/websrv"
)

func main() {
	cfg := config.Default()

	// Upstream-fiche flags (kept for familiarity)...
	flag.StringVar(&cfg.Domain, "d", cfg.Domain, "domain prefixed to paste URLs")
	flag.StringVar(&cfg.OutputDir, "o", cfg.OutputDir, "output directory for pastes")
	flag.IntVar(&cfg.SlugLen, "s", cfg.SlugLen, "paste slug length")
	flag.BoolVar(&cfg.HTTPS, "S", cfg.HTTPS, "use https:// in paste URLs")
	flag.IntVar(&cfg.BufferLen, "B", cfg.BufferLen, "max paste size in bytes")
	// ...plus fiche-agentic additions.
	flag.StringVar(&cfg.SSHAddr, "ssh", cfg.SSHAddr, "SSH listen address")
	flag.StringVar(&cfg.HTTPAddr, "http", cfg.HTTPAddr, "HTTP listen address")
	flag.StringVar(&cfg.HostKeyPath, "hostkey", cfg.HostKeyPath, "SSH host key path (generated if missing)")
	flag.StringVar(&cfg.DefaultRoom, "room", cfg.DefaultRoom, "default chat room")
	flag.Parse()

	store, err := paste.New(cfg.OutputDir, cfg.SlugLen, cfg.BufferLen, cfg.Domain, cfg.HTTPS)
	if err != nil {
		log.Fatalf("paste store: %v", err)
	}
	hub := chat.New()

	// Web server.
	web := websrv.New(hub, store)
	httpSrv := &http.Server{Addr: cfg.HTTPAddr, Handler: web.Handler()}
	go func() {
		log.Printf("web   listening on %s", cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	// SSH server.
	sshSrv, err := sshsrv.New(cfg, hub, store)
	if err != nil {
		log.Fatalf("ssh server: %v", err)
	}
	go func() {
		log.Printf("ssh   listening on %s", cfg.SSHAddr)
		if err := sshSrv.ListenAndServe(); err != nil {
			log.Printf("ssh: %v", err)
		}
	}()

	// Wait for a signal, then shut down both servers.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	log.Println("shutting down…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	_ = sshSrv.Shutdown(shutdownCtx)
}
