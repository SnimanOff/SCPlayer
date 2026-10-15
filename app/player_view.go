package app

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"sc_player/api/soundcloud"
	"sc_player/player"
)

type PlayerView struct {
	a                             *Application
	root                          fyne.CanvasObject
	art                           *canvas.Image
	title, author, elapsed, total *widget.Label
	seek, volume                  *widget.Slider
	pause, previous, next         *widget.Button
	programmatic                  bool
	dragging                      bool
}

func newPlayerView(a *Application) *PlayerView {
	v := &PlayerView{a: a}
	v.art = canvas.NewImageFromResource(theme.MediaMusicIcon())
	v.art.FillMode = canvas.ImageFillContain
	v.art.SetMinSize(fyne.NewSize(120, 120))
	v.title = widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	v.title.Truncation = fyne.TextTruncateEllipsis
	v.author = widget.NewLabel("")
	v.author.Alignment = fyne.TextAlignCenter
	v.author.Truncation = fyne.TextTruncateEllipsis
	v.elapsed = widget.NewLabel("0:00")
	v.total = widget.NewLabel("0:00")
	v.seek = widget.NewSlider(0, 1)
	v.seek.Step = 0.1
	v.seek.OnChanged = func(float64) {
		if !v.programmatic {
			v.dragging = true
		}
	}

	v.seek.OnChangeEnded = func(seconds float64) {
		if v.programmatic {
			return
		}
		v.dragging = false
		a.action(func(ctx context.Context) error { return a.engine.Seek(ctx, seconds) })
	}
	v.pause = widget.NewButtonWithIcon("", theme.MediaPlayIcon(), a.togglePause)
	v.previous = widget.NewButtonWithIcon("", theme.MediaSkipPreviousIcon(), a.previous)
	v.next = widget.NewButtonWithIcon("", theme.MediaSkipNextIcon(), func() { a.move(1) })
	v.pause.Importance = widget.HighImportance
	v.volume = widget.NewSlider(0, 100)
	v.volume.Step = 1
	v.volume.SetValue(a.volume)

	v.volume.OnChanged = a.setVolume
	v.volume.OnChangeEnded = func(value float64) {
		a.savePreference(func(p fyne.Preferences) { p.SetFloat("volume", value) })
	}
	back := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), a.leavePlayer)
	back.Importance = widget.LowImportance
	transport := container.NewCenter(container.NewHBox(
		container.NewGridWrap(fyne.NewSize(56, 48), v.previous),
		container.NewGridWrap(fyne.NewSize(72, 48), v.pause),
		container.NewGridWrap(fyne.NewSize(56, 48), v.next)))
	controls := container.NewVBox(v.title, v.author,
		container.NewBorder(nil, nil, v.elapsed, v.total, v.seek),
		transport,
		container.NewBorder(nil, nil, widget.NewIcon(theme.VolumeDownIcon()), widget.NewIcon(theme.VolumeUpIcon()), v.volume))
	top := container.NewBorder(nil, nil, back, nil)
	v.root = container.NewBorder(top, controls, nil, nil, container.NewPadded(v.art))
	return v
}

func coverURL(track soundcloud.Track) string {
	raw := track.ArtworkURL
	if raw == "" {
		raw = track.User.AvatarURL
	}
	return strings.Replace(raw, "-large.", "-t500x500.", 1)
}

func (v *PlayerView) setTrack(track soundcloud.Track, ctx context.Context) {
	v.title.SetText(track.Title)
	v.author.SetText(track.Creator())
	v.art.Resource = theme.MediaMusicIcon()
	v.art.Image = nil
	v.art.Refresh()
	v.dragging = false
	v.seek.Disable()
	v.pause.Disable()
	v.previous.Disable()
	v.next.Disable()
	v.programmatic = true
	v.seek.SetValue(0)
	v.programmatic = false
	v.elapsed.SetText("0:00")
	v.total.SetText(duration(float64(track.Duration) / 1000))
	cover := coverURL(track)
	if cover == "" {
		return
	}
	go func() {
		resource := loadArtwork(ctx, cover)
		if resource != nil {
			v.a.ui(func() {
				if ctx.Err() == nil {
					v.art.Resource = resource
					v.art.Refresh()
				}
			})
		}
	}()
}
func (v *PlayerView) refresh(state player.State) {
	if v.a.playing.Key() == "" || v.a.loading {
		return
	}
	v.pause.Enable()
	if state.Paused || state.Idle {
		v.pause.SetIcon(theme.MediaPlayIcon())
	} else {
		v.pause.SetIcon(theme.MediaPauseIcon())
	}
	if v.a.queue.Index > 0 || state.Position > 3 {
		v.previous.Enable()
	} else {
		v.previous.Disable()
	}
	if v.a.queue.Index+1 < len(v.a.queue.Tracks) {
		v.next.Enable()
	} else {
		v.next.Disable()
	}
	total := state.Duration
	if total <= 0 {
		total = float64(v.a.playing.Duration) / 1000
	}
	if total > 0 && !state.Idle {
		v.seek.Enable()
	} else {
		v.seek.Disable()
	}
	if !v.dragging {
		v.programmatic = true
		v.seek.Max = max(total, 1)
		v.seek.SetValue(state.Position)
		v.programmatic = false
	}
	elapsed := duration(state.Position)
	if state.Buffering {
		elapsed += " …"
	}
	v.elapsed.SetText(elapsed)
	v.total.SetText(duration(total))
}

func loadArtwork(ctx context.Context, raw string) fyne.Resource {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return nil
	}
	info, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || info.Width > 4096 || info.Height > 4096 {
		return nil
	}
	return fyne.NewStaticResource("artwork", data)
}
