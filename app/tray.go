package app

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/systray"
)

type windowState int

const (
	windowShown windowState = iota
	hiddenClosed
	hiddenMinimized
)

func (a *Application) setupTray() {
	desk, ok := a.GUI.(desktop.App)
	if !ok || !trayAvailable() {
		return
	}
	a.tray = desk
	a.trayMenu = fyne.NewMenu("SC Player")
	a.refreshTray()

	systray.SetOnTapped(func() { fyne.Do(a.ShowWindow) })
}

func (a *Application) hideToTray(minimized bool) {
	if minimized {
		hideMinimized(a.Window)
		a.hidden = hiddenMinimized
		return
	}
	a.Window.Hide()
	a.hidden = hiddenClosed
}

func (a *Application) ShowWindow() {
	if a.hidden == hiddenClosed {
		a.Window.Show()
	}
	a.hidden = windowShown
	restoreWindow(a.Window)
	a.Window.RequestFocus()
}

func (a *Application) quit() {
	a.Window.Hide()
	if a.beginClose() {
		go func() {
			a.shutdown()
			fyne.Do(a.GUI.Quit)
		}()
	}
}

func (a *Application) trayItems() []*fyne.MenuItem {
	state := a.engine.Snapshot()
	loaded := a.playing.Key() != ""
	var items []*fyne.MenuItem
	if loaded {
		now := fyne.NewMenuItem(trayTitle(a.playing.Title+" — "+a.playing.Creator()), nil)
		now.Disabled = true
		items = append(items, now, fyne.NewMenuItemSeparator())
	}
	label := "Пауза"
	if !loaded || state.Paused || state.Idle {
		label = "Играть"
	}
	pause := fyne.NewMenuItem(label, a.togglePause)
	pause.Disabled = !loaded
	previous := fyne.NewMenuItem("Предыдущий трек", a.previous)
	previous.Disabled = !loaded
	next := fyne.NewMenuItem("Следующий трек", func() { a.move(1) })
	next.Disabled = a.queue.Index+1 >= len(a.queue.Tracks)

	quit := fyne.NewMenuItem("Выход", a.quit)
	quit.IsQuit = true
	return append(items, fyne.NewMenuItem("Показать плеер", a.ShowWindow), fyne.NewMenuItemSeparator(),
		pause, previous, next, fyne.NewMenuItemSeparator(), quit)
}

func (a *Application) refreshTray() {
	if a.tray == nil {
		return
	}
	items := a.trayItems()
	var key strings.Builder
	for _, item := range items {
		key.WriteString(item.Label + strconv.FormatBool(item.Disabled) + "\x00")
	}
	if key.String() == a.trayKey {
		return
	}
	a.trayKey = key.String()
	a.trayMenu.Items = items
	a.tray.SetSystemTrayMenu(a.trayMenu)
}

func trayTitle(title string) string {
	if runes := []rune(title); len(runes) > 60 {
		return string(runes[:59]) + "…"
	}
	return title
}
