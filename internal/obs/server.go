// Package obs serves a local, read-only now-playing widget for OBS Browser Sources.
package obs

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"sc_player/api/soundcloud"
	"sc_player/player"
)

const DefaultAddress = "127.0.0.1:17345"

//go:embed widget.html
var widgetHTML []byte

type snapshot struct {
	Active  bool   `json:"active"`
	Title   string `json:"title"`
	Artist  string `json:"artist"`
	Artwork string `json:"artwork"`
	Paused  bool   `json:"paused"`
}

type Server struct {
	mu            sync.Mutex
	address       string
	activeAddress string
	server        *http.Server
	listener      net.Listener
	stateMu       sync.RWMutex
	state         snapshot
}

func New(address string) *Server {
	if address == "" {
		address = DefaultAddress
	}
	return &Server{address: address}
}

// Start is idempotent. Only literal loopback addresses are accepted so that the
// widget never exposes the user's listening activity to the local network.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		return nil
	}
	host, _, err := net.SplitHostPort(s.address)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return fmt.Errorf("OBS: адрес должен быть локальным (например, %s)", DefaultAddress)
	}
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return fmt.Errorf("OBS: не удалось открыть %s: %w", s.address, err)
	}
	s.activeAddress = listener.Addr().String()
	s.listener = listener
	s.server = &http.Server{
		Handler:           http.HandlerFunc(s.serveHTTP),
		ReadHeaderTimeout: 3 * time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	server := s.server
	go func() { _ = server.Serve(listener) }()
	return nil
}

// Close immediately disconnects Browser Sources. A later Start may reuse this
// Server, including its latest playback snapshot.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server == nil {
		return nil
	}
	// Close the listener ourselves as Serve may not have registered it with the
	// HTTP server yet. This makes an immediate disable/re-enable reliable.
	_ = s.listener.Close()
	err := s.server.Close()
	if errors.Is(err, net.ErrClosed) {
		err = nil
	}
	s.server = nil
	s.listener = nil
	s.activeAddress = ""
	return err
}

func (s *Server) URL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	address := s.address
	if s.activeAddress != "" {
		address = s.activeAddress
	}
	return "http://" + address + "/obs"
}

// Update copies only display metadata; session tokens and stream URLs never
// leave the player. Long strings remain intact and are ellipsized by the widget
// at its actual rendered width, including Unicode titles.
func (s *Server) Update(track soundcloud.Track, state player.State) {
	var next snapshot
	if track.Key() != "" && !state.Idle {
		artwork := track.ArtworkURL
		if artwork == "" {
			artwork = track.User.AvatarURL
		}
		parsed, err := url.Parse(artwork)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			artwork = ""
		}
		next = snapshot{Active: true, Title: track.Title, Artist: track.Creator(), Artwork: artwork, Paused: state.Paused}
	}
	s.stateMu.Lock()
	s.state = next
	s.stateMu.Unlock()
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	// Reject DNS rebinding requests even though the socket is loopback-only.
	host := r.Host
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	if !net.ParseIP(host).IsLoopback() {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch r.URL.Path {
	case "/obs", "/obs/":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; img-src https:; base-uri 'none'")
		if r.Method != http.MethodHead {
			_, _ = w.Write(widgetHTML)
		}
	case "/api/now-playing":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if r.Method != http.MethodHead {
			s.stateMu.RLock()
			state := s.state
			s.stateMu.RUnlock()
			_ = json.NewEncoder(w).Encode(state)
		}
	default:
		http.NotFound(w, r)
	}
}
