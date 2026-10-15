package instance

import "golang.org/x/sys/windows"

var procAllowSetForegroundWindow = windows.NewLazySystemDLL("user32.dll").NewProc("AllowSetForegroundWindow")

func allowForeground() {
	const anyProcess = ^uintptr(0)
	procAllowSetForegroundWindow.Call(anyProcess)
}
