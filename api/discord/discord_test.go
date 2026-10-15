package discord

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"sc_player/api/soundcloud"
	"sc_player/player"
)

func TestFramesAndSizeLimit(t *testing.T) {
	var buffer bytes.Buffer
	if err := writeFrame(&buffer, 1, map[string]string{"title": "Музыка"}); err != nil {
		t.Fatal(err)
	}
	op, data, err := readFrame(&buffer)
	if err != nil || op != 1 || !strings.Contains(string(data), "Музыка") {
		t.Fatalf("frame: %d %s %v", op, data, err)
	}
	var header [8]byte
	binary.LittleEndian.PutUint32(header[4:], 1<<20+1)
	if _, _, err := readFrame(bytes.NewReader(header[:])); err == nil {
		t.Fatal("oversized frame accepted")
	}
	if _, _, err := readFrame(strings.NewReader("short")); err == nil {
		t.Fatal("truncated frame accepted")
	}
}

func TestPresencePlayingAndPaused(t *testing.T) {
	u := Update{Track: soundcloud.Track{ID: 1, Title: strings.Repeat("я", 140), Artist: "Artist"}, State: player.State{Position: 30, Duration: 120}}
	a := activity(u)
	if len([]rune(a["details"].(string))) != 128 || a["state"] != "Artist" || a["timestamps"] == nil || a["status_display_type"] != 2 {
		t.Fatalf("bad activity: %+v", a)
	}
	u.At = time.Now().Add(-14 * time.Second)
	if start := activity(u)["timestamps"].(map[string]int64)["start"]; start != u.At.Unix()-30 {
		t.Fatalf("resend moved start time: %d", start)
	}
	u.State.Paused = true
	if a = activity(u); a["timestamps"] != nil || !strings.HasPrefix(a["state"].(string), "Пауза") {
		t.Fatalf("bad pause activity: %+v", a)
	}
}

func TestDiscordWithoutConfigurationClosesPromptly(t *testing.T) {
	defer func(id string) { DefaultClientID = id }(DefaultClientID)
	DefaultClientID = ""
	p := New("", true)
	select {
	case status := <-p.Statuses():
		if !strings.Contains(status, "не настроен") {
			t.Fatalf("unexpected status %q", status)
		}
	case <-time.After(time.Second):
		t.Fatal("no configuration status")
	}
	p.SetEnabled(false)
	p.Close()
}

func TestPresenceCanBeDisabledAtRuntime(t *testing.T) {

	missing := filepath.Join(t.TempDir(), "discord-ipc-0")
	if runtime.GOOS == "windows" {
		missing = `\\.\pipe\sc-player-test-` + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	defer func(saved func() []string) { pipePaths = saved }(pipePaths)
	pipePaths = func() []string { return []string{missing} }
	p := New("123", false)
	defer p.Close()
	wait := func(want string) {
		deadline := time.After(3 * time.Second)
		for {
			select {
			case status := <-p.Statuses():
				if strings.Contains(status, want) {
					return
				}
			case <-deadline:
				t.Fatalf("status %q not reported", want)
			}
		}
	}
	wait("выключен")
	p.SetEnabled(true)
	wait("ожидание")
	p.SetEnabled(false)
	wait("выключен")
}

func TestActivityFieldsFitDiscordLimits(t *testing.T) {
	units := func(s string) int { return len(utf16.Encode([]rune(s))) }
	long := field(strings.Repeat("🎵", 100))
	if units(long) > 128 || !utf8.ValidString(long) {
		t.Fatalf("emoji title exceeds 128 units: %d", units(long))
	}
	if short := field(" Ю "); units(short) != 2 || !strings.HasPrefix(short, "Ю") {
		t.Fatalf("one-letter title not padded: %q", short)
	}
	if field("   ") != "" {
		t.Fatal("blank field kept")
	}
	a := activity(Update{Track: soundcloud.Track{ID: 1, Title: "Ю", User: soundcloud.User{AvatarURL: "https://i1.sndcdn.com/avatars-x-large.jpg"}}, State: player.State{Paused: true}})
	if units(a["details"].(string)) != 2 || a["state"] != "Пауза ·" {
		t.Fatalf("short or missing fields: %+v", a)
	}
	if a["assets"].(map[string]string)["large_image"] != "https://i1.sndcdn.com/avatars-x-t300x300.jpg" {
		t.Fatalf("no avatar fallback: %+v", a["assets"])
	}
}

func TestRefusedActivityKeepsDiscordsReason(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go func() {
		defer server.Close()
		_, data, err := readFrame(server)
		if err != nil {
			return
		}
		var request struct {
			Nonce string `json:"nonce"`
		}
		_ = json.Unmarshal(data, &request)
		_ = writeFrame(server, 1, map[string]any{"cmd": "SET_ACTIVITY", "evt": "ERROR", "nonce": request.Nonce,
			"data": map[string]any{"code": 4000, "message": "details length must be at least 2 characters long"}})
	}()
	err := setActivity(client, map[string]any{"details": "x"})
	var refused rejected
	if !errors.As(err, &refused) || !strings.Contains(refused.message, "at least 2") {
		t.Fatalf("refusal lost: %v", err)
	}
	if got := refusal([]byte(`{"code":4000,"message":"Invalid Client ID"}`)); got.message != "Invalid Client ID" {
		t.Fatalf("handshake refusal: %q", got.message)
	}
}
