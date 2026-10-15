package player

import (
	"sc_player/api/soundcloud"
	"testing"
)

func TestQueueNavigationAndIsolation(t *testing.T) {
	var q Queue
	if _, ok := q.Current(); ok {
		t.Fatal("empty queue has a current track")
	}
	tracks := []soundcloud.Track{{ID: 1}, {ID: 2}, {ID: 3}}
	if !q.Set(tracks, 1) {
		t.Fatal("set failed")
	}
	tracks[1].ID = 99
	if got, _ := q.Current(); got.ID != 2 {
		t.Fatal("queue aliases caller's slice")
	}
	if q.Set(tracks, -1) || q.Set(nil, 0) || q.Move(2) {
		t.Fatal("invalid selection accepted")
	}
	if q.Index != 1 {
		t.Fatal("invalid move changed index")
	}
	if !q.Move(1) || q.Move(1) {
		t.Fatal("queue must stop at the end")
	}
	if !q.Move(-2) || q.Move(-1) {
		t.Fatal("queue must stop at the beginning")
	}
}
