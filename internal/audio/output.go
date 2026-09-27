// Package audio defines the PCM output boundary used by zone players.
package audio

import "io"

type SampleFormat uint8

const (
	SignedInt16LE SampleFormat = iota + 1
)

type PCMFormat struct {
	SampleRate   int
	Channels     int
	SampleFormat SampleFormat
}

// Output represents a shared audio backend. Open creates a stream handle,
// not a separate physical device; a router can feed it one mixed stream.
type Output interface {
	Open(source io.Reader, format PCMFormat) (Stream, error)
}

type Stream interface {
	Play()
	Pause()
	IsPlaying() bool
	Err() error
	SetVolume(float64)
}
