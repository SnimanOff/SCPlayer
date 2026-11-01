package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"sc_player/api/discord"
	"sc_player/api/soundcloud"
	"sc_player/internal/obs"
	"sc_player/player"
)

type Library interface {
	Me(context.Context) (soundcloud.User, error)
	Search(context.Context, string, string) (soundcloud.Page[soundcloud.Track], error)
	Playlists(context.Context, string) (soundcloud.Page[soundcloud.Playlist], error)
	PlaylistTracks(context.Context, string, string) (soundcloud.Page[soundcloud.Track], error)
	Likes(context.Context) (soundcloud.Page[soundcloud.Track], error)
	Related(context.Context, string) (soundcloud.Page[soundcloud.Track], error)
	Feed(context.Context) ([]soundcloud.Track, error)
	Stream(context.Context, soundcloud.Track) (string, error)
	Mixes(context.Context) ([]soundcloud.Selection, error)
	MixTracks(context.Context, soundcloud.Mix) ([]soundcloud.Track, error)
}

type Module interface {
	ID() string
	Build(*Application) fyne.CanvasObject
	Refresh(*Application, context.Context)
}

type Application struct {
	GUI           fyne.App
	Window        fyne.Window
	Library       Library
	sc            *soundcloud.Session
	engine        player.Engine
	presence      discord.Service
	modules       []Module
	ctx           context.Context
	cancel        context.CancelFunc
	session       context.Context
	sessionCancel context.CancelFunc
	playCancel    context.CancelFunc
	playSerial    sync.Mutex
	closed        atomic.Bool
	queue         player.Queue
	playing       soundcloud.Track
	loading       bool
	playReady     bool
	playVersion   uint64
	loggedIn      bool
	loggingIn     bool
	loginCancel   context.CancelFunc
	loginVersion  uint64
	loginProgress *widget.ProgressBarInfinite
	nowPlaying    *widget.Button
	status        *widget.Label
	account       *widget.Button
	body          *fyne.Container
	home          fyne.CanvasObject
	playerView    *PlayerView
	lastList      fyne.CanvasObject
	listCancel    context.CancelFunc
	discordStatus *widget.Label
	discordToggle *widget.Check
	obs           *obs.Server
	obsEnabled    bool
	obsError      error
	accents       []*canvas.Rectangle
	volume        float64
	volumeTarget  atomic.Uint64
	volumeBusy    atomic.Bool
	dark          bool
	prefs         chan func(fyne.Preferences)
	prefsDone     chan struct{}
	tray          desktop.App
	trayMenu      *fyne.Menu
	trayKey       string
	hidden        windowState

	SignIn func(context.Context, string) (string, error)
}

func New(gui fyne.App, sc *soundcloud.Session, library Library, engine player.Engine, presence discord.Service, modules ...Module) *Application {
	ctx, cancel := context.WithCancel(context.Background())
	a := &Application{GUI: gui, sc: sc, Library: library, engine: engine, presence: presence, ctx: ctx, cancel: cancel, volume: gui.Preferences().FloatWithFallback("volume", 80)}
	a.obs = obs.New(obs.DefaultAddress)
	a.session, a.sessionCancel = context.WithCancel(ctx)

	_ = engine.SetVolume(ctx, a.volume)
	if len(modules) == 0 {
		modules = []Module{NewSearchModule(), NewMixesModule(), NewPlaylistsModule(), NewRecommendationsModule()}
	}
	a.modules = modules
	a.Window = gui.NewWindow("SC Player")
	a.Window.Resize(fyne.NewSize(390, 720))
	a.dark = gui.Preferences().BoolWithFallback("dark", true)
	a.prefs, a.prefsDone = make(chan func(fyne.Preferences), 32), make(chan struct{})
	go a.savePreferences()
	gui.Settings().SetTheme(newAngularTheme(a.dark))
	a.status = widget.NewLabel("")
	a.status.Wrapping = fyne.TextWrapWord
	a.discordStatus = widget.NewLabel("Discord: подключение…")
	a.discordStatus.Truncation = fyne.TextTruncateEllipsis

	a.discordToggle = widget.NewCheck("", nil)
	a.discordToggle.SetChecked(gui.Preferences().BoolWithFallback("discord", true))
	a.discordToggle.OnChanged = func(on bool) {
		a.presence.SetEnabled(on)
		a.savePreference(func(p fyne.Preferences) { p.SetBool("discord", on) })
	}
	if !a.presence.Configured() {
		a.discordToggle.Disable()
	}
	a.account = widget.NewButton("Войти", a.login)
	themeButton := widget.NewButtonWithIcon("", theme.ColorPaletteIcon(), a.toggleTheme)
	logo := container.NewBorder(nil, nil, a.accent(14, 14), nil, widget.NewLabelWithStyle("SC PLAYER", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	header := container.NewVBox(container.NewBorder(nil, nil, logo, container.NewHBox(themeButton, a.account)), widget.NewSeparator())
	a.body = container.NewStack()
	a.playerView = newPlayerView(a)
	a.nowPlaying = widget.NewButtonWithIcon("К плееру", theme.MediaMusicIcon(), func() { a.show(a.playerView.root) })
	a.nowPlaying.Hide()
	content := container.NewVBox()
	for i, module := range modules {
		if i > 0 {
			content.Add(widget.NewSeparator())
		}
		content.Add(module.Build(a))
	}
	content.Add(widget.NewHyperlink("Музыка из SoundCloud", mustURL("https://soundcloud.com")))
	a.home = container.NewVScroll(content)

	a.body.Objects = []fyne.CanvasObject{a.home}
	obsButton := widget.NewButton("OBS", a.showOBS)
	discordRow := container.NewBorder(nil, nil, a.discordToggle, obsButton, a.discordStatus)
	footer := container.NewVBox(a.nowPlaying, a.status, widget.NewSeparator(), discordRow)
	a.Window.SetContent(container.NewBorder(header, footer, nil, nil, a.body))

	a.Window.SetCloseIntercept(func() {
		if a.tray != nil {
			a.hideToTray(false)
			return
		}
		a.quit()
	})
	if gui.Preferences().BoolWithFallback("obs", false) {
		a.message(a.setOBSEnabled(true))
	}
	return a
}

func (a *Application) Run() {
	go a.poll()
	a.restore()

	a.setupTray()
	a.GUI.Lifecycle().SetOnStarted(func() {
		a.paintTitleBar()
		a.GUI.Settings().AddListener(func(fyne.Settings) { fyne.Do(a.paintTitleBar) })
	})
	a.Window.ShowAndRun()
}

func (a *Application) paintTitleBar() {
	t := newAngularTheme(a.dark)
	paintTitleBar(a.Window, a.dark, t.Color(theme.ColorNameBackground, 0), t.Color(theme.ColorNameForeground, 0), t.Color(theme.ColorNameSeparator, 0))
}
func (a *Application) Close() {
	if a.beginClose() {
		a.shutdown()
	}
}

func (a *Application) beginClose() bool {
	if a.closed.Swap(true) {
		return false
	}
	a.sessionCancel()
	if a.playCancel != nil {
		a.playCancel()
	}
	return true
}

func (a *Application) shutdown() {
	_ = a.obs.Close()
	_ = a.engine.Close()
	a.presence.Close()
	a.cancel()
	<-a.prefsDone
}

func (a *Application) savePreference(write func(fyne.Preferences)) {
	select {
	case a.prefs <- write:
	case <-a.ctx.Done():
	}
}

func (a *Application) savePreferences() {
	defer close(a.prefsDone)
	for {
		select {
		case write := <-a.prefs:
			write(a.GUI.Preferences())
		case <-a.ctx.Done():
			for {
				select {
				case write := <-a.prefs:
					write(a.GUI.Preferences())
				default:
					return
				}
			}
		}
	}
}
func (a *Application) ui(fn func()) {
	if a.closed.Load() {
		return
	}
	fyne.Do(func() {
		if !a.closed.Load() {
			fn()
		}
	})
}
func (a *Application) background(ctx context.Context, work func(context.Context) error, done func(error)) {
	go func() {
		err := work(ctx)
		a.ui(func() {
			if ctx.Err() == nil {
				done(err)
			}
		})
	}()
}
func (a *Application) message(err error) {
	if err != nil && !errors.Is(err, context.Canceled) {
		a.status.SetText(err.Error())
	}
}
func (a *Application) show(object fyne.CanvasObject) {
	a.body.Objects = []fyne.CanvasObject{object}
	a.body.Refresh()

	if a.playing.Key() != "" && object != a.playerView.root {
		a.nowPlaying.Show()
	} else {
		a.nowPlaying.Hide()
	}
}
func (a *Application) toggleTheme() {
	a.dark = !a.dark
	dark := a.dark
	a.savePreference(func(p fyne.Preferences) { p.SetBool("dark", dark) })
	a.GUI.Settings().SetTheme(newAngularTheme(dark))
	a.paintAccents()
}

func (a *Application) restore() {
	a.show(a.home)
	if a.sc != nil && a.sc.LoggedIn() {
		a.connected()
	} else {
		a.signedOut()
	}
}
func (a *Application) login() {
	if a.loggingIn {
		return
	}
	if a.loggedIn {
		a.logout()
		return
	}
	if a.SignIn == nil || a.sc == nil {
		return
	}
	a.account.Disable()
	a.loggingIn = true
	a.status.SetText("")
	a.loginVersion++
	version := a.loginVersion
	ctx, cancel := context.WithTimeout(a.session, 10*time.Minute)
	a.loginCancel = cancel
	a.show(a.newPendingLoginView())
	stale := a.sc.Stale()
	go func() {
		token, err := a.SignIn(ctx, stale)
		if err == nil {
			err = a.sc.SetToken(token)
		}
		cancel()
		a.ui(func() { a.finishLogin(version, err) })
	}()
}

func (a *Application) finishLogin(version uint64, err error) {
	if version != a.loginVersion {
		return
	}
	a.stopLoginProgress()
	a.loggingIn = false
	a.loginCancel = nil
	a.account.Enable()
	if err != nil {
		a.show(a.home)
		if errors.Is(err, context.DeadlineExceeded) {
			a.status.SetText("Время входа истекло. Нажмите «Войти», чтобы попробовать ещё раз.")
		} else {
			a.message(err)
		}
		return
	}
	a.Window.RequestFocus()
	a.show(a.home)
	a.connected()
}

func (a *Application) cancelLogin() {
	if a.loginCancel != nil {
		a.loginCancel()
		a.loginCancel = nil
	}
	a.loginVersion++
	a.stopLoginProgress()
	a.loggingIn = false
	a.account.Enable()
	a.status.SetText("")
	a.show(a.home)
}

func (a *Application) stopLoginProgress() {
	if a.loginProgress != nil {
		a.loginProgress.Stop()
		a.loginProgress = nil
	}
}
func (a *Application) refreshModules() {
	for _, module := range a.modules {
		module.Refresh(a, a.session)
	}
}
func (a *Application) connected() {
	a.loggedIn = true
	a.account.Enable()
	a.account.SetText("Выйти")
	a.status.SetText("")
	a.refreshModules()
	a.background(a.session, func(ctx context.Context) error {
		user, err := a.Library.Me(ctx)
		if err == nil {
			a.ui(func() {
				if ctx.Err() == nil {
					a.Window.SetTitle("SC Player · " + user.Username)
				}
			})
		}
		return err
	}, func(err error) {
		if errors.Is(err, soundcloud.ErrLoginRequired) {
			a.signedOut()
		}
		a.message(err)
	})
}

func (a *Application) signedOut() {
	a.loggedIn = false
	a.account.SetText("Войти")
	a.Window.SetTitle("SC Player")
	a.refreshModules()
}
func (a *Application) logout() {
	a.sessionCancel()
	if a.playCancel != nil {
		a.playCancel()
	}
	a.playVersion++
	a.session, a.sessionCancel = context.WithCancel(a.ctx)
	a.loggedIn = false
	a.nowPlaying.Hide()
	a.loading = false
	a.playReady = false
	a.queue = player.Queue{}
	a.playing = soundcloud.Track{}
	a.updateOBS()
	a.lastList = nil
	a.account.Disable()
	a.show(a.home)
	a.status.SetText("")
	a.presence.Update(soundcloud.Track{}, player.State{Idle: true})
	a.refreshTray()
	a.background(a.ctx, func(ctx context.Context) error {
		a.playSerial.Lock()
		defer a.playSerial.Unlock()
		_ = a.engine.Stop(ctx)
		return a.sc.Logout()
	}, func(err error) {
		a.account.Enable()
		a.message(err)
		a.signedOut()
	})
}

func (a *Application) Play(tracks []soundcloud.Track, index int) {
	if !a.queue.Set(tracks, index) {
		return
	}
	a.playCurrent()
}
func (a *Application) playCurrent() {
	track, ok := a.queue.Current()
	if !ok {
		return
	}
	if a.playCancel != nil {
		a.playCancel()
	}
	ctx, cancel := context.WithCancel(a.session)
	a.playCancel = cancel
	a.playVersion++
	version := a.playVersion
	a.loading = true
	a.playReady = false
	a.playing = track
	a.updateOBS()
	a.status.SetText("Загрузка трека…")
	a.playerView.setTrack(track, ctx)
	a.show(a.playerView.root)
	a.refreshTray()
	a.background(ctx, func(ctx context.Context) error {
		a.playSerial.Lock()
		defer a.playSerial.Unlock()
		if err := ctx.Err(); err != nil {
			return err
		}
		_ = a.engine.Stop(ctx)
		stream, err := a.Library.Stream(ctx, track)
		if err != nil {
			return err
		}

		return a.engine.Load(ctx, stream)
	}, func(err error) {
		if version != a.playVersion {
			return
		}
		a.loading = false
		a.playReady = err == nil
		a.updateOBS()
		if err != nil {
			a.message(err)
			return
		}
		a.status.SetText("")
		if track.Access == "preview" {
			a.status.SetText("SoundCloud разрешает только фрагмент трека")
		}
	})
}
func (a *Application) move(delta int) {
	if a.queue.Move(delta) {
		a.playCurrent()
	}
}
func (a *Application) action(work func(context.Context) error) {

	a.background(a.session, work, a.message)
}
func (a *Application) updatePresence() { a.presence.Update(a.playing, a.engine.Snapshot()) }
func (a *Application) poll() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	presenceTicker := time.NewTicker(15 * time.Second)
	defer presenceTicker.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.ui(func() {
				a.playerView.refresh(a.engine.Snapshot())
				a.updateOBS()

				if a.tray != nil && a.hidden == windowShown && windowMinimized(a.Window) {
					a.hideToTray(true)
				}
			})
		case <-presenceTicker.C:
			a.ui(a.updatePresence)
		case status := <-a.presence.Statuses():
			a.ui(func() { a.discordStatus.SetText(status) })
		case event := <-a.engine.Events():
			a.ui(func() {
				a.updateOBS()
				if event.Err != nil {
					a.message(event.Err)
				}
				if event.Changed {
					a.updatePresence()
					a.refreshTray()
				}
				if event.Ended && !a.loading {
					if a.queue.Move(1) {
						a.playCurrent()
					} else {
						a.status.SetText("Очередь закончилась")
						a.updatePresence()
						a.refreshTray()
					}
				}
			})
		}
	}
}
func mustURL(raw string) *url.URL { u, _ := url.Parse(raw); return u }
func duration(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	n := int(seconds)
	if n >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", n/3600, (n/60)%60, n%60)
	}
	return fmt.Sprintf("%d:%02d", n/60, n%60)
}

func (a *Application) setVolume(value float64) {
	a.volume = value
	a.volumeTarget.Store(math.Float64bits(value))
	if a.volumeBusy.Swap(true) {
		return
	}
	go func() {
		for {
			want := math.Float64frombits(a.volumeTarget.Load())
			if err := a.engine.SetVolume(a.ctx, want); err != nil {
				a.ui(func() { a.message(err) })
			}
			a.volumeBusy.Store(false)

			if math.Float64frombits(a.volumeTarget.Load()) == want || a.volumeBusy.Swap(true) {
				return
			}
		}
	}()
}

func (a *Application) leavePlayer() {
	if a.lastList != nil {
		a.show(a.lastList)
		return
	}
	a.show(a.home)
}

func (a *Application) togglePause() {
	state := a.engine.Snapshot()
	if state.Idle {
		a.playCurrent()
		return
	}
	a.action(func(ctx context.Context) error { return a.engine.Pause(ctx, !state.Paused) })
}

func (a *Application) previous() {
	if a.engine.Snapshot().Position > 3 {
		a.action(func(ctx context.Context) error { return a.engine.Seek(ctx, 0) })
		return
	}
	a.move(-1)
}
