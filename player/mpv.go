package player

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"sc_player/internal/localipc"
)

type response struct {
	Error     string          `json:"error"`
	Data      json.RawMessage `json:"data"`
	RequestID uint64          `json:"request_id"`
	Event     string          `json:"event"`
	Name      string          `json:"name"`
	Reason    string          `json:"reason"`
}

type MPV struct {
	path      string
	startMu   sync.Mutex
	mu        sync.Mutex
	writeMu   sync.Mutex
	conn      net.Conn
	cmd       *exec.Cmd
	done      chan struct{}
	directory string
	closed    bool
	nextID    uint64
	pending   map[uint64]chan response
	state     State
	volume    float64
	events    chan Event
	ctx       context.Context
	cancel    context.CancelFunc
}

func NewMPV(path string) *MPV {
	ctx, cancel := context.WithCancel(context.Background())
	return &MPV{path: path, pending: make(map[uint64]chan response), events: make(chan Event, 8), ctx: ctx, cancel: cancel, volume: 80, state: State{Volume: 80, Idle: true}}
}

func FindMPV(configured string) (string, error) {
	if configured != "" {
		return exec.LookPath(configured)
	}
	name := "mpv"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if executable, err := os.Executable(); err == nil {
		for _, path := range []string{filepath.Join(filepath.Dir(executable), name), filepath.Join(filepath.Dir(executable), "bin", name)} {
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path, nil
			}
		}
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", errors.New("аудиодвижок mpv не найден; установите mpv или задайте MPV_PATH (см. README.md)")
	}
	return path, nil
}

func (p *MPV) start(ctx context.Context) error {
	p.startMu.Lock()
	defer p.startMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	connected, closed := p.conn != nil, p.closed
	p.mu.Unlock()
	if closed {
		return errors.New("плеер закрыт")
	}
	if connected {
		return nil
	}

	p.mu.Lock()
	oldCmd, oldDone, oldDir := p.cmd, p.done, p.directory
	p.cmd, p.done, p.directory = nil, nil, ""
	p.mu.Unlock()
	if oldCmd != nil {
		_ = oldCmd.Process.Kill()
		<-oldDone
	}
	if oldDir != "" {
		_ = os.RemoveAll(oldDir)
	}
	path, err := FindMPV(p.path)
	if err != nil {
		return err
	}
	directory, err := os.MkdirTemp("", "sc-player-")
	if err != nil {
		return err
	}
	socket := filepath.Join(directory, "mpv.sock")
	if runtime.GOOS == "windows" {
		socket = `\\.\pipe\` + filepath.Base(directory)
	}
	p.mu.Lock()
	volume := p.volume
	p.mu.Unlock()

	cmd := exec.Command(path, "--idle=yes", "--no-video", "--no-terminal", "--no-config", "--ytdl=no", "--input-ipc-server="+socket, "--cache=yes", "--demuxer-max-bytes=16MiB", "--cache-secs=60", "--volume="+strconv.FormatFloat(volume, 'f', -1, 64))
	localipc.PrepareChild(cmd)
	if err := cmd.Start(); err != nil {
		os.RemoveAll(directory)
		return fmt.Errorf("запуск mpv: %w", err)
	}
	localipc.BindChild(cmd)
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	connectCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	var conn net.Conn
	for {
		conn, err = localipc.Dial(connectCtx, socket)
		if err == nil {
			break
		}
		select {
		case <-connectCtx.Done():
			_ = cmd.Process.Kill()
			<-done
			os.RemoveAll(directory)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New("mpv не ответил при запуске")
		case <-done:
			os.RemoveAll(directory)
			return errors.New("mpv завершился при запуске")
		case <-time.After(50 * time.Millisecond):
		}
	}
	p.mu.Lock()
	p.conn, p.cmd, p.done, p.directory = conn, cmd, done, directory
	p.mu.Unlock()
	go p.read(conn)
	for index, name := range []string{"time-pos", "duration", "pause", "volume", "paused-for-cache", "idle-active"} {
		if err := p.command(ctx, "observe_property", index+1, name); err != nil {

			p.mu.Lock()
			p.conn, p.cmd, p.done, p.directory = nil, nil, nil, ""
			p.state.Idle = true
			p.mu.Unlock()
			_ = conn.Close()
			_ = cmd.Process.Kill()
			<-done
			_ = os.RemoveAll(directory)
			return err
		}
	}

	p.mu.Lock()
	latest := p.volume
	p.mu.Unlock()
	if latest != volume {
		return p.command(ctx, "set_property", "volume", latest)
	}
	return nil
}

func (p *MPV) command(ctx context.Context, args ...any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p.mu.Lock()
	conn := p.conn
	if conn == nil {
		p.mu.Unlock()
		return errors.New("аудиодвижок не запущен")
	}
	p.nextID++
	id := p.nextID
	result := make(chan response, 1)
	p.pending[id] = result
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.pending, id); p.mu.Unlock() }()
	data, err := json.Marshal(map[string]any{"command": args, "request_id": id})
	if err != nil {
		return err
	}
	p.writeMu.Lock()
	deadline, _ := ctx.Deadline()
	_ = conn.SetWriteDeadline(deadline)
	_, err = conn.Write(append(data, '\n'))
	p.writeMu.Unlock()
	if err != nil {
		return errors.New("потеряно соединение с аудиодвижком")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.ctx.Done():
		return errors.New("плеер закрыт")
	case response := <-result:
		if response.Error != "success" {
			return fmt.Errorf("mpv: %s", response.Error)
		}
		return nil
	}
}

func (p *MPV) read(conn net.Conn) {
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		var msg response
		if json.Unmarshal(scanner.Bytes(), &msg) != nil {
			continue
		}
		p.mu.Lock()
		if p.conn != conn {
			p.mu.Unlock()
			break
		}
		if msg.RequestID != 0 {
			if ch := p.pending[msg.RequestID]; ch != nil {
				select {
				case ch <- msg:
				default:
				}
			}
		}

		changed := msg.Event == "playback-restart"
		if msg.Event == "property-change" {
			switch msg.Name {
			case "time-pos":
				_ = json.Unmarshal(msg.Data, &p.state.Position)
			case "duration":
				_ = json.Unmarshal(msg.Data, &p.state.Duration)
				changed = true
			case "pause":
				_ = json.Unmarshal(msg.Data, &p.state.Paused)
				changed = true
			case "volume":
				_ = json.Unmarshal(msg.Data, &p.state.Volume)
			case "paused-for-cache":
				_ = json.Unmarshal(msg.Data, &p.state.Buffering)
			case "idle-active":
				_ = json.Unmarshal(msg.Data, &p.state.Idle)
				changed = true
			}
		}
		if msg.Event == "end-file" {
			p.state.Idle = true
			p.state.Buffering = false
		}
		p.mu.Unlock()
		if changed {
			p.notify()
		}
		if msg.Event == "end-file" {
			if msg.Reason == "eof" {
				p.emit(Event{Ended: true})
			}
			if msg.Reason == "error" {
				p.emit(Event{Err: errors.New("не удалось воспроизвести поток; попробуйте снова")})
			}
		}
	}
	p.mu.Lock()
	if p.conn != conn {
		p.mu.Unlock()
		_ = conn.Close()
		return
	}
	p.conn = nil
	for _, ch := range p.pending {
		select {
		case ch <- response{Error: "connection closed"}:
		default:
		}
	}
	closed := p.closed
	p.state.Idle = true
	p.mu.Unlock()
	_ = conn.Close()
	if !closed {
		p.emit(Event{Err: errors.New("аудиодвижок завершил работу")})
	}
}

func (p *MPV) emit(event Event) {
	select {
	case p.events <- event:
	case <-p.ctx.Done():
	}
}

func (p *MPV) notify() {
	select {
	case p.events <- Event{Changed: true}:
	default:
	}
}
func (p *MPV) Events() <-chan Event { return p.events }
func (p *MPV) Snapshot() State      { p.mu.Lock(); defer p.mu.Unlock(); return p.state }

func (p *MPV) Load(ctx context.Context, stream string) error {
	if err := p.start(ctx); err != nil {
		return err
	}
	p.mu.Lock()
	p.state.Position = 0
	p.state.Duration = 0
	p.mu.Unlock()
	if err := p.command(ctx, "loadfile", stream, "replace"); err != nil {
		return err
	}
	return p.Pause(ctx, false)
}
func (p *MPV) Pause(ctx context.Context, paused bool) error {
	return p.command(ctx, "set_property", "pause", paused)
}
func (p *MPV) Seek(ctx context.Context, seconds float64) error {
	return p.command(ctx, "seek", seconds, "absolute+exact")
}

func (p *MPV) SetVolume(ctx context.Context, volume float64) error {
	volume = min(max(volume, 0), 100)
	p.mu.Lock()
	p.volume = volume
	connected := p.conn != nil
	p.mu.Unlock()
	if !connected {
		return ctx.Err()
	}
	return p.command(ctx, "set_property", "volume", volume)
}
func (p *MPV) Stop(ctx context.Context) error {
	p.mu.Lock()
	connected := p.conn != nil
	p.mu.Unlock()
	if !connected {
		return nil
	}
	return p.command(ctx, "stop")
}
func (p *MPV) Close() error {
	p.startMu.Lock()
	defer p.startMu.Unlock()
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	conn, cmd, done, directory := p.conn, p.cmd, p.done, p.directory
	p.mu.Unlock()
	if conn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = p.command(ctx, "quit")
		cancel()
		_ = conn.Close()
	}
	p.cancel()
	if cmd != nil {
		select {
		case <-done:
		case <-time.After(time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}
	if directory != "" {
		return os.RemoveAll(directory)
	}
	return nil
}
