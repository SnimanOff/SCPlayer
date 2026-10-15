package weblogin

import (
	"bufio"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func firefoxRoots() []string {
	var roots []string
	if appData := os.Getenv("APPDATA"); appData != "" {
		roots = append(roots, filepath.Join(appData, "Mozilla", "Firefox"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots,
			filepath.Join(home, ".mozilla", "firefox"),
			filepath.Join(home, ".config", "mozilla", "firefox"),
			filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox"),
			filepath.Join(home, ".var", "app", "org.mozilla.firefox", ".mozilla", "firefox"))
	}
	return roots
}

func firefoxProfiles(roots []string) []string {
	seen := map[string]bool{}
	var profiles []string
	for _, root := range roots {
		file, err := os.Open(filepath.Join(root, "profiles.ini"))
		if err != nil {
			continue
		}
		var section string
		relative := map[string]bool{}
		paths := map[string]string{}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				section = line[1 : len(line)-1]
				relative[section] = true
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			switch {
			case !ok:
			case key == "IsRelative":
				relative[section] = value != "0"
			case key == "Path" && strings.HasPrefix(section, "Profile"),
				key == "Default" && strings.HasPrefix(section, "Install"):
				paths[section] = value
			}
		}
		file.Close()
		for section, path := range paths {
			if relative[section] {
				path = filepath.Join(root, filepath.FromSlash(path))
			}
			if _, err := os.Stat(filepath.Join(path, "cookies.sqlite")); err == nil && !seen[path] {
				seen[path] = true
				profiles = append(profiles, path)
			}
		}
	}
	return profiles
}

func cookieSnapshot(profile string) (token string, accessed int64, err error) {
	dir, err := os.MkdirTemp("", "sc-player-cookies-")
	if err != nil {
		return "", 0, err
	}
	defer os.RemoveAll(dir)
	for _, name := range []string{"cookies.sqlite", "cookies.sqlite-wal"} {
		if err := copyFile(filepath.Join(profile, name), filepath.Join(dir, name)); err != nil && name == "cookies.sqlite" {
			return "", 0, err
		}
	}
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(filepath.Join(dir, "cookies.sqlite")))
	if err != nil {
		return "", 0, err
	}
	defer db.Close()

	err = db.QueryRow(`SELECT value, lastAccessed FROM moz_cookies
		WHERE name = 'oauth_token' AND host IN ('.soundcloud.com', 'soundcloud.com') AND expiry > ?
		ORDER BY lastAccessed DESC LIMIT 1`, time.Now().Unix()).Scan(&token, &accessed)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, nil
	}
	return token, accessed, err
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func firefoxToken(profiles []string) string {
	var best string
	var newest int64
	for _, profile := range profiles {
		token, accessed, err := cookieSnapshot(profile)
		if err == nil && token != "" && accessed >= newest {
			best, newest = token, accessed
		}
	}
	return best
}
