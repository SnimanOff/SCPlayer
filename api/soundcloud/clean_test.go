package soundcloud

import (
	"encoding/json"
	"testing"
)

func TestCleanTextRemovesLinks(t *testing.T) {
	for in, want := range map[string]string{
		"Song https://hypeddit.com/abc":                    "Song",
		"Song (FREE DL: hypeddit.com/abc)":                 "Song",
		"SONG | T.ME/CHANNEL":                              "SONG",
		"Track [www.site.ru]":                              "Track",
		"Name - soundcloud.com/name":                       "Name",
		"TG: t.me/xxx | Song":                              "Song",
		"Song — Download: https://x.com/a?b=c":             "Song",
		"Song 🔥 bit.ly/x 🔥":                                "Song 🔥 🔥",
		"Song | booking: mail@gmail.com":                   "Song",
		"Artist x Artist2 - Song (prod. by Someone)":       "Artist x Artist2 - Song (prod. by Someone)",
		"feat.me x Vol.2 - AC/DC (Mr.Brown remix)":         "feat.me x Vol.2 - AC/DC (Mr.Brown remix)",
		"Live in St.Petersburg/Moscow":                     "Live in St.Petersburg/Moscow",
		"Song.mp3 | Track.wav":                             "Song.mp3 | Track.wav",
		"HITS.RU":                                          "",
		"Песня (скачать: сайт.рф/песня)":                   "Песня",
		"Песня | вк: vk.com/group | инст: instagram.com/x": "Песня",
		"DJ ZUP Ralii":                                     "DJ ZUP Ralii",
		"  extra   spaces  ":                               "extra spaces",
		"Hotmail.com remix":                                "remix",
		"Song [Free Download] https://a.b/c":               "Song [Free Download]",

		"Giá Như Anh Là Em |Lofi .| - Lệ Quyên": "Giá Như Anh Là Em |Lofi .| - Lệ Quyên",
		"~.~": "~.~",
		"Vì anh chẳng muốn thấy em khóc ,...":                      "Vì anh chẳng muốn thấy em khóc ,...",
		"deathmarried - athens (t.me/askatiu fixed)":               "deathmarried - athens (fixed)",
		"elyaplugg! - FRESH (LEAK BY T.ME/ELYAPLUGGSONGS)":         "elyaplugg! - FRESH",
		"NK - Попа Как У Ким (подпишись на instagram.com/Mullikk)": "NK - Попа Как У Ким",
		"hmUU on @icntxX on instagram.com":                         "hmUU on @icntxX",
		"SINISTER SLOWED BOOST  TRXNQL by t.me/slowear":            "SINISTER SLOWED BOOST TRXNQL",
		"пошел ты | телега - t.me/hescia":                          "пошел ты",
		"moosy + low honor | тг - t.me/hescia":                     "moosy + low honor",
		"Live at Wembley t.me/x":                                   "Live at Wembley",
		"t.me/m0ralfag / prod.  axxeron":                           "prod. axxeron",
	} {
		if got := cleanText(in); got != want {
			t.Errorf("cleanText(%q) = %q, want %q", in, got, want)
		}
	}
	if cleanTitle("https://t.me/only") != "Без названия" || cleanTitle("") != "" {
		t.Fatal("a title made only of a link must still have a name")
	}
}

func TestDecodedNamesHaveNoLinks(t *testing.T) {
	var track Track
	err := json.Unmarshal([]byte(`{"id":1,"title":"Song | t.me/promo","publisher_metadata":{"artist":"Artist (site.com)"},
		"user":{"username":"soundcloud.com/dj","permalink":"dj-name"}}`), &track)
	if err != nil {
		t.Fatal(err)
	}
	if track.Title != "Song" || track.Artist != "Artist" || track.User.Username != "dj-name" {
		t.Fatalf("links left: %+v", track)
	}
	var playlist Playlist
	if err := json.Unmarshal([]byte(`{"id":2,"title":"Mix www.promo.com","tracks":[{"id":3,"title":"A bit.ly/x"}]}`), &playlist); err != nil {
		t.Fatal(err)
	}
	if playlist.Title != "Mix" || playlist.Tracks[0].Title != "A" {
		t.Fatalf("links left: %+v", playlist)
	}
	var shelf Selection
	if err := json.Unmarshal([]byte(`{"title":"Mixed for dj site.com","items":{"collection":[
		{"id":"soundcloud:system-playlists:your-moods:1:1","kind":"system-playlist","title":"Your Mix 1","short_description":"A, t.me/b, C"}]}}`), &shelf); err != nil {
		t.Fatal(err)
	}
	if shelf.Title != "Mixed for dj" || shelf.Mixes[0].Description != "A, C" {
		t.Fatalf("links left: %+v", shelf)
	}
}
