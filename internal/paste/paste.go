// Package paste is the pastebin core, ported from upstream fiche (fiche.c).
//
// Like upstream, each paste is written to its own directory named by a
// random slug, with the content in an index.txt file, so the on-disk
// layout can be served directly by a static web server. Slug generation,
// the symbol alphabet, and the default sizes mirror fiche.
package paste

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Symbols is the alphabet used for slug generation (matches fiche's
// Fiche_Symbols: a-z A-Z 0-9).
const Symbols = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// Store creates and reads pastes under a single output directory.
type Store struct {
	Dir       string
	SlugLen   int
	Domain    string
	HTTPS     bool
	BufferLen int
}

// Info is a lightweight description of a stored paste.
type Info struct {
	Slug    string
	Size    int64
	Created time.Time
}

// New returns a Store and ensures the output directory exists.
func New(dir string, slugLen, bufferLen int, domain string, https bool) (*Store, error) {
	if slugLen < 1 {
		slugLen = 4
	}
	if bufferLen < 1 {
		bufferLen = 32 * 1024
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}
	return &Store{
		Dir:       dir,
		SlugLen:   slugLen,
		Domain:    domain,
		HTTPS:     https,
		BufferLen: bufferLen,
	}, nil
}

// Create stores data as a new paste and returns its slug. Input larger
// than BufferLen is truncated, mirroring fiche's fixed-buffer behaviour.
func (s *Store) Create(data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("refusing to store empty paste")
	}
	if len(data) > s.BufferLen {
		data = data[:s.BufferLen]
	}

	slug, err := s.uniqueSlug()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(s.Dir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create paste dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.txt"), data, 0o644); err != nil {
		return "", fmt.Errorf("write paste: %w", err)
	}
	return slug, nil
}

// Read returns the stored content for a slug.
func (s *Store) Read(slug string) ([]byte, error) {
	if !ValidSlug(slug) {
		return nil, fmt.Errorf("invalid slug")
	}
	return os.ReadFile(filepath.Join(s.Dir, slug, "index.txt"))
}

// URL builds the public URL for a slug, matching fiche's scheme+domain+slug.
func (s *Store) URL(slug string) string {
	scheme := "http"
	if s.HTTPS {
		scheme = "https"
	}
	domain := strings.TrimRight(s.Domain, "/")
	return fmt.Sprintf("%s://%s/%s", scheme, domain, slug)
}

// List returns up to limit pastes, newest first.
func (s *Store) List(limit int) ([]Info, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}
	var out []Info
	for _, e := range entries {
		if !e.IsDir() || !ValidSlug(e.Name()) {
			continue
		}
		fi, err := os.Stat(filepath.Join(s.Dir, e.Name(), "index.txt"))
		if err != nil {
			continue
		}
		out = append(out, Info{Slug: e.Name(), Size: fi.Size(), Created: fi.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// uniqueSlug generates a random slug that does not already exist on disk,
// growing the length if it keeps colliding (same strategy as fiche).
func (s *Store) uniqueSlug() (string, error) {
	length := s.SlugLen
	for attempts := 0; attempts < 64; attempts++ {
		slug, err := randomSlug(length)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(s.Dir, slug)); os.IsNotExist(err) {
			return slug, nil
		}
		if attempts > 0 && attempts%8 == 0 {
			length++ // too many collisions at this length, widen it
		}
	}
	return "", fmt.Errorf("could not allocate a free slug")
}

func randomSlug(n int) (string, error) {
	var b strings.Builder
	max := big.NewInt(int64(len(Symbols)))
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b.WriteByte(Symbols[idx.Int64()])
	}
	return b.String(), nil
}

// ValidSlug reports whether name is composed only of slug characters,
// guarding the web and read paths against directory traversal.
func ValidSlug(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !strings.ContainsRune(Symbols, rune(name[i])) {
			return false
		}
	}
	return true
}
