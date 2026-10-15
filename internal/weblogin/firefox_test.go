package weblogin

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeCookies(t *testing.T, profile string, rows [][]any) {
	t.Helper()
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", filepath.Join(profile, "cookies.sqlite"))
	if err == nil {
		err = db.Ping()
	}
	if err != nil {
		t.Skipf("SQLite unavailable (CGO off?): %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; CREATE TABLE moz_cookies (name TEXT, value TEXT, host TEXT, expiry INTEGER, lastAccessed INTEGER)`); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if _, err := db.Exec(`INSERT INTO moz_cookies VALUES (?, ?, ?, ?, ?)`, row...); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFirefoxProfilesAndSessionCookie(t *testing.T) {
	root := t.TempDir()
	absolute := filepath.Join(t.TempDir(), "elsewhere")
	ini := "[Profile0]\nName=old\nIsRelative=1\nPath=Profiles/old.default\n\n" +
		"[Profile1]\nName=moved\nIsRelative=0\nPath=" + absolute + "\n\n" +
		"[Profile2]\nName=empty\nIsRelative=1\nPath=Profiles/no-cookies\n\n" +
		"[Install308046B0AF4A39CB]\nDefault=Profiles/main.default-release\nLocked=1\n"
	if err := os.WriteFile(filepath.Join(root, "profiles.ini"), []byte(ini), 0o600); err != nil {
		t.Fatal(err)
	}
	future, past := time.Now().Add(time.Hour).Unix(), time.Now().Add(-time.Hour).Unix()
	writeCookies(t, filepath.Join(root, "Profiles", "old.default"), [][]any{
		{"oauth_token", "2-111111-1-OlderSession", ".soundcloud.com", future * 1000, 100},
	})
	writeCookies(t, filepath.Join(root, "Profiles", "main.default-release"), [][]any{
		{"oauth_token", "2-222222-2-NewestSession", ".soundcloud.com", future, 300},
		{"oauth_token", "2-333333-3-ExpiredSession", ".soundcloud.com", past, 900},
		{"oauth_token", "2-444444-4-ForeignSite", ".example.com", future, 900},
		{"sc_anonymous_id", "x", ".soundcloud.com", future, 900},
	})
	writeCookies(t, absolute, nil)
	os.MkdirAll(filepath.Join(root, "Profiles", "no-cookies"), 0o700)

	profiles := firefoxProfiles([]string{root, filepath.Join(root, "missing")})
	if len(profiles) != 3 {
		t.Fatalf("profiles: %v", profiles)
	}
	if got := firefoxToken(profiles); got != "2-222222-2-NewestSession" {
		t.Fatalf("token %q", got)
	}
	if got := firefoxToken([]string{absolute}); got != "" {
		t.Fatalf("empty profile gave %q", got)
	}
}

func TestFirefoxSkipsRejectedSession(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "main")
	future := time.Now().Add(time.Hour).Unix()
	writeCookies(t, profile, [][]any{{"oauth_token", "2-111111-1-ExpiredSession", ".soundcloud.com", future, 100}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	got := make(chan string, 1)
	go func() {
		token, _ := waitForFirefox(ctx, []string{profile}, "2-111111-1-ExpiredSession")
		got <- token
	}()
	select {
	case token := <-got:
		t.Fatalf("returned %q before a new sign-in", token)
	case <-time.After(1500 * time.Millisecond):
	}
	db, err := sql.Open("sqlite3", filepath.Join(profile, "cookies.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	_, err = db.Exec(`UPDATE moz_cookies SET value = '2-111111-1-FreshSession', lastAccessed = 200`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if token := <-got; token != "2-111111-1-FreshSession" {
		t.Fatalf("got %q", token)
	}
}
