package discord

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"sc_player/api/soundcloud"
	"sc_player/internal/localipc"
	"sc_player/player"
)

var DefaultClientID = "1059572226942304356"

type Service interface {
	Update(soundcloud.Track, player.State)

	SetEnabled(bool)

	Configured() bool
	Statuses() <-chan string
	Close()
}

type Update struct {
	Track soundcloud.Track
	State player.State

	At time.Time
}

type Presence struct {
	id       string
	updates  chan Update
	enabled  chan bool
	statuses chan string
	cancel   context.CancelFunc
	done     chan struct{}
}

func New(clientID string, enabled bool) *Presence {
	if clientID == "" {
		clientID = DefaultClientID
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &Presence{id: clientID, updates: make(chan Update, 1), enabled: make(chan bool, 1), statuses: make(chan string, 1), cancel: cancel, done: make(chan struct{})}
	go p.run(ctx, enabled)
	return p
}
func (p *Presence) SetEnabled(on bool) {
	select {
	case p.enabled <- on:
	default:
		select {
		case <-p.enabled:
		default:
		}
		select {
		case p.enabled <- on:
		default:
		}
	}
}
func (p *Presence) Update(track soundcloud.Track, state player.State) {
	update := Update{Track: track, State: state, At: time.Now()}
	select {
	case p.updates <- update:
	default:
		select {
		case <-p.updates:
		default:
		}
		select {
		case p.updates <- update:
		default:
		}
	}
}
func (p *Presence) Statuses() <-chan string { return p.statuses }
func (p *Presence) Configured() bool        { return p.id != "" }
func (p *Presence) Close() {
	p.cancel()
	<-p.done
}
func (p *Presence) status(message string) {
	select {
	case p.statuses <- message:
	default:
		select {
		case <-p.statuses:
		default:
		}
		select {
		case p.statuses <- message:
		default:
		}
	}
}

func (p *Presence) run(ctx context.Context, enabled bool) {
	defer close(p.done)
	if p.id == "" {
		p.status("Discord: не настроен в этой сборке")
		<-ctx.Done()
		return
	}
	timer := time.NewTicker(15 * time.Second)
	defer timer.Stop()
	var conn net.Conn
	var current Update
	dirty := true
	last := ""
	say := func(message string) {
		if message != last {
			last = message
			p.status(message)
		}
	}
	disconnect := func() {
		if conn != nil {
			_ = setActivity(conn, nil)
			_ = conn.Close()
			conn = nil
		}
	}
	defer disconnect()
	for {
		if !enabled {
			disconnect()
			say("Discord: выключен")
		}
		if enabled && conn == nil {
			var err error
			conn, err = connect(ctx, p.id)
			var refused rejected
			switch {
			case err == nil:
				say("Discord: подключён")
				dirty = true
			case errors.As(err, &refused):
				say("Discord отклонил приложение: " + refused.message)
			default:
				say("Discord: ожидание запуска Discord")
			}
		}
		if conn != nil && dirty {
			err := setActivity(conn, activity(current))
			var refused rejected
			switch {
			case err == nil:
				dirty = false
				say(shown(current))
			case errors.As(err, &refused):

				dirty = false
				say("Discord не принял статус: " + refused.message)
			default:
				_ = conn.Close()
				conn = nil
				say("Discord: ожидание запуска Discord")
			}
		}
		select {
		case <-ctx.Done():
			return
		case current = <-p.updates:
			dirty = true
		case enabled = <-p.enabled:
			dirty = true
		case <-timer.C:
			dirty = true
		}
	}
}

func shown(update Update) string {
	if update.Track.Key() == "" {
		return "Discord: подключён"
	}
	return "Discord: в профиле — " + update.Track.Title
}

func activity(update Update) map[string]any {
	if update.Track.Key() == "" {
		return map[string]any{"type": 2, "details": "Выбирает музыку", "state": "SoundCloud"}
	}
	title := field(update.Track.Title)
	if title == "" {
		title = "Без названия"
	}
	creator := update.Track.Creator()
	if update.State.Paused || update.State.Idle {
		creator = strings.TrimSpace("Пауза · " + creator)
	}

	a := map[string]any{"type": 2, "status_display_type": 2, "details": title}
	if state := field(creator); state != "" {
		a["state"] = state
	}
	art := update.Track.ArtworkURL
	if art == "" {
		art = update.Track.User.AvatarURL
	}
	if art != "" {
		a["assets"] = map[string]string{"large_image": strings.Replace(art, "-large.", "-t300x300.", 1), "large_text": title}
	}
	if update.Track.PermalinkURL != "" {
		a["buttons"] = []map[string]string{{"label": "Открыть в SoundCloud", "url": update.Track.PermalinkURL}}
	}
	if !update.State.Paused && !update.State.Idle && update.State.Duration > 0 {
		at := update.At
		if at.IsZero() {
			at = time.Now()
		}
		start := at.Unix() - int64(update.State.Position)
		a["timestamps"] = map[string]int64{"start": start, "end": start + int64(update.State.Duration)}
	}
	return a
}

func field(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	units := 0
	for i, r := range value {
		n := max(utf16.RuneLen(r), 1)
		if units+n > 128 {
			value = value[:i]
			break
		}
		units += n
	}
	for ; units < 2; units++ {
		value += "\u200b"
	}
	return value
}

func paths() []string {
	var paths []string
	for i := 0; i < 10; i++ {
		name := "discord-ipc-" + strconv.Itoa(i)
		if runtime.GOOS == "windows" {
			paths = append(paths, `\\.\pipe\`+name)
			continue
		}
		for _, directory := range []string{os.Getenv("XDG_RUNTIME_DIR"), os.Getenv("TMPDIR"), os.Getenv("TMP"), os.TempDir()} {
			if directory != "" {
				paths = append(paths, filepath.Join(directory, name), filepath.Join(directory, "app/com.discordapp.Discord", name), filepath.Join(directory, "snap.discord", name))
			}
		}
	}
	return paths
}

var pipePaths = paths

var errNotRunning = errors.New("Discord is not running")

type rejected struct{ message string }

func (r rejected) Error() string { return r.message }

func refusal(data []byte) rejected {
	var reply struct {
		Message string `json:"message"`
		Data    struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	_ = json.Unmarshal(data, &reply)
	message := reply.Data.Message
	if message == "" {
		message = reply.Message
	}
	if message == "" {
		message = "без объяснения"
	}
	return rejected{message: message}
}

func connect(ctx context.Context, id string) (net.Conn, error) {
	failure := errNotRunning
	for _, path := range pipePaths() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		dialCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		conn, err := localipc.Dial(dialCtx, path)
		cancel()
		if err != nil {
			continue
		}
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		if err = writeFrame(conn, 0, map[string]any{"v": 1, "client_id": id}); err == nil {
			var op uint32
			var data []byte
			op, data, err = readFrame(conn)
			var reply struct {
				Event string `json:"evt"`
			}
			if err == nil {
				_ = json.Unmarshal(data, &reply)
				if op == 2 || reply.Event != "READY" {
					err = refusal(data)
					failure = err
				}
			}
		}
		if err == nil {
			return conn, nil
		}
		_ = conn.Close()
	}
	return nil, failure
}

func setActivity(conn net.Conn, activity map[string]any) error {
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	nonce := strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := writeFrame(conn, 1, map[string]any{"cmd": "SET_ACTIVITY", "args": map[string]any{"pid": os.Getpid(), "activity": activity}, "nonce": nonce}); err != nil {
		return err
	}
	for {
		op, data, err := readFrame(conn)
		if err != nil {
			return err
		}
		if op == 3 {
			if err := writeRaw(conn, 4, data); err != nil {
				return err
			}
			continue
		}
		if op == 2 {
			return errors.New("Discord disconnected")
		}
		var reply struct {
			Nonce string `json:"nonce"`
			Event string `json:"evt"`
		}
		if err := json.Unmarshal(data, &reply); err != nil {
			return err
		}
		if reply.Nonce == nonce {
			if reply.Event == "ERROR" {
				return refusal(data)
			}
			return nil
		}
	}
}

func writeFrame(w io.Writer, opcode uint32, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return writeRaw(w, opcode, data)
}
func writeRaw(w io.Writer, opcode uint32, data []byte) error {
	frame := make([]byte, 8+len(data))
	binary.LittleEndian.PutUint32(frame, opcode)
	binary.LittleEndian.PutUint32(frame[4:], uint32(len(data)))
	copy(frame[8:], data)
	for len(frame) > 0 {
		n, err := w.Write(frame)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		frame = frame[n:]
	}
	return nil
}
func readFrame(r io.Reader) (uint32, []byte, error) {
	var header [8]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	length := binary.LittleEndian.Uint32(header[4:])
	if length > 1<<20 {
		return 0, nil, fmt.Errorf("Discord frame too large: %d", length)
	}
	data := make([]byte, int(length))
	_, err := io.ReadFull(r, data)
	return binary.LittleEndian.Uint32(header[:]), data, err
}
