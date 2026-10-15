package app

import (
	"context"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"math"
	"sc_player/api/discord"
	"sc_player/api/soundcloud"
	"sc_player/player"
	"sync/atomic"
	"testing"
	"time"
)

type testEngine struct {
	closed bool
	state  player.State
	seeks  atomic.Int32

	volumes atomic.Int32
	volume  atomic.Uint64
}

func (*testEngine) Load(context.Context, string) error    { return nil }
func (*testEngine) Pause(context.Context, bool) error     { return nil }
func (e *testEngine) Seek(context.Context, float64) error { e.seeks.Add(1); return nil }
func (e *testEngine) SetVolume(_ context.Context, v float64) error {
	time.Sleep(time.Millisecond)
	e.volumes.Add(1)
	e.volume.Store(math.Float64bits(v))
	return nil
}
func (*testEngine) Stop(context.Context) error  { return nil }
func (e *testEngine) Snapshot() player.State    { return e.state }
func (*testEngine) Events() <-chan player.Event { return nil }
func (e *testEngine) Close() error              { e.closed = true; return nil }

type testPresence struct{ closed bool }

func (*testPresence) Update(soundcloud.Track, player.State) {}
func (*testPresence) Statuses() <-chan string               { return nil }
func (*testPresence) SetEnabled(bool)                       {}
func (*testPresence) Configured() bool                      { return true }
func (p *testPresence) Close()                              { p.closed = true }

var _ discord.Service = (*testPresence)(nil)

type testModule struct{ built bool }

func (*testModule) ID() string { return "replacement" }
func (m *testModule) Build(*Application) fyne.CanvasObject {
	m.built = true
	return widget.NewLabel("Replacement module")
}
func (*testModule) Refresh(*Application, context.Context) {}

func TestApplicationModulesThemeAndClose(t *testing.T) {
	gui := test.NewApp()
	defer gui.Quit()
	e, p, module := &testEngine{}, &testPresence{}, &testModule{}
	a := New(gui, nil, nil, e, p, module)
	if !module.built || len(a.modules) != 1 || a.modules[0].ID() != "replacement" {
		t.Fatal("modules cannot be replaced")
	}
	if a.body.Objects[0] != a.home || a.nowPlaying.Visible() {
		t.Fatal("unexpected initial screen")
	}
	a.toggleTheme()

	a.Close()
	a.Close()
	if gui.Preferences().BoolWithFallback("dark", true) {
		t.Fatal("theme preference was not saved")
	}
	if !e.closed || !p.closed {
		t.Fatal("close leaked services")
	}
}

func TestTrackListAndPlaybackControls(t *testing.T) {
	gui := test.NewApp()
	defer gui.Quit()
	a := New(gui, nil, nil, &testEngine{}, &testPresence{})
	defer a.Close()
	tracks := []soundcloud.Track{{ID: 1, Title: "First", Artist: "Artist", Duration: 90000}, {ID: 2, Title: "Second", Access: "preview"}}
	list := newTrackList(a)
	list.set(tracks)

	list.list.Resize(fyne.NewSize(340, 180))
	_ = test.WidgetRenderer(list.list)
	a.queue.Set(tracks, 0)
	a.playing = tracks[0]
	v := a.playerView
	v.setTrack(tracks[0], context.Background())
	v.refresh(player.State{Position: 10, Duration: 90})
	if v.title.Text != "First" || v.author.Text != "Artist" || v.elapsed.Text != "0:10" || v.total.Text != "1:30" || v.seek.Disabled() || v.next.Disabled() || v.previous.Disabled() {
		t.Fatal("playing track controls not populated")
	}
	if v.dragging {
		t.Fatal("programmatic progress update triggered seeking")
	}
	v.dragging = true
	v.setTrack(tracks[1], context.Background())
	if v.dragging {
		t.Fatal("new track inherited previous slider drag")
	}
	a.queue.Move(1)
	a.playing = tracks[1]
	v.refresh(player.State{Idle: true})
	if !v.next.Disabled() || !v.seek.Disabled() {
		t.Fatal("idle/end-of-queue controls enabled")
	}
}

func TestDurationFormatting(t *testing.T) {
	for input, want := range map[float64]string{-1: "0:00", 0: "0:00", 61.9: "1:01", 3661: "1:01:01"} {
		if got := duration(input); got != want {
			t.Errorf("duration(%v)=%s, want %s", input, got, want)
		}
	}
}

func TestMixesShowOnlyChosenShelves(t *testing.T) {
	m := NewMixesModule()
	for urn, want := range map[string]bool{
		"soundcloud:selections:personalized-tracks:1748362038": true,
		"soundcloud:selections:your-moods":                     true,
		"soundcloud:selections:personalized-albums":            true,
		"soundcloud:selections:recently-played:1748362038":     false,
		"soundcloud:selections:made-for-you":                   false,
		"soundcloud:selections:personalized-tracks-extra":      false,
		"": false,
	} {
		if m.wanted(urn) != want {
			t.Errorf("wanted(%q) != %v", urn, want)
		}
	}
}

func TestProgressUpdatesDoNotSeek(t *testing.T) {
	gui := test.NewApp()
	defer gui.Quit()
	e := &testEngine{}
	a := New(gui, nil, nil, e, &testPresence{})
	defer a.Close()
	tracks := []soundcloud.Track{{ID: 1, Title: "First", Duration: 90000}}
	a.queue.Set(tracks, 0)
	a.playing = tracks[0]
	v := a.playerView
	v.setTrack(tracks[0], context.Background())
	for second := 1.0; second <= 5; second++ {
		v.refresh(player.State{Position: second, Duration: 90})
	}
	time.Sleep(200 * time.Millisecond)
	if n := e.seeks.Load(); n != 0 {
		t.Fatalf("progress updates sent %d seeks", n)
	}
	v.seek.OnChangeEnded(30)
	time.Sleep(200 * time.Millisecond)
	if e.seeks.Load() != 1 {
		t.Fatal("user seek was ignored")
	}
}

func TestVolumeFollowsDragWithoutFlooding(t *testing.T) {
	gui := test.NewApp()
	defer gui.Quit()
	e := &testEngine{}
	a := New(gui, nil, nil, e, &testPresence{})
	defer a.Close()
	before := e.volumes.Load()
	for value := 1.0; value <= 100; value++ {
		a.playerView.volume.SetValue(value)
	}
	deadline := time.Now().Add(2 * time.Second)
	for math.Float64frombits(e.volume.Load()) != 100 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	sent := e.volumes.Load() - before
	if got := math.Float64frombits(e.volume.Load()); got != 100 {
		t.Fatalf("last volume lost: engine has %v", got)
	}
	if sent >= 100 || sent < 1 {
		t.Fatalf("%d volume commands for 100 slider steps", sent)
	}
	time.Sleep(50 * time.Millisecond)
	if e.volumes.Load()-before != sent {
		t.Fatal("volume commands kept coming after the drag")
	}
}

func TestPlayerReturnsToTrackList(t *testing.T) {
	gui := test.NewApp()
	defer gui.Quit()
	a := New(gui, nil, nil, &testEngine{}, &testPresence{})
	defer a.Close()
	a.showTracks("Поиск: test", soundcloud.Page[soundcloud.Track]{Collection: []soundcloud.Track{{ID: 1, Title: "One"}}}, nil)
	results := a.body.Objects[0]
	a.show(a.playerView.root)
	a.leavePlayer()
	if a.body.Objects[0] != results {
		t.Fatal("player did not return to the search results")
	}
	var back *widget.Button
	var find func(fyne.CanvasObject)
	find = func(o fyne.CanvasObject) {
		if b, ok := o.(*widget.Button); ok && b.Text == "Назад" {
			back = b
		}
		if c, ok := o.(*fyne.Container); ok {
			for _, child := range c.Objects {
				find(child)
			}
		}
	}
	find(results)
	if back == nil {
		t.Fatal("no back button on the list")
	}
	test.Tap(back)
	a.show(a.playerView.root)
	a.leavePlayer()
	if a.body.Objects[0] != a.home {
		t.Fatal("closed list reopened instead of home")
	}
}

type fakeTray struct{ menus int }

func (f *fakeTray) SetSystemTrayMenu(*fyne.Menu)  { f.menus++ }
func (*fakeTray) SetSystemTrayIcon(fyne.Resource) {}
func (*fakeTray) SetSystemTrayWindow(fyne.Window) {}

func TestTrayMenuFollowsPlayback(t *testing.T) {
	gui := test.NewApp()
	defer gui.Quit()
	e := &testEngine{state: player.State{Idle: true}}
	a := New(gui, nil, nil, e, &testPresence{})
	defer a.Close()
	tray := &fakeTray{}
	a.tray, a.trayMenu = tray, fyne.NewMenu("SC Player")
	labels := func() (out []string) {
		for _, item := range a.trayMenu.Items {
			if !item.IsSeparator {
				out = append(out, fmt.Sprintf("%s/%v", item.Label, item.Disabled))
			}
		}
		return out
	}
	a.refreshTray()
	a.refreshTray()
	if tray.menus != 1 || fmt.Sprint(labels()) != "[Показать плеер/false Играть/true Предыдущий трек/true Следующий трек/true Выход/false]" {
		t.Fatalf("idle tray: %d builds %v", tray.menus, labels())
	}
	if quit := a.trayMenu.Items[len(a.trayMenu.Items)-1]; !quit.IsQuit {
		t.Fatal("Fyne would add a second Quit that skips the shutdown")
	}
	tracks := []soundcloud.Track{{ID: 1, Title: "First", Artist: "Artist"}, {ID: 2, Title: "Second"}}
	a.queue.Set(tracks, 0)
	a.playing = tracks[0]
	e.state = player.State{Position: 5, Duration: 90}
	a.refreshTray()
	if tray.menus != 2 || fmt.Sprint(labels()) != "[First — Artist/true Показать плеер/false Пауза/false Предыдущий трек/false Следующий трек/false Выход/false]" {
		t.Fatalf("playing tray: %d builds %v", tray.menus, labels())
	}
}

func TestWindowHidesToTrayAndComesBack(t *testing.T) {
	gui := test.NewApp()
	defer gui.Quit()
	a := New(gui, nil, nil, &testEngine{}, &testPresence{})
	defer a.Close()
	a.Window.Show()
	a.hideToTray(false)
	if a.hidden != hiddenClosed {
		t.Fatal("closing did not hide to the tray")
	}
	a.ShowWindow()
	if a.hidden != windowShown {
		t.Fatal("tray did not bring the window back")
	}
}
