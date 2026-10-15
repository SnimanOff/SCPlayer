package weblogin

import (
	"os/exec"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var assocQueryString = windows.NewLazySystemDLL("shlwapi.dll").NewProc("AssocQueryStringW")

func defaultFirefox() (string, bool) {
	const assocStrExecutable = 2
	assoc, _ := windows.UTF16PtrFromString("https")
	verb, _ := windows.UTF16PtrFromString("open")
	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := assocQueryString.Find(); err != nil {
		return "", false
	}
	hr, _, _ := assocQueryString.Call(0, assocStrExecutable, uintptr(unsafe.Pointer(assoc)), uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if hr != 0 {
		return "", false
	}
	path := windows.UTF16ToString(buf)
	return path, strings.EqualFold(filepath.Base(path), "firefox.exe")
}

func openFirefox(executable, url string) error {
	return startDetached(exec.Command(executable, url))
}
