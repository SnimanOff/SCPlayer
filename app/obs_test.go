package app

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"sc_player/api/soundcloud"
	"sc_player/internal/obs"
	"sc_player/player"
)

func obsControls(a *Application) (*widget.Check, *widget.Button) {
	content := a.obsSettings().(*fyne.Container)
	return content.Objects[0].(*widget.Check), content.Objects[2].(*widget.Button)
}

func TestOBSControlsPlaybackAndShutdown(t *testing.T) {
	gui := test.NewApp()
	defer gui.Quit()
	e := &testEngine{state: player.State{Paused: true}}
	a := New(gui, nil, nil, e, &testPresence{}, &testModule{})
	defer a.Close()
	a.obs = obs.New("127.0.0.1:0")
	enabled, copyURL := obsControls(a)
	if enabled.Checked || !copyURL.Disabled() {
		t.Fatal("OBS should be opt-in")
	}
	enabled.SetChecked(true)
	if !enabled.Checked || copyURL.Disabled() || a.obsError != nil {
		t.Fatalf("OBS could not be enabled: %v", a.obsError)
	}
	test.Tap(copyURL)
	if got := gui.Clipboard().Content(); got != a.obs.URL() {
		t.Fatalf("clipboard URL = %q, want %q", got, a.obs.URL())
	}
	client := &http.Client{Timeout: time.Second}
	endpoint := strings.TrimSuffix(a.obs.URL(), "/obs") + "/api/now-playing"
	read := func() (state struct {
		Active bool
		Title  string
		Paused bool
	}) {
		t.Helper()
		response, err := client.Get(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
			t.Fatal(err)
		}
		return
	}
	a.playing = soundcloud.Track{ID: 1, Title: "Трек 🎵"}
	a.playReady = true
	a.updateOBS()
	if state := read(); !state.Active || state.Title != a.playing.Title || !state.Paused {
		t.Fatalf("paused track missing: %+v", state)
	}
	// While switching tracks, the engine can still report the previous audio.
	a.playing = soundcloud.Track{ID: 2, Title: "Следующий"}
	a.loading = true
	a.updateOBS()
	if read().Active {
		t.Fatal("loading track was published over previous audio")
	}
	a.loading, a.playReady = false, false
	a.updateOBS()
	if read().Active {
		t.Fatal("failed load became visible")
	}
	a.playReady = true
	e.state = player.State{Idle: true}
	a.updateOBS()
	if read().Active {
		t.Fatal("idle player retained the card")
	}
	enabled.SetChecked(false)
	if enabled.Checked || !copyURL.Disabled() || e.closed {
		t.Fatal("disabling OBS should only stop the widget")
	}
	enabled.SetChecked(true)
	endpoint = strings.TrimSuffix(a.obs.URL(), "/obs") + "/api/now-playing"
	a.Close()
	if !gui.Preferences().Bool("obs") {
		t.Fatal("enabled preference was not saved")
	}
	if response, err := client.Get(endpoint); err == nil {
		response.Body.Close()
		t.Fatal("OBS server survived application shutdown")
	}
}

func TestOBSBusyPortRollsBackSwitch(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	gui := test.NewApp()
	defer gui.Quit()
	e := &testEngine{}
	a := New(gui, nil, nil, e, &testPresence{}, &testModule{})
	defer a.Close()
	a.obs = obs.New(listener.Addr().String())
	enabled, copyURL := obsControls(a)
	enabled.SetChecked(true)
	if enabled.Checked || a.obsEnabled || a.obsError == nil || !copyURL.Disabled() {
		t.Fatal("busy port left the widget marked as enabled")
	}
	if e.closed {
		t.Fatal("widget error stopped the player")
	}
	a.Close()
	if gui.Preferences().Bool("obs") {
		t.Fatal("failed enable was saved as enabled")
	}
}
