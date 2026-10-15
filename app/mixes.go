package app

import (
	"context"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"sc_player/api/soundcloud"
)

type MixesModule struct {
	Shelves    []string
	hint       *widget.Label
	shelves    *fyne.Container
	cancel     context.CancelFunc
	openCancel context.CancelFunc
}

func NewMixesModule() *MixesModule {
	return &MixesModule{Shelves: []string{
		"soundcloud:selections:personalized-tracks",
		"soundcloud:selections:your-moods",
		"soundcloud:selections:personalized-albums",
	}}
}
func (*MixesModule) ID() string { return "mixes" }
func (m *MixesModule) Build(a *Application) fyne.CanvasObject {
	m.hint = wrapped("")
	m.shelves = container.NewVBox()
	return container.NewVBox(a.Heading("Подборки SoundCloud"), m.hint, m.shelves)
}

func (m *MixesModule) Refresh(a *Application, ctx context.Context) {
	if m.cancel != nil {
		m.cancel()
	}
	ctx, m.cancel = context.WithCancel(ctx)
	m.shelves.RemoveAll()
	m.hint.SetText("Загрузка…")
	m.hint.Show()
	var shelves []soundcloud.Selection
	a.background(ctx, func(ctx context.Context) error {
		all, err := a.Library.Mixes(ctx)
		for _, shelf := range all {
			if m.wanted(shelf.URN) {
				shelves = append(shelves, shelf)
			}
		}
		return err
	}, func(err error) {
		if err != nil {
			m.hint.SetText(err.Error())
			return
		}
		if len(shelves) == 0 {
			m.hint.SetText("SoundCloud пока ничего не подобрал")
			if !a.loggedIn {
				m.hint.SetText("Войдите в SoundCloud, чтобы увидеть свои подборки")
			}
			return
		}
		m.hint.Hide()
		artwork := make(chan struct{}, 4)
		for _, shelf := range shelves {
			cards := container.NewHBox()
			for _, mix := range shelf.Mixes {
				cards.Add(m.card(a, ctx, mix, artwork))
			}
			title := widget.NewLabelWithStyle(shelf.Title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			title.Truncation = fyne.TextTruncateEllipsis
			m.shelves.Add(container.NewVBox(title, container.NewHScroll(cards)))
		}
		a.home.Refresh()
	})
}

const coverSize = 104

func (m *MixesModule) card(a *Application, ctx context.Context, mix soundcloud.Mix, artwork chan struct{}) fyne.CanvasObject {
	placeholder := canvas.NewRectangle(color.NRGBA{R: 128, G: 128, B: 128, A: 60})
	cover := canvas.NewImageFromResource(nil)
	cover.FillMode = canvas.ImageFillContain
	art := container.NewGridWrap(fyne.NewSize(coverSize, coverSize), container.NewStack(placeholder, cover))
	title := widget.NewLabel(mix.Title)
	title.Truncation = fyne.TextTruncateEllipsis
	caption := container.NewGridWrap(fyne.NewSize(coverSize, title.MinSize().Height), title)
	open := widget.NewButton("", func() { m.open(a, mix) })
	open.Importance = widget.LowImportance
	if mix.ArtworkURL != "" {
		go func() {
			select {
			case artwork <- struct{}{}:
			case <-ctx.Done():
				return
			}
			resource := loadArtwork(ctx, mix.ArtworkURL)
			<-artwork
			if resource != nil {
				a.ui(func() {
					if ctx.Err() == nil {
						cover.Resource = resource
						cover.Refresh()
					}
				})
			}
		}()
	}
	return container.NewStack(open, container.NewVBox(art, caption))
}

func (m *MixesModule) open(a *Application, mix soundcloud.Mix) {
	if m.openCancel != nil {
		m.openCancel()
	}
	ctx, cancel := context.WithCancel(a.session)
	m.openCancel = cancel
	a.status.SetText("Загрузка подборки…")
	var tracks []soundcloud.Track
	a.background(ctx, func(ctx context.Context) error {
		var err error
		tracks, err = a.Library.MixTracks(ctx, mix)
		return err
	}, func(err error) {
		if err != nil {
			a.message(err)
			return
		}
		if len(tracks) == 0 {
			a.status.SetText("Подборка пуста")
			return
		}
		a.Play(tracks, 0)
	})
}

func (m *MixesModule) wanted(urn string) bool {
	for _, prefix := range m.Shelves {
		if urn == prefix || strings.HasPrefix(urn, prefix+":") {
			return true
		}
	}
	return false
}
