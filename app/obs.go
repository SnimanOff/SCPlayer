package app

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"sc_player/api/soundcloud"
	"sc_player/player"
)

func (a *Application) setOBSEnabled(enabled bool) error {
	if enabled {
		// Publish the current state before accepting the first browser request.
		a.updateOBS()
		if err := a.obs.Start(); err != nil {
			a.obsEnabled = false
			a.obsError = fmt.Errorf("Не удалось включить виджет OBS. Возможно, порт занят: %w", err)
			return a.obsError
		}
	} else {
		_ = a.obs.Close()
	}
	a.obsEnabled = enabled
	a.obsError = nil
	return nil
}

func (a *Application) updateOBS() {
	if !a.playReady || a.loading {
		a.obs.Update(soundcloud.Track{}, player.State{Idle: true})
		return
	}
	a.obs.Update(a.playing, a.engine.Snapshot())
}

func (a *Application) obsSettings() fyne.CanvasObject {
	enabled := widget.NewCheck("Включить виджет OBS", nil)
	enabled.SetChecked(a.obsEnabled)
	address := widget.NewEntry()
	address.SetText(a.obs.URL())
	address.Disable()
	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	copyURL := widget.NewButton("Копировать ссылку", func() {
		a.GUI.Clipboard().SetContent(a.obs.URL())
		status.SetText("Ссылка скопирована")
	})
	refresh := func() {
		if a.obsEnabled {
			copyURL.Enable()
			status.SetText("Виджет включён")
		} else {
			copyURL.Disable()
			status.SetText("Виджет выключен")
		}
		if a.obsError != nil {
			status.SetText(a.obsError.Error())
		}
	}
	enabled.OnChanged = func(on bool) {
		_ = a.setOBSEnabled(on)
		// A failed bind must not leave the switch displaying a running widget.
		enabled.Checked = a.obsEnabled
		enabled.Refresh()
		saved := a.obsEnabled
		a.savePreference(func(p fyne.Preferences) { p.SetBool("obs", saved) })
		refresh()
	}
	refresh()
	help := widget.NewLabel("В OBS добавьте источник «Браузер», вставьте ссылку и задайте размер 480 × 96. Плеер и OBS должны работать на одном компьютере. Длинные строки обрезаются с многоточием.")
	help.Wrapping = fyne.TextWrapWord
	return container.NewVBox(enabled, address, copyURL, status, help)
}

func (a *Application) showOBS() {
	var popup *widget.PopUp
	closeButton := widget.NewButton("Закрыть", func() { popup.Hide() })
	title := widget.NewLabelWithStyle("Виджет для OBS", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	popup = widget.NewModalPopUp(container.NewBorder(title, closeButton, nil, nil, a.obsSettings()), a.Window.Canvas())
	popup.Resize(fyne.NewSize(350, 300))
	popup.Show()
}
