//go:build !windows

package localipc

import (
	"context"
	"net"
	"os"
)

func Dial(ctx context.Context, path string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", path)
}

func Listen(path string) (net.Listener, error) {
	listener, err := net.Listen("unix", path)
	if err == nil {
		return listener, nil
	}
	if conn, dialErr := net.Dial("unix", path); dialErr == nil {
		conn.Close()
		return nil, err
	}
	if os.Remove(path) != nil {
		return nil, err
	}
	return net.Listen("unix", path)
}
