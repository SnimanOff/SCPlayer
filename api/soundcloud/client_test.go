package soundcloud

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func reply(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

type fakeCreds struct {
	id, token           string
	rotated, expired    string
	discoveries         int
	nextID, discoverErr string
}

func (f *fakeCreds) ClientID(context.Context) (string, error) {
	if f.id == "" {
		f.discoveries++
		f.id = f.nextID
	}
	return f.id, nil
}
func (f *fakeCreds) InvalidateClientID(id string) { f.rotated = id; f.id = "" }
func (f *fakeCreds) Token() string                { return f.token }
func (f *fakeCreds) Expire(t string)              { f.expired = t; f.token = "" }

func TestSearchRotatesClientIDAndRejectsForeignPages(t *testing.T) {
	creds := &fakeCreds{id: "old", nextID: "new", token: "user-token"}
	c := NewClient(creds)
	calls := 0
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "OAuth user-token" {
			t.Error("missing OAuth header")
		}
		if r.URL.Path != "/search/tracks" || r.URL.Query().Get("q") != "русская музыка & jazz" {
			t.Errorf("query not encoded correctly: %s", r.URL)
		}
		if r.URL.Query().Get("client_id") == "old" {
			return reply(401, ""), nil
		}
		return reply(200, `{"collection":[{"id":1,"title":"Track","policy":"SNIP"}],"next_href":"https://api-v2.soundcloud.com/search/tracks?q=x&offset=30"}`), nil
	})
	page, err := c.Search(context.Background(), "  русская музыка & jazz  ", "")
	if err != nil || len(page.Collection) != 1 || page.Collection[0].Access != "preview" || page.Next == "" || calls != 2 || creds.rotated != "old" || creds.expired != "" {
		t.Fatalf("client_id rotation failed: %+v %v calls=%d", page, err, calls)
	}
	for _, next := range []string{"https://evil.example/tracks", "//evil.example/tracks", "http://api-v2.soundcloud.com/tracks", "https://user@api-v2.soundcloud.com/tracks"} {
		if _, err := c.Search(context.Background(), "", next); err == nil {
			t.Fatalf("foreign pagination accepted: %s", next)
		}
	}
	if calls != 2 {
		t.Fatal("sent credentials outside API")
	}
}

func TestExpiredSessionFallsBackOrAsksToSignIn(t *testing.T) {
	creds := &fakeCreds{id: "id", nextID: "id", token: "stale"}
	c := NewClient(creds)
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" {
			return reply(401, ""), nil
		}
		return reply(200, `{"collection":[]}`), nil
	})
	if _, err := c.Search(context.Background(), "q", ""); err != nil || creds.expired != "stale" {
		t.Fatalf("public request did not fall back to anonymous: %v", err)
	}
	creds.token = "stale2"
	if _, err := c.Feed(context.Background()); !errors.Is(err, ErrLoginRequired) || creds.expired != "stale2" {
		t.Fatalf("private request with expired session: %v", err)
	}
	if _, err := c.Likes(context.Background()); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("anonymous likes: %v", err)
	}
}

func TestStreamPicksPlayableTranscoding(t *testing.T) {
	const media = `{"id":7,"policy":"ALLOW","track_authorization":"auth","media":{"transcodings":[
		{"url":"https://api-v2.soundcloud.com/media/7/enc/stream/hls","preset":"aac_160k","format":{"protocol":"ctr-encrypted-hls"}},
		{"url":"https://api-v2.soundcloud.com/media/7/abr/stream/hls","preset":"abr_sq","format":{"protocol":"hls"}},
		{"url":"https://api-v2.soundcloud.com/media/7/aac/stream/hls","preset":"aac_160k","format":{"protocol":"hls"}}]}}`
	for _, tc := range []struct{ name, aac, want string }{
		{"aac", `{"url":"https://cdn.example/aac.m3u8"}`, "https://cdn.example/aac.m3u8"},
		{"fallback", ``, "https://cdn.example/abr.m3u8"},
		{"unsafe", `{"url":"file:///tmp/audio"}`, "https://cdn.example/abr.m3u8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient(&fakeCreds{id: "id"})
			c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "api-v2.soundcloud.com" || strings.Contains(r.URL.Path, "/enc/") {
					t.Fatalf("unexpected request %s", r.URL)
				}
				switch r.URL.Path {
				case "/tracks/7":
					return reply(200, media), nil
				case "/media/7/aac/stream/hls":
					if r.URL.Query().Get("track_authorization") != "auth" || r.URL.Query().Get("client_id") != "id" {
						t.Error("stream request lacks authorization")
					}
					if tc.aac == "" {
						return reply(404, "{}"), nil
					}
					return reply(200, tc.aac), nil
				default:
					return reply(200, `{"url":"https://cdn.example/abr.m3u8"}`), nil
				}
			})
			got, err := c.Stream(context.Background(), Track{ID: 7})
			if got != tc.want || err != nil {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
	if _, err := NewClient(&fakeCreds{id: "id"}).Stream(context.Background(), Track{ID: 1, Access: "blocked"}); err == nil {
		t.Fatal("blocked track streamed")
	}
}

func TestPlaylistTracksFillsStubsInOrder(t *testing.T) {
	c := NewClient(&fakeCreds{id: "id"})
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/tracks" {
			if r.URL.Query().Get("ids") != "2,3" {
				t.Errorf("ids=%s", r.URL.Query().Get("ids"))
			}
			return reply(200, `[{"id":3,"title":"Three"},{"id":2,"title":"Two"}]`), nil
		}
		return reply(200, `{"id":9,"tracks":[{"id":1,"title":"One"},{"id":2},{"id":3}]}`), nil
	})
	page, err := c.PlaylistTracks(context.Background(), "9", "")
	if err != nil || len(page.Collection) != 3 || page.Collection[1].Title != "Two" || page.Collection[2].Title != "Three" || page.Next != "" {
		t.Fatalf("bad playlist: %+v %v", page, err)
	}
}

func TestRateLimitDoesNotWaitIndefinitely(t *testing.T) {
	c := NewClient(&fakeCreds{id: "id", token: "t"})
	c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		r := reply(429, "")
		r.Header.Set("Retry-After", "120")
		return r, nil
	})
	_, err := c.Me(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.RetryAfter != 120*time.Second {
		t.Fatalf("missing rate limit: %v", err)
	}
}

func TestPageReusedAcrossResponseFormats(t *testing.T) {
	var p Page[Track]
	for _, body := range []string{`{"collection":[{"id":1}],"next_href":"next"}`, `{"collection":[{"id":2}]}`, `[{"id":3,"publisher_metadata":{"artist":"A"},"user":{"username":"U"}}]`} {
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			t.Fatal(err)
		}
	}
	if p.Next != "" || len(p.Collection) != 1 || p.Collection[0].Key() != "3" || p.Collection[0].Creator() != "A" || p.Collection[0].Access != "playable" {
		t.Fatalf("stale page: %+v", p)
	}
	if (Playlist{}).Key() != "" {
		t.Fatal("empty playlist has an ID")
	}
}

func TestFeedFiltersAndDeduplicates(t *testing.T) {
	c := NewClient(&fakeCreds{id: "id", token: "t"})
	c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		return reply(200, `{"collection":[{"type":"track","track":{"id":1}},{"type":"track-repost","track":{"id":1}},{"type":"playlist","playlist":{"id":2}},{"type":"track","track":{"id":3}}]}`), nil
	})
	tracks, err := c.Feed(context.Background())
	if err != nil || len(tracks) != 2 || tracks[1].ID != 3 {
		t.Fatalf("bad feed: %+v %v", tracks, err)
	}
}

func TestMixesShelvesAndTracks(t *testing.T) {
	c := NewClient(&fakeCreds{id: "id", token: "t"})
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/mixed-selections":
			return reply(200, `{"collection":[
				{"title":"Mixed for Джони","items":{"collection":[
					{"id":"soundcloud:system-playlists:your-moods:1:1","kind":"system-playlist","title":"Your Mix 1","short_description":"A, B","calculated_artwork_url":"https://i1.sndcdn.com/mix.jpg","tracks":[{"id":5},{"id":4}]},
					{"id":"soundcloud:system-playlists:weekly:1","kind":"system-playlist","title":"Related tracks: Long Name","short_title":"Long Name"},
					{"id":77,"kind":"playlist","title":"Curated","artwork_url":"https://i1.sndcdn.com/p.jpg"},
					{"id":"unknown","kind":"thing"}]}},
				{"title":"Empty","items":{"collection":[]}}]}`), nil
		case "/tracks":
			return reply(200, `[{"id":4,"title":"Four"},{"id":5,"title":"Five"}]`), nil
		case "/system-playlists/soundcloud:system-playlists:weekly:1":
			return reply(200, `{"tracks":[{"id":4}]}`), nil
		case "/playlists/77":
			return reply(200, `{"id":77,"tracks":[{"id":5,"title":"Five"}]}`), nil
		}
		t.Fatalf("unexpected %s", r.URL)
		return nil, nil
	})
	shelves, err := c.Mixes(context.Background())
	if err != nil || len(shelves) != 1 || len(shelves[0].Mixes) != 3 {
		t.Fatalf("shelves: %+v %v", shelves, err)
	}
	mixes := shelves[0].Mixes
	if mixes[0].Description != "A, B" || mixes[0].ArtworkURL == "" || mixes[1].Title != "Long Name" || mixes[2].Key != "77" {
		t.Fatalf("mixes: %+v", mixes)
	}
	for i, want := range []string{"Five", "Four", "Five"} {
		tracks, err := c.MixTracks(context.Background(), mixes[i])
		if err != nil || len(tracks) == 0 || tracks[0].Title != want {
			t.Fatalf("mix %d: %+v %v", i, tracks, err)
		}
	}
}
