package player

import "sc_player/api/soundcloud"

type Queue struct {
	Tracks []soundcloud.Track
	Index  int
}

func (q *Queue) Set(tracks []soundcloud.Track, index int) bool {
	if index < 0 || index >= len(tracks) {
		return false
	}
	q.Tracks = append([]soundcloud.Track(nil), tracks...)
	q.Index = index
	return true
}
func (q *Queue) Current() (soundcloud.Track, bool) {
	if q.Index < 0 || q.Index >= len(q.Tracks) {
		return soundcloud.Track{}, false
	}
	return q.Tracks[q.Index], true
}
func (q *Queue) Move(delta int) bool {
	index := q.Index + delta
	if index < 0 || index >= len(q.Tracks) {
		return false
	}
	q.Index = index
	return true
}
