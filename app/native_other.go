//go:build !windows

package app

import (
	"image/color"

	"fyne.io/fyne/v2"
	"github.com/godbus/dbus/v5"
)

func paintTitleBar(fyne.Window, bool, color.Color, color.Color, color.Color) {}
func windowMinimized(fyne.Window) bool                                       { return false }
func hideMinimized(fyne.Window)                                              {}
func restoreWindow(fyne.Window)                                              {}

func trayAvailable() bool {
	bus, err := dbus.SessionBus()
	if err != nil {
		return false
	}
	var owned bool
	err = bus.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, "org.kde.StatusNotifierWatcher").Store(&owned)
	return err == nil && owned
}
