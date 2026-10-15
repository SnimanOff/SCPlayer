package soundcloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var ErrLoginRequired = errors.New("войдите в SoundCloud")

type TokenStore interface {
	Load() (string, error)
	Save(string) error
	Delete() error
}

type Session struct {
	http      *http.Client
	store     TokenStore
	cachePath string
	home      string

	mu         sync.Mutex
	discoverMu sync.Mutex
	clientID   string
	token      string
	stale      string
}

func NewSession(store TokenStore, cachePath string) *Session {
	s := &Session{store: store, cachePath: cachePath, home: "https://soundcloud.com/",
		http: &http.Client{Timeout: 20 * time.Second}}
	if store != nil {
		s.token, _ = store.Load()
	}
	if cachePath != "" {
		if data, err := os.ReadFile(cachePath); err == nil && clientIDPattern.Match(data) {
			s.clientID = string(data)
		}
	}
	return s
}

func NewDesktopSession() *Session {
	cache := ""
	if dir, err := os.UserCacheDir(); err == nil {
		cache = filepath.Join(dir, "sc_player", "client_id")
	}
	return NewSession(KeyringStore{Account: "web-session"}, cache)
}

var (
	clientIDPattern = regexp.MustCompile(`^[A-Za-z0-9]{32}$`)
	hydrationID     = regexp.MustCompile(`"hydratable":"apiClient","data":\{"id":"([A-Za-z0-9]{32})"`)
	scriptID        = regexp.MustCompile(`client_id[:=]"([A-Za-z0-9]{32})"`)
	assetScript     = regexp.MustCompile(`<script crossorigin src="(https://a-v2\.sndcdn\.com/assets/[A-Za-z0-9._-]+\.js)"`)
	tokenPattern    = regexp.MustCompile(`^[A-Za-z0-9-]{16,256}$`)
)

func ValidToken(value string) bool { return tokenPattern.MatchString(value) }

func (s *Session) LoggedIn() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token != ""
}

func (s *Session) Token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

func (s *Session) Stale() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stale
}

func (s *Session) SetToken(token string) error {
	token = strings.TrimSpace(token)
	if !ValidToken(token) {
		return errors.New("SoundCloud вернул некорректную сессию")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = token
	if s.store != nil {
		if err := s.store.Save(token); err != nil {
			return fmt.Errorf("сохранение сессии: %w", err)
		}
	}
	return nil
}

func (s *Session) Expire(rejected string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rejected == "" || s.token != rejected {
		return
	}
	s.token, s.stale = "", rejected
	if s.store != nil {
		_ = s.store.Delete()
	}
}

func (s *Session) Logout() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = ""
	if s.store != nil {
		return s.store.Delete()
	}
	return nil
}

func (s *Session) ClientID(ctx context.Context) (string, error) {
	if id := s.cachedID(); id != "" {
		return id, nil
	}
	s.discoverMu.Lock()
	defer s.discoverMu.Unlock()
	if id := s.cachedID(); id != "" {
		return id, nil
	}
	id, err := s.discover(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("не удалось подключиться к SoundCloud: %w", err)
	}
	s.mu.Lock()
	s.clientID = id
	s.mu.Unlock()
	if s.cachePath != "" && os.MkdirAll(filepath.Dir(s.cachePath), 0o700) == nil {
		_ = os.WriteFile(s.cachePath, []byte(id), 0o600)
	}
	return id, nil
}

func (s *Session) InvalidateClientID(rejected string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clientID == rejected {
		s.clientID = ""
	}
}

func (s *Session) discover(ctx context.Context) (string, error) {
	page, err := s.fetch(ctx, s.home, 2<<20)
	if err != nil {
		return "", err
	}
	if m := hydrationID.FindSubmatch(page); m != nil {
		return string(m[1]), nil
	}
	scripts := assetScript.FindAllSubmatch(page, -1)

	for i := len(scripts) - 1; i >= 0 && i >= len(scripts)-8; i-- {
		body, err := s.fetch(ctx, string(scripts[i][1]), 8<<20)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			continue
		}
		if m := scriptID.FindSubmatch(body); m != nil {
			return string(m[1]), nil
		}
	}
	return "", errors.New("client_id не найден на soundcloud.com")
}

func (s *Session) fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"

func (s *Session) cachedID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clientID
}
