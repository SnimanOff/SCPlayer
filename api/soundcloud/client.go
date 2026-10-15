package soundcloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrSessionExpired = fmt.Errorf("сессия SoundCloud истекла — войдите снова: %w", ErrLoginRequired)

type Credentials interface {
	ClientID(context.Context) (string, error)
	InvalidateClientID(string)
	Token() string
	Expire(string)
}

type Client struct {
	baseURL string
	http    *http.Client
	creds   Credentials

	mu        sync.Mutex
	userToken string
	userID    int64
}

func NewClient(creds Credentials) *Client {
	return &Client{baseURL: "https://api-v2.soundcloud.com", creds: creds,
		http: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

type APIError struct {
	Status     int
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	switch e.Status {
	case 401:
		return "SoundCloud отклонил запрос; повторите попытку позже"
	case 403, 406:
		return "SoundCloud ограничил доступ к этому треку"
	case 404:
		return "трек или плейлист не найден"
	case 429:
		return "лимит запросов SoundCloud; повторите попытку позже"
	default:
		return fmt.Sprintf("SoundCloud: HTTP %d", e.Status)
	}
}

func (c *Client) endpoint(path string) (*url.URL, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	u = base.ResolveReference(u)
	if u.Scheme != base.Scheme || u.Host != base.Host || u.User != nil {
		return nil, errors.New("SoundCloud вернул ссылку вне API")
	}
	return u, nil
}

func (c *Client) get(ctx context.Context, path string, private bool, target any) error {
	endpoint, err := c.endpoint(path)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 3; attempt++ {
		id, err := c.creds.ClientID(ctx)
		if err != nil {
			return err
		}
		token := c.creds.Token()
		if private && token == "" {
			return ErrLoginRequired
		}
		query := endpoint.Query()
		query.Set("client_id", id)
		endpoint.RawQuery = query.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return err
		}
		if token != "" {
			req.Header.Set("Authorization", "OAuth "+token)
		}
		req.Header.Set("Accept", "application/json; charset=utf-8")
		req.Header.Set("User-Agent", userAgent)
		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New("нет соединения с SoundCloud")
		}
		if resp.StatusCode == http.StatusOK {
			err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(target)
			resp.Body.Close()
			return err
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			c.creds.InvalidateClientID(id)
			continue
		}
		if resp.StatusCode == http.StatusUnauthorized && token != "" {
			c.creds.Expire(token)
			if private {
				return ErrSessionExpired
			}
			continue
		}
		wait := time.Duration(1<<attempt) * time.Second
		if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && seconds > 0 {
			wait = time.Duration(seconds) * time.Second
		}
		if (resp.StatusCode == 429 || resp.StatusCode >= 500) && attempt < 2 && wait <= 5*time.Second {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
				continue
			}
		}
		return &APIError{Status: resp.StatusCode, RetryAfter: wait}
	}
	return errors.New("SoundCloud временно недоступен")
}

func (c *Client) Me(ctx context.Context) (User, error) {
	token := c.creds.Token()
	var user User
	if err := c.get(ctx, "/me", true, &user); err != nil {
		return user, err
	}
	c.mu.Lock()
	c.userToken, c.userID = token, user.ID
	c.mu.Unlock()
	return user, nil
}

func (c *Client) me(ctx context.Context) (string, error) {
	token := c.creds.Token()
	if token == "" {
		return "", ErrLoginRequired
	}
	c.mu.Lock()
	id := c.userID
	if c.userToken != token {
		id = 0
	}
	c.mu.Unlock()
	if id == 0 {
		user, err := c.Me(ctx)
		if err != nil {
			return "", err
		}
		id = user.ID
	}
	return strconv.FormatInt(id, 10), nil
}

func (c *Client) Search(ctx context.Context, query, next string) (Page[Track], error) {
	if next == "" {
		next = "/search/tracks?" + url.Values{"q": {strings.TrimSpace(query)}, "limit": {"30"}, "linked_partitioning": {"1"}}.Encode()
	}
	var page Page[Track]
	err := c.get(ctx, next, false, &page)
	return page, err
}

func (c *Client) Playlists(ctx context.Context, next string) (Page[Playlist], error) {
	var page Page[Playlist]
	if next == "" {
		id, err := c.me(ctx)
		if err != nil {
			return page, err
		}
		next = "/users/" + id + "/playlists_without_albums?limit=30&linked_partitioning=1"
	}
	err := c.get(ctx, next, true, &page)
	return page, err
}

func (c *Client) PlaylistTracks(ctx context.Context, key, _ string) (Page[Track], error) {
	var playlist Playlist
	if err := c.get(ctx, "/playlists/"+url.PathEscape(key)+"?representation=full", false, &playlist); err != nil {
		return Page[Track]{}, err
	}
	tracks, err := c.fill(ctx, playlist.Tracks)
	return Page[Track]{Collection: tracks}, err
}

func (c *Client) fill(ctx context.Context, tracks []Track) ([]Track, error) {
	full := make(map[int64]Track, len(tracks))
	var missing []string
	for _, track := range tracks {
		if track.Title != "" {
			full[track.ID] = track
		} else if track.ID != 0 {
			missing = append(missing, track.Key())
		}
	}
	for len(missing) > 0 {
		batch := missing[:min(50, len(missing))]
		missing = missing[len(batch):]
		var loaded []Track
		if err := c.get(ctx, "/tracks?ids="+strings.Join(batch, ","), false, &loaded); err != nil {
			return nil, err
		}
		for _, track := range loaded {
			full[track.ID] = track
		}
	}
	result := make([]Track, 0, len(tracks))
	for _, track := range tracks {
		if track, ok := full[track.ID]; ok {
			result = append(result, track)
		}
	}
	return result, nil
}

func (c *Client) Mixes(ctx context.Context) ([]Selection, error) {
	var page Page[Selection]
	if err := c.get(ctx, "/mixed-selections?limit=20", false, &page); err != nil {
		return nil, err
	}
	shelves := make([]Selection, 0, len(page.Collection))
	for _, shelf := range page.Collection {
		if len(shelf.Mixes) > 0 {
			shelves = append(shelves, shelf)
		}
	}
	return shelves, nil
}

func (c *Client) MixTracks(ctx context.Context, mix Mix) ([]Track, error) {
	if len(mix.Tracks) > 0 {
		return c.fill(ctx, mix.Tracks)
	}
	if !strings.HasPrefix(mix.Key, "soundcloud:system-playlists:") {
		page, err := c.PlaylistTracks(ctx, mix.Key, "")
		return page.Collection, err
	}
	var playlist struct {
		Tracks []Track `json:"tracks"`
	}
	if err := c.get(ctx, "/system-playlists/"+url.PathEscape(mix.Key), false, &playlist); err != nil {
		return nil, err
	}
	return c.fill(ctx, playlist.Tracks)
}

func (c *Client) Likes(ctx context.Context) (Page[Track], error) {
	id, err := c.me(ctx)
	if err != nil {
		return Page[Track]{}, err
	}
	var likes Page[struct {
		Track Track `json:"track"`
	}]
	if err := c.get(ctx, "/users/"+id+"/track_likes?limit=30&linked_partitioning=1", true, &likes); err != nil {
		return Page[Track]{}, err
	}
	page := Page[Track]{Next: likes.Next}
	for _, like := range likes.Collection {
		if like.Track.Key() != "" {
			page.Collection = append(page.Collection, like.Track)
		}
	}
	return page, nil
}

func (c *Client) Related(ctx context.Context, key string) (Page[Track], error) {
	var page Page[Track]
	err := c.get(ctx, "/tracks/"+url.PathEscape(key)+"/related?limit=30&linked_partitioning=1", false, &page)
	return page, err
}

func (c *Client) Feed(ctx context.Context) ([]Track, error) {
	var page Page[struct {
		Type  string `json:"type"`
		Track Track  `json:"track"`
	}]
	if err := c.get(ctx, "/stream?limit=30&linked_partitioning=1", true, &page); err != nil {
		return nil, err
	}
	tracks := make([]Track, 0, len(page.Collection))
	seen := map[string]bool{}
	for _, item := range page.Collection {
		key := item.Track.Key()
		if strings.HasPrefix(item.Type, "track") && key != "" && !seen[key] {
			tracks = append(tracks, item.Track)
			seen[key] = true
		}
	}
	return tracks, nil
}

func streamRank(t Transcoding) int {
	switch {
	case t.Format.Protocol == "hls" && strings.HasPrefix(t.Preset, "aac"):
		return 0
	case t.Format.Protocol == "progressive":
		return 1
	case t.Format.Protocol == "hls" && !strings.HasPrefix(t.Preset, "abr"):
		return 2
	case t.Format.Protocol == "hls":
		return 3
	default:
		return -1
	}
}

func (c *Client) Stream(ctx context.Context, track Track) (string, error) {
	if track.Access == "blocked" {
		return "", errors.New("этот трек недоступен для воспроизведения вне SoundCloud")
	}
	if track.Key() == "" {
		return "", errors.New("у трека нет идентификатора")
	}
	if len(track.Media.Transcodings) == 0 || track.TrackAuthorization == "" {
		if err := c.get(ctx, "/tracks/"+track.Key(), false, &track); err != nil {
			return "", err
		}
	}
	var candidates []Transcoding
	for _, t := range track.Media.Transcodings {
		if streamRank(t) >= 0 {
			candidates = append(candidates, t)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return streamRank(candidates[i]) < streamRank(candidates[j]) })
	err := errors.New("SoundCloud не предоставил аудиопоток")
	for _, t := range candidates {
		var resolved struct {
			URL string `json:"url"`
		}
		path := t.URL + "?" + url.Values{"track_authorization": {track.TrackAuthorization}}.Encode()
		if err = c.get(ctx, path, false, &resolved); err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			continue
		}
		u, parseErr := url.Parse(resolved.URL)
		if parseErr != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			err = errors.New("некорректный адрес аудиопотока")
			continue
		}
		return resolved.URL, nil
	}
	return "", err
}
