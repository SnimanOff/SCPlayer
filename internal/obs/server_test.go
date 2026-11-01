package obs

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"sc_player/api/soundcloud"
	"sc_player/player"
)

func request(t *testing.T, s *Server, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	s.serveHTTP(w, httptest.NewRequest(method, "http://127.0.0.1:17345"+path, nil))
	return w
}

func readState(t *testing.T, s *Server) snapshot {
	t.Helper()
	w := request(t, s, http.MethodGet, "/api/now-playing")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body)
	}
	var got snapshot
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestPlaybackStates(t *testing.T) {
	s := New("")
	if got := readState(t, s); got.Active {
		t.Fatal("new server must start hidden")
	}
	track := soundcloud.Track{ID: 1, Title: strings.Repeat("Длинное название 🎶 ", 30) + "<script>alert(1)</script>", Artist: "Исполнитель", ArtworkURL: "https://i1.sndcdn.com/art.jpg", TrackAuthorization: "private-token"}
	s.Update(track, player.State{})
	got := readState(t, s)
	if !got.Active || got.Title != track.Title || got.Artist != track.Artist || got.Artwork != track.ArtworkURL || got.Paused {
		t.Fatalf("playing state = %+v", got)
	}
	body := request(t, s, http.MethodGet, "/api/now-playing").Body.String()
	if strings.Contains(body, "<script>") || strings.Contains(body, "private-token") {
		t.Fatalf("unsafe or private metadata in JSON: %s", body)
	}
	s.Update(track, player.State{Paused: true})
	if got := readState(t, s); !got.Active || !got.Paused {
		t.Fatalf("paused state = %+v", got)
	}
	s.Update(track, player.State{Idle: true})
	if got := readState(t, s); got != (snapshot{}) {
		t.Fatalf("idle state retains metadata: %+v", got)
	}
	s.Update(soundcloud.Track{}, player.State{})
	if got := readState(t, s); got.Active {
		t.Fatal("no track must stay hidden")
	}
}

func TestArtworkAndCreator(t *testing.T) {
	s := New("")
	track := soundcloud.Track{ID: 1, User: soundcloud.User{Username: "Uploader", AvatarURL: "https://i1.sndcdn.com/avatar.jpg"}}
	s.Update(track, player.State{})
	if got := readState(t, s); got.Artwork != track.User.AvatarURL || got.Artist != "Uploader" {
		t.Fatalf("fallback metadata = %+v", got)
	}
	for _, artwork := range []string{"javascript:alert(1)", "file:///secret", "http://127.0.0.1/private", "https://", "https://user:password@example.com/art.jpg", "%"} {
		track.ArtworkURL = artwork
		s.Update(track, player.State{})
		if got := readState(t, s); got.Artwork != "" {
			t.Errorf("accepted invalid artwork %q", artwork)
		}
	}
}

func TestHTTPContract(t *testing.T) {
	s := New("")
	for _, path := range []string{"/obs", "/obs/", "/api/now-playing"} {
		w := request(t, s, http.MethodGet, path)
		if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("GET %s: code %d, headers %v", path, w.Code, w.Header())
		}
		head := request(t, s, http.MethodHead, path)
		if head.Code != http.StatusOK || head.Body.Len() != 0 {
			t.Fatalf("HEAD %s: code %d, body length %d", path, head.Code, head.Body.Len())
		}
		post := request(t, s, http.MethodPost, path)
		if post.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST %s: code %d", path, post.Code)
		}
	}
	for _, path := range []string{"/", "/other", "/../../sc-player.json", "/api/play"} {
		if w := request(t, s, http.MethodGet, path); w.Code != http.StatusNotFound {
			t.Errorf("GET %s: code %d", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	s.serveHTTP(w, httptest.NewRequest(http.MethodGet, "http://attacker.example/api/now-playing", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("foreign Host accepted: %d", w.Code)
	}
}

func TestServerLifecycle(t *testing.T) {
	s := New("127.0.0.1:0")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	firstURL := s.URL()
	if err := s.Start(); err != nil || s.URL() != firstURL {
		t.Fatalf("second Start: %v, URL %s", err, s.URL())
	}
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(firstURL)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(body), "SC Player") {
		t.Fatalf("widget request: %d, %v", response.StatusCode, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if response, err := client.Get(firstURL); err == nil {
		response.Body.Close()
		t.Fatal("closed server still accepts connections")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("restart: %v", err)
	}
	response, err = client.Get(s.URL())
	if err != nil {
		t.Fatalf("restarted server: %v", err)
	}
	response.Body.Close()
}

func TestInvalidOrOccupiedAddress(t *testing.T) {
	for _, address := range []string{"0.0.0.0:17345", "192.168.1.1:17345", ":17345", "localhost:17345", "invalid"} {
		s := New(address)
		if err := s.Start(); err == nil {
			_ = s.Close()
			t.Errorf("non-loopback address accepted: %s", address)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := New(listener.Addr().String())
	defer s.Close()
	if err := s.Start(); err == nil {
		t.Error("occupied port did not report an error")
	}
	listener.Close()
	if err := s.Start(); err != nil {
		t.Fatalf("retry after port was freed: %v", err)
	}
}

func TestImmediateRestartOnSamePort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	s := New(address)
	defer s.Close()
	for i := range 100 {
		if err := s.Start(); err != nil {
			t.Fatalf("start %d: %v", i, err)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("close %d: %v", i, err)
		}
	}
}

func TestConcurrentUpdates(t *testing.T) {
	s := New("")
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				s.Update(soundcloud.Track{ID: int64(i + 1), Title: "Track", Artist: "Artist"}, player.State{Paused: i%2 == 0})
				got := readState(t, s)
				if !got.Active || got.Title != "Track" || got.Artist != "Artist" {
					t.Errorf("incomplete snapshot: %+v", got)
					return
				}
			}
		}()
	}
	wg.Wait()
}
