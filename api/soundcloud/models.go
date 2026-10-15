package soundcloud

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

type User struct {
	ID        int64  `json:"id"`
	URN       string `json:"urn"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}

func (u *User) UnmarshalJSON(data []byte) error {
	type plain User
	var decoded struct {
		plain
		Permalink string `json:"permalink"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*u = User(decoded.plain)
	if u.Username = cleanText(u.Username); u.Username == "" {
		u.Username = decoded.Permalink
	}
	return nil
}

type Transcoding struct {
	URL     string `json:"url"`
	Preset  string `json:"preset"`
	Snipped bool   `json:"snipped"`
	Format  struct {
		Protocol string `json:"protocol"`
		MimeType string `json:"mime_type"`
	} `json:"format"`
}

type Track struct {
	ID                 int64  `json:"id"`
	URN                string `json:"urn"`
	Title              string `json:"title"`
	User               User   `json:"user"`
	Artist             string `json:"-"`
	ArtworkURL         string `json:"artwork_url"`
	PermalinkURL       string `json:"permalink_url"`
	Duration           int64  `json:"duration"`
	Access             string `json:"-"`
	Policy             string `json:"policy"`
	Streamable         bool   `json:"streamable"`
	TrackAuthorization string `json:"track_authorization"`
	Media              struct {
		Transcodings []Transcoding `json:"transcodings"`
	} `json:"media"`
}

func (t *Track) UnmarshalJSON(data []byte) error {
	type plain Track
	var decoded struct {
		plain
		Publisher struct {
			Artist string `json:"artist"`
		} `json:"publisher_metadata"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*t = Track(decoded.plain)
	t.Title = cleanTitle(t.Title)
	t.Artist = cleanText(decoded.Publisher.Artist)
	switch t.Policy {
	case "BLOCK":
		t.Access = "blocked"
	case "SNIP":
		t.Access = "preview"
	default:
		t.Access = "playable"
	}
	return nil
}

func (t Track) Key() string {
	if t.ID != 0 {
		return strconv.FormatInt(t.ID, 10)
	}
	return ""
}

func (t Track) Creator() string {
	if t.Artist != "" {
		return t.Artist
	}
	return t.User.Username
}

type Playlist struct {
	ID         int64   `json:"id"`
	URN        string  `json:"urn"`
	Title      string  `json:"title"`
	User       User    `json:"user"`
	TrackCount int     `json:"track_count"`
	Tracks     []Track `json:"tracks"`
}

func (p *Playlist) UnmarshalJSON(data []byte) error {
	type plain Playlist
	if err := json.Unmarshal(data, (*plain)(p)); err != nil {
		return err
	}
	p.Title = cleanTitle(p.Title)
	return nil
}

func (p Playlist) Key() string {
	if p.ID == 0 {
		return ""
	}
	return strconv.FormatInt(p.ID, 10)
}

type Page[T any] struct {
	Collection []T    `json:"collection"`
	Next       string `json:"next_href"`
}

func (p *Page[T]) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] == '[' {
		p.Next = ""
		return json.Unmarshal(data, &p.Collection)
	}
	type plain Page[T]
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = Page[T](decoded)
	return nil
}

type Selection struct {
	URN   string
	Title string
	Mixes []Mix
}

type Mix struct {
	Key         string
	Title       string
	Description string
	ArtworkURL  string
	Tracks      []Track
}

func (m *Mix) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID                   json.RawMessage `json:"id"`
		Kind                 string          `json:"kind"`
		Title                string          `json:"title"`
		ShortTitle           string          `json:"short_title"`
		ShortDescription     string          `json:"short_description"`
		ArtworkURL           string          `json:"artwork_url"`
		CalculatedArtworkURL string          `json:"calculated_artwork_url"`
		Tracks               []Track         `json:"tracks"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	var urn string
	var id int64
	if json.Unmarshal(raw.ID, &urn) == nil && strings.HasPrefix(urn, "soundcloud:system-playlists:") {
		m.Key = urn
	} else if json.Unmarshal(raw.ID, &id) == nil && id != 0 && raw.Kind == "playlist" {
		m.Key = strconv.FormatInt(id, 10)
	}
	m.Title = cleanTitle(raw.Title)
	if raw.ShortTitle != "" {
		m.Title = cleanTitle(raw.ShortTitle)
	}
	m.Description = cleanText(raw.ShortDescription)
	m.ArtworkURL = raw.ArtworkURL
	if m.ArtworkURL == "" {
		m.ArtworkURL = raw.CalculatedArtworkURL
	}
	m.Tracks = raw.Tracks
	return nil
}

func (s *Selection) UnmarshalJSON(data []byte) error {
	var raw struct {
		URN   string `json:"urn"`
		Title string `json:"title"`
		Items struct {
			Collection []Mix `json:"collection"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.URN, s.Title = raw.URN, cleanText(raw.Title)
	s.Mixes = s.Mixes[:0]
	for _, mix := range raw.Items.Collection {
		if mix.Key != "" {
			s.Mixes = append(s.Mixes, mix)
		}
	}
	return nil
}
