package main

import (
	"os"

	"sc_player/api/discord"
	"sc_player/api/soundcloud"
	"sc_player/app"
	"sc_player/internal/config"
	"sc_player/internal/icon"
	"sc_player/internal/instance"
	"sc_player/internal/weblogin"
	"sc_player/player"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/widget"
)

func main() {

	if len(os.Args) == 2 && os.Args[1] == weblogin.Flag {
		os.Exit(weblogin.Run())
	}

	if instance.Notify() {
		return
	}
	gui := fyneapp.NewWithID("io.scplayer.desktop")
	gui.SetIcon(icon.Resource)
	cfg, err := config.Load()
	if err != nil {
		window := gui.NewWindow("SC Player")
		window.Resize(fyne.NewSize(390, 200))
		message := widget.NewLabel(err.Error())
		message.Wrapping = fyne.TextWrapWord
		window.SetContent(message)
		window.ShowAndRun()
		return
	}
	session := soundcloud.NewDesktopSession()

	presence := discord.New(cfg.DiscordID, gui.Preferences().BoolWithFallback("discord", true))
	application := app.New(gui, session, soundcloud.NewClient(session), player.NewMPV(cfg.MPVPath), presence)
	application.SignIn = weblogin.SignIn
	if later, err := instance.Listen(func() { fyne.Do(application.ShowWindow) }); err == nil {
		defer later.Close()
	}
	defer application.Close()
	application.Run()
}
