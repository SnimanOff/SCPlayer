package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"sc_player/api/soundcloud"
)

type trackList struct {
	tracks []soundcloud.Track
	list   *widget.List
}

func newTrackList(a *Application) *trackList {
	t := &trackList{}
	t.list = widget.NewList(func() int { return len(t.tracks) }, func() fyne.CanvasObject {
		title := widget.NewLabel("")
		title.Truncation = fyne.TextTruncateEllipsis
		author := widget.NewLabel("")
		author.Truncation = fyne.TextTruncateEllipsis
		author.TextStyle.Italic = true
		return container.NewBorder(nil, nil, nil, widget.NewLabel("0:00"), container.NewVBox(title, author))
	}, func(id widget.ListItemID, object fyne.CanvasObject) {
		track := t.tracks[id]
		row := object.(*fyne.Container)
		labels := row.Objects[0].(*fyne.Container)
		title := track.Title
		if track.Access == "preview" {
			title += " · фрагмент"
		}
		if track.Access == "blocked" {
			title += " · недоступен"
		}
		labels.Objects[0].(*widget.Label).SetText(title)
		labels.Objects[1].(*widget.Label).SetText(track.Creator())
		row.Objects[1].(*widget.Label).SetText(duration(float64(track.Duration) / 1000))
	})
	t.list.OnSelected = func(id widget.ListItemID) { t.list.UnselectAll(); a.Play(t.tracks, id) }
	return t
}
func (t *trackList) set(tracks []soundcloud.Track) {
	t.tracks = append([]soundcloud.Track(nil), tracks...)
	t.list.Refresh()
}

type SearchModule struct {
	entry  *widget.Entry
	button *widget.Button
	cancel context.CancelFunc
}

func NewSearchModule() *SearchModule { return &SearchModule{} }
func (*SearchModule) ID() string     { return "search" }
func (m *SearchModule) Build(a *Application) fyne.CanvasObject {
	m.entry = widget.NewEntry()
	m.entry.SetPlaceHolder("Найти трек в SoundCloud")
	search := func() {
		query := strings.TrimSpace(m.entry.Text)
		if query == "" {
			return
		}
		if m.cancel != nil {
			m.cancel()
		}
		ctx, cancel := context.WithCancel(a.session)
		m.cancel = cancel
		m.button.Disable()
		a.status.SetText("Поиск…")
		var page soundcloud.Page[soundcloud.Track]
		a.background(ctx, func(ctx context.Context) error {
			var err error
			page, err = a.Library.Search(ctx, query, "")
			return err
		}, func(err error) {
			m.button.Enable()
			if err != nil {
				a.message(err)
				return
			}
			a.status.SetText("")
			a.showTracks("Поиск: "+query, page, func(ctx context.Context, next string) (soundcloud.Page[soundcloud.Track], error) {
				return a.Library.Search(ctx, query, next)
			})
		})
	}
	m.button = widget.NewButtonWithIcon("", theme.SearchIcon(), search)
	m.entry.OnSubmitted = func(string) { search() }
	return container.NewVBox(a.Heading("Поиск"), container.NewBorder(nil, nil, nil, m.button, m.entry))
}
func (m *SearchModule) Refresh(a *Application, ctx context.Context) {
	m.entry.SetText("")
	m.button.Enable()
}

func (a *Application) showTracks(title string, page soundcloud.Page[soundcloud.Track], load func(context.Context, string) (soundcloud.Page[soundcloud.Track], error)) {
	if a.listCancel != nil {
		a.listCancel()
	}
	ctx, cancel := context.WithCancel(a.session)
	a.listCancel = cancel
	list := newTrackList(a)
	list.set(page.Collection)
	label := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	label.Truncation = fyne.TextTruncateEllipsis
	next := page.Next
	more := widget.NewButton("Ещё", nil)
	if next == "" {
		more.Hide()
	}
	more.OnTapped = func() {
		more.Disable()
		a.background(ctx, func(ctx context.Context) error { var err error; page, err = load(ctx, next); return err }, func(err error) {
			more.Enable()
			if err != nil {
				a.message(err)
				return
			}
			list.set(append(list.tracks, page.Collection...))
			next = page.Next
			if next == "" {
				more.Hide()
			}
		})
	}
	back := widget.NewButtonWithIcon("Назад", theme.NavigateBackIcon(), func() {
		cancel()
		a.lastList = nil
		a.show(a.home)
	})
	var body fyne.CanvasObject = list.list
	if len(page.Collection) == 0 {
		body = widget.NewLabel("Ничего не найдено")
	}

	a.lastList = container.NewBorder(container.NewVBox(back, label), more, nil, nil, body)
	a.show(a.lastList)
}

type PlaylistsModule struct {
	gen    int
	items  []soundcloud.Playlist
	list   *widget.List
	hint   *widget.Label
	more   *widget.Button
	next   string
	cancel context.CancelFunc
}

func NewPlaylistsModule() *PlaylistsModule { return &PlaylistsModule{} }
func (*PlaylistsModule) ID() string        { return "playlists" }
func (m *PlaylistsModule) Build(a *Application) fyne.CanvasObject {
	m.hint = widget.NewLabel("")
	m.hint.Wrapping = fyne.TextWrapWord
	m.list = widget.NewList(func() int { return len(m.items) }, func() fyne.CanvasObject {
		label := widget.NewLabel("")
		label.Truncation = fyne.TextTruncateEllipsis
		return label
	}, func(id widget.ListItemID, object fyne.CanvasObject) {
		p := m.items[id]
		object.(*widget.Label).SetText(fmt.Sprintf("%s · %d", p.Title, p.TrackCount))
	})
	m.list.OnSelected = func(id widget.ListItemID) { m.list.UnselectAll(); m.open(a, m.items[id]) }
	m.more = widget.NewButton("Ещё плейлисты", func() { m.load(a, a.session, false) })
	m.more.Hide()
	return container.NewVBox(a.Heading("Плейлисты"), m.hint, container.NewGridWrap(fyne.NewSize(340, 110), m.list), m.more)
}
func (m *PlaylistsModule) Refresh(a *Application, ctx context.Context) {
	m.gen++
	m.items = nil
	m.list.Refresh()
	m.next = ""
	m.load(a, ctx, true)
}
func (m *PlaylistsModule) load(a *Application, ctx context.Context, reset bool) {
	m.hint.SetText("Загрузка…")
	m.more.Disable()
	next, gen := m.next, m.gen
	var page soundcloud.Page[soundcloud.Playlist]
	a.background(ctx, func(ctx context.Context) error { var err error; page, err = a.Library.Playlists(ctx, next); return err }, func(err error) {
		if gen != m.gen {
			return
		}
		m.more.Enable()
		if err != nil {
			m.hint.SetText(err.Error())
			return
		}
		if reset {
			m.items = nil
		}
		m.items = append(m.items, page.Collection...)
		m.next = page.Next
		m.list.Refresh()
		m.hint.SetText("")
		if len(m.items) == 0 {
			m.hint.SetText("В SoundCloud пока нет плейлистов")
		}
		if m.next == "" {
			m.more.Hide()
		} else {
			m.more.Show()
		}
	})
}
func (m *PlaylistsModule) open(a *Application, playlist soundcloud.Playlist) {
	if m.cancel != nil {
		m.cancel()
	}
	ctx, cancel := context.WithCancel(a.session)
	m.cancel = cancel
	a.status.SetText("Загрузка плейлиста…")
	var tracks []soundcloud.Track
	a.background(ctx, func(ctx context.Context) error {
		next := ""
		seen := map[string]bool{}
		for {
			page, err := a.Library.PlaylistTracks(ctx, playlist.Key(), next)
			if err != nil {
				return err
			}
			tracks = append(tracks, page.Collection...)
			next = page.Next
			if next == "" {
				return nil
			}
			if seen[next] {
				return errors.New("SoundCloud повторил страницу плейлиста")
			}
			seen[next] = true
		}
	}, func(err error) {
		if err != nil {
			a.message(err)
			return
		}
		if len(tracks) == 0 {
			a.status.SetText("Плейлист пуст")
			return
		}
		a.Play(tracks, 0)
	})
}

type RecommendationsModule struct {
	gen                   int
	feed, related         *trackList
	feedHint, relatedHint *widget.Label
	refresh               *widget.Button
}

func NewRecommendationsModule() *RecommendationsModule { return &RecommendationsModule{} }
func (*RecommendationsModule) ID() string              { return "recommendations" }
func (m *RecommendationsModule) Build(a *Application) fyne.CanvasObject {
	m.feed = newTrackList(a)
	m.related = newTrackList(a)
	m.feedHint = widget.NewLabel("")
	m.relatedHint = widget.NewLabel("")
	m.feedHint.Wrapping = fyne.TextWrapWord
	m.relatedHint.Wrapping = fyne.TextWrapWord
	m.refresh = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() { m.Refresh(a, a.session) })
	tabs := container.NewAppTabs(container.NewTabItem("Лента", container.NewBorder(m.feedHint, nil, nil, nil, m.feed.list)), container.NewTabItem("Для вас", container.NewBorder(m.relatedHint, nil, nil, nil, m.related.list)))
	return container.NewVBox(container.NewBorder(nil, nil, a.Heading("Из SoundCloud"), m.refresh), container.NewGridWrap(fyne.NewSize(340, 220), tabs))
}
func (m *RecommendationsModule) Refresh(a *Application, ctx context.Context) {
	m.gen++
	gen := m.gen
	current := func() bool { return ctx.Err() == nil && gen == m.gen }
	m.refresh.Disable()
	m.feed.set(nil)
	m.related.set(nil)
	m.feedHint.SetText("Загрузка…")
	m.relatedHint.SetText("Загрузка…")
	a.background(ctx, func(ctx context.Context) error {
		feed, feedErr := a.Library.Feed(ctx)
		a.ui(func() {
			if !current() {
				return
			}
			m.feed.set(feed)
			m.feedHint.SetText("")
			if feedErr != nil {
				m.feedHint.SetText(feedErr.Error())
			} else if len(feed) == 0 {
				m.feedHint.SetText("Ваша лента пока пуста")
			}
		})
		likes, likesErr := a.Library.Likes(ctx)
		var seed soundcloud.Track
		if len(likes.Collection) > 0 {
			seed = likes.Collection[0]
		} else if len(feed) > 0 {
			seed = feed[0]
		}
		if seed.Key() == "" {
			if likesErr != nil {
				return likesErr
			}
			a.ui(func() {
				if current() {
					m.relatedHint.SetText("Отметьте любимые треки в SoundCloud — здесь появятся похожие")
				}
			})
			return nil
		}
		related, err := a.Library.Related(ctx, seed.Key())
		if err != nil {
			return err
		}
		a.ui(func() {
			if current() {
				m.related.set(related.Collection)
				m.relatedHint.SetText("По мотивам: " + seed.Title)
				if len(related.Collection) == 0 {
					m.relatedHint.SetText("SoundCloud пока не предложил похожие треки")
				}
			}
		})
		return nil
	}, func(err error) {
		if gen != m.gen {
			return
		}
		m.refresh.Enable()
		if err != nil {
			m.relatedHint.SetText(err.Error())
		}
	})
}
