// Package config holds runtime settings for fiche-agentic.
//
// It carries forward the knobs from upstream fiche (domain, output dir,
// slug length, https, buffer size) and adds the ones the agentic fork
// needs: separate SSH and HTTP listen addresses and an SSH host key path.
package config

// Config is the fully-resolved runtime configuration.
type Config struct {
	// SSHAddr is the listen address for the SSH chat/paste server.
	SSHAddr string

	// HTTPAddr is the listen address for the read-only web viewer.
	HTTPAddr string

	// OutputDir is where pastes are stored on disk (one dir per slug,
	// matching upstream fiche's layout so an nginx root still works).
	OutputDir string

	// Domain is the host (and optional path) prepended to paste URLs.
	Domain string

	// HTTPS controls whether generated paste URLs use https://.
	HTTPS bool

	// SlugLen is the length of a generated paste slug.
	SlugLen int

	// BufferLen caps the size in bytes of a single piped paste.
	BufferLen int

	// HostKeyPath is the SSH server host key (generated on first run
	// if it does not exist).
	HostKeyPath string

	// DefaultRoom is the chat room a session joins when none is named.
	DefaultRoom string
}

// Default returns a Config populated with sensible development defaults.
func Default() Config {
	return Config{
		SSHAddr:     ":2222",
		HTTPAddr:    ":8080",
		OutputDir:   "./data/pastes",
		Domain:      "localhost:8080",
		HTTPS:       false,
		SlugLen:     4,
		BufferLen:   32 * 1024, // 32 KiB, same default as upstream fiche
		HostKeyPath: "./data/ssh_host_ed25519_key",
		DefaultRoom: "lobby",
	}
}
