package instance

import (
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func TestSecondLaunchShowsFirst(t *testing.T) {
	endpoint := filepath.Join(t.TempDir(), "player.sock")
	if runtime.GOOS == "windows" {
		endpoint = `\\.\pipe\sc-player-test-` + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	defer func(saved func() string) { address = saved }(address)
	address = func() string { return endpoint }

	if Notify() {
		t.Fatal("notified a player that does not run")
	}
	shown := make(chan struct{}, 1)
	closer, err := Listen(func() { shown <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(func() {}); err == nil {
		t.Fatal("a second player took over the endpoint")
	}
	if !Notify() {
		t.Fatal("running player did not answer")
	}
	select {
	case <-shown:
	case <-time.After(2 * time.Second):
		t.Fatal("show not called")
	}
	closer.Close()
	if Notify() {
		t.Fatal("closed player still answered")
	}
}
