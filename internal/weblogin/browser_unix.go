//go:build !windows

package weblogin

import (
	"os/exec"
	"strings"
)

func defaultFirefox() (string, bool) {
	out, err := exec.Command("xdg-settings", "get", "default-web-browser").Output()
	return "", err == nil && strings.Contains(strings.ToLower(string(out)), "firefox")
}

func openFirefox(_, url string) error {
	return startDetached(exec.Command("xdg-open", url))
}
