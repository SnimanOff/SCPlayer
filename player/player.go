package player

import "context"

type State struct {
	Position  float64
	Duration  float64
	Volume    float64
	Paused    bool
	Buffering bool
	Idle      bool
}

type Event struct {
	Ended bool

	Changed bool
	Err     error
}

type Engine interface {
	Load(context.Context, string) error
	Pause(context.Context, bool) error
	Seek(context.Context, float64) error
	SetVolume(context.Context, float64) error
	Stop(context.Context) error
	Snapshot() State
	Events() <-chan Event
	Close() error
}
