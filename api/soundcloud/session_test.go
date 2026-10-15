package soundcloud

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memoryStore struct{ token string }

func (m *memoryStore) Load() (string, error) { return m.token, nil }
func (m *memoryStore) Save(t string) error   { m.token = t; return nil }
func (m *memoryStore) Delete() error         { m.token = ""; return nil }

const testID = "AbCdEfGhIjKlMnOpQrStUvWxYz012345"

func TestClientIDDiscoveryAndCache(t *testing.T) {
	for name, pages := range map[string]map[string]string{
		"hydration": {"/": `<script>window.__sc_hydration = [{"hydratable":"apiClient","data":{"id":"` + testID + `","isExpiring":false}}]</script>`},
		"bundle": {
			"/":              `<script crossorigin src="https://a-v2.sndcdn.com/assets/1-a.js"></script><script crossorigin src="https://a-v2.sndcdn.com/assets/2-b.js"></script>`,
			"/assets/2-b.js": `var x={}`,
			"/assets/1-a.js": `n=({client_id:"` + testID + `",env:"production"})`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			cache := filepath.Join(t.TempDir(), "sc", "client_id")
			s := NewSession(nil, cache)
			requests := 0
			s.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
				requests++
				body, ok := pages[r.URL.Path]
				if !ok {
					return reply(404, ""), nil
				}
				return reply(200, body), nil
			})
			id, err := s.ClientID(context.Background())
			if err != nil || id != testID {
				t.Fatalf("got %q, %v", id, err)
			}
			if data, _ := os.ReadFile(cache); string(data) != testID {
				t.Fatal("client_id not cached")
			}
			before := requests
			if id, _ := NewSession(nil, cache).ClientID(context.Background()); id != testID || requests != before {
				t.Fatal("cached client_id not reused")
			}
			s.InvalidateClientID(testID)
			if _, err := s.ClientID(context.Background()); err != nil || requests == before {
				t.Fatal("rotated client_id not rediscovered")
			}
		})
	}
}

func TestSessionToken(t *testing.T) {
	store := &memoryStore{token: "2-123456-789-AbCdEf"}
	s := NewSession(store, "")
	if !s.LoggedIn() {
		t.Fatal("stored session not restored")
	}
	for _, bad := range []string{"", "short", "has space in it 12345", strings.Repeat("a", 300), "x;y=1234567890123456"} {
		if s.SetToken(bad) == nil {
			t.Fatalf("accepted token %q", bad)
		}
	}
	if err := s.SetToken(" 2-999999-111-ZyXwVu "); err != nil || store.token != "2-999999-111-ZyXwVu" {
		t.Fatal("token not saved")
	}
	s.Expire("older")
	if !s.LoggedIn() {
		t.Fatal("expired a newer token")
	}
	s.Expire("2-999999-111-ZyXwVu")
	if s.LoggedIn() || store.token != "" {
		t.Fatal("rejected token kept")
	}
	if s.Stale() != "2-999999-111-ZyXwVu" {
		t.Fatal("rejected token not remembered for sign-in")
	}
}
