package weblogin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"sc_player/api/soundcloud"
)

const Flag = "--soundcloud-login"

const signInURL = "https://soundcloud.com/signin"

var ErrCancelled = fmt.Errorf("вход отменён: %w", context.Canceled)

func SignIn(ctx context.Context, stale string) (string, error) {
	if firefox, ok := defaultFirefox(); ok {
		if profiles := firefoxProfiles(firefoxRoots()); len(profiles) > 0 && openFirefox(firefox, signInURL) == nil {
			return waitForFirefox(ctx, profiles, stale)
		}
	}
	return signInWindow(ctx, stale)
}

func waitForFirefox(ctx context.Context, profiles []string, stale string) (string, error) {
	written := map[string]time.Time{}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		var changed []string
		for _, profile := range profiles {
			var stamp time.Time
			for _, name := range []string{"cookies.sqlite", "cookies.sqlite-wal"} {
				if info, err := os.Stat(filepath.Join(profile, name)); err == nil && info.ModTime().After(stamp) {
					stamp = info.ModTime()
				}
			}
			if !stamp.Equal(written[profile]) {
				written[profile] = stamp
				changed = append(changed, profile)
			}
		}
		if token := firefoxToken(changed); soundcloud.ValidToken(token) && token != stale {
			return token, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

func signInWindow(ctx context.Context, stale string) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, executable, Flag)
	cmd.Stdout = &stdout
	if stale != "" {

		cmd.Env = append(os.Environ(), staleVariable+"="+stale)
	}
	err = cmd.Run()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	token := strings.TrimSpace(stdout.String())
	if err == nil && soundcloud.ValidToken(token) {
		return token, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == exitCancelled {
		return "", ErrCancelled
	}
	return "", errors.New("не удалось открыть окно входа SoundCloud")
}

const exitCancelled = 3

const staleVariable = "SC_PLAYER_STALE_TOKEN"

const captureScript = `(function () {
	if (location.hostname !== "soundcloud.com") return;
	var last = "";
	setInterval(function () {
		var m = document.cookie.match(/(?:^|;\s*)oauth_token=([^;]+)/);
		if (m && m[1] !== last) { last = m[1]; window.scPlayerToken(decodeURIComponent(m[1])); }
	}, 500);
})();`

func startDetached(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
