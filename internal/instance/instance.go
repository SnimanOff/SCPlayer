package instance

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"time"

	"sc_player/internal/localipc"
)

var address = func() string {
	if runtime.GOOS == "windows" {
		user := regexp.MustCompile(`[^A-Za-z0-9_.-]`).ReplaceAllString(os.Getenv("USERDOMAIN")+"-"+os.Getenv("USERNAME"), "_")
		return `\\.\pipe\sc-player-` + user
	}
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "sc-player-"+strconv.Itoa(os.Getuid()))
		_ = os.MkdirAll(dir, 0o700)
	}
	return filepath.Join(dir, "sc-player.sock")
}

func Notify() bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := localipc.Dial(ctx, address())
	if err != nil {
		return false
	}
	defer conn.Close()
	allowForeground()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.WriteString(conn, "show\n"); err != nil {
		return false
	}
	reply, err := bufio.NewReader(conn).ReadString('\n')
	return err == nil && reply == "ok\n"
}

func Listen(show func()) (io.Closer, error) {
	listener, err := localipc.Listen(address())
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
				if line, err := bufio.NewReader(conn).ReadString('\n'); err == nil && line == "show\n" {
					show()
					_, _ = io.WriteString(conn, "ok\n")
				}
			}()
		}
	}()
	return listener, nil
}
