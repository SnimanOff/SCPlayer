//go:build !ci

package weblogin

import (
	"fmt"
	"os"

	webview "github.com/webview/webview_go"

	"sc_player/api/soundcloud"
)

func Run() int {
	w := webview.New(false)
	if w == nil {
		return 1
	}
	defer w.Destroy()
	w.SetTitle("Вход в SoundCloud — SC Player")
	w.SetSize(480, 720, webview.HintNone)
	captured := false
	stale := os.Getenv(staleVariable)
	_ = w.Bind("scPlayerToken", func(token string) {
		if captured || !soundcloud.ValidToken(token) || token == stale {
			return
		}
		captured = true
		fmt.Fprintln(os.Stdout, token)
		w.Terminate()
	})
	w.Init(captureScript)

	w.Navigate(signInURL)
	w.Run()
	if !captured {
		return exitCancelled
	}
	return 0
}
