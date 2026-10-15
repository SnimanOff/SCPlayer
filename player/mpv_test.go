package player

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"
)

func TestMPVProtocolAndEvents(t *testing.T) {
	p := NewMPV("")
	client, server := net.Pipe()
	p.conn = client
	go p.read(client)
	defer p.Close()
	defer server.Close()
	commands := make(chan []any, 8)
	go func() {
		scanner := bufio.NewScanner(server)
		for scanner.Scan() {
			var request struct {
				Command []any  `json:"command"`
				ID      uint64 `json:"request_id"`
			}
			if json.Unmarshal(scanner.Bytes(), &request) != nil {
				return
			}
			commands <- request.Command
			_, _ = fmt.Fprintf(server, "{\"request_id\":%d,\"error\":\"success\"}\n", request.ID)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, volume := range []float64{-4, 150} {
		if err := p.SetVolume(ctx, volume); err != nil {
			t.Fatal(err)
		}
		command := <-commands
		want := float64(0)
		if volume > 100 {
			want = 100
		}
		if command[0] != "set_property" || command[1] != "volume" || command[2] != want {
			t.Fatalf("bad volume command: %v", command)
		}
	}
	if err := p.Seek(ctx, 42); err != nil {
		t.Fatal(err)
	}
	if command := <-commands; command[0] != "seek" || command[2] != "absolute+exact" {
		t.Fatalf("bad seek: %v", command)
	}
	_, _ = fmt.Fprintln(server, `{"event":"property-change","name":"time-pos","data":42}`)
	_, _ = fmt.Fprintln(server, `{"event":"property-change","name":"duration","data":90}`)
	_, _ = fmt.Fprintln(server, `{"event":"end-file","reason":"eof"}`)
	if event := nextEvent(ctx, t, p); !event.Changed {
		t.Fatalf("duration change not reported: %+v", event)
	}
	if event := nextEvent(ctx, t, p); !event.Ended || event.Err != nil {
		t.Fatalf("unexpected event: %+v", event)
	}
	if state := p.Snapshot(); state.Position != 42 || state.Duration != 90 || !state.Idle {
		t.Fatalf("bad state: %+v", state)
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if err := p.Pause(canceled, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func nextEvent(ctx context.Context, t *testing.T, p *MPV) Event {
	t.Helper()
	select {
	case event := <-p.Events():
		return event
	case <-ctx.Done():
		t.Fatal("event missing")
		return Event{}
	}
}

func TestMPVUnavailableAndClosed(t *testing.T) {
	p := NewMPV("definitely-not-an-installed-mpv")
	if err := p.Load(context.Background(), "https://example.com/audio"); err == nil {
		t.Fatal("missing mpv accepted")
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.Load(context.Background(), "https://example.com/audio"); err == nil {
		t.Fatal("closed player restarted")
	}
}

func TestMPVProcessIntegration(t *testing.T) {
	binary := os.Getenv("SC_PLAYER_MPV_TEST_BINARY")
	if binary == "" {
		t.Skip("set SC_PLAYER_MPV_TEST_BINARY to test a real mpv process")
	}
	p := NewMPV(binary)
	defer p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := p.SetVolume(ctx, 37); err != nil {
		t.Fatal(err)
	}
	if err := p.start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := p.Pause(ctx, true); err != nil {
		t.Fatal(err)
	}
	for state := p.Snapshot(); state.Volume != 37 || !state.Paused; state = p.Snapshot() {
		select {
		case <-ctx.Done():
			t.Fatalf("property updates missing: %+v", state)
		case <-time.After(10 * time.Millisecond):
		}
	}
	p.mu.Lock()
	directory, process := p.directory, p.cmd.Process
	p.mu.Unlock()
	if err := process.Kill(); err != nil {
		t.Fatal(err)
	}
	for event := nextEvent(ctx, t, p); event.Err == nil; event = nextEvent(ctx, t, p) {
		if !event.Changed {
			t.Fatalf("crash did not report an error: %+v", event)
		}
	}
	if err := p.start(ctx); err != nil {
		t.Fatalf("restart failed: %v", err)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("crashed process socket directory leaked: %v", err)
	}
	p.mu.Lock()
	directory = p.directory
	p.mu.Unlock()
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("socket directory leaked after close: %v", err)
	}
}
