package app

import (
	"image/color"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"golang.org/x/sys/windows"
)

func paintTitleBar(w fyne.Window, dark bool, caption, text, border color.Color) {
	native, ok := w.(driver.NativeWindow)
	if !ok {
		return
	}
	native.RunNative(func(context any) {
		win, ok := context.(driver.WindowsWindowContext)
		if !ok || win.HWND == 0 {
			return
		}
		hwnd := windows.HWND(win.HWND)
		var on uint32
		if dark {
			on = 1
		}
		_ = windows.DwmSetWindowAttribute(hwnd, windows.DWMWA_USE_IMMERSIVE_DARK_MODE, unsafe.Pointer(&on), 4)
		for attribute, c := range map[uint32]color.Color{windows.DWMWA_CAPTION_COLOR: caption, windows.DWMWA_TEXT_COLOR: text, windows.DWMWA_BORDER_COLOR: border} {
			ref := colorRef(c)
			_ = windows.DwmSetWindowAttribute(hwnd, attribute, unsafe.Pointer(&ref), 4)
		}
	})
}

func colorRef(c color.Color) uint32 {
	r, g, b, _ := c.RGBA()
	return uint32(b>>8)<<16 | uint32(g>>8)<<8 | uint32(r>>8)
}

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procIsIconic            = user32.NewProc("IsIconic")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
)

func hwnd(w fyne.Window) uintptr {
	var handle uintptr
	if native, ok := w.(driver.NativeWindow); ok {
		native.RunNative(func(context any) {
			if win, ok := context.(driver.WindowsWindowContext); ok {
				handle = win.HWND
			}
		})
	}
	return handle
}

func windowMinimized(w fyne.Window) bool {
	h := hwnd(w)
	if h == 0 {
		return false
	}
	iconic, _, _ := procIsIconic.Call(h)
	return iconic != 0 && windows.IsWindowVisible(windows.HWND(h))
}

func hideMinimized(w fyne.Window) {
	if h := hwnd(w); h != 0 {
		procShowWindow.Call(h, windows.SW_HIDE)
	}
}

func restoreWindow(w fyne.Window) {
	h := hwnd(w)
	if h == 0 {
		return
	}
	if iconic, _, _ := procIsIconic.Call(h); iconic != 0 || !windows.IsWindowVisible(windows.HWND(h)) {
		procShowWindow.Call(h, windows.SW_RESTORE)
	}
	procSetForegroundWindow.Call(h)
}

func trayAvailable() bool { return true }
