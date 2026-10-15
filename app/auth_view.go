package app

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func wrapped(text string) *widget.Label {
	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapWord
	return label
}

func (a *Application) newPendingLoginView() fyne.CanvasObject {
	progress := widget.NewProgressBarInfinite()
	a.loginProgress = progress
	cancel := widget.NewButton("Отменить вход", a.cancelLogin)
	return container.NewPadded(container.NewVBox(
		a.Heading("Вход в SoundCloud"),
		widget.NewSeparator(), wrapped("Войдите на сайте SoundCloud в открывшемся окне браузера. Как только вход завершится, здесь появятся ваши плейлисты и лента."),
		progress, cancel))
}
