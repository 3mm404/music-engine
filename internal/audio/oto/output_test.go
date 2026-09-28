package oto

import (
	"music-engine/internal/audio"
	"strings"
	"testing"
)

func TestUnsupportedFormatAndClosedOutput(t *testing.T) {
	o := NewOutput()
	if _, err := o.Open(strings.NewReader(""), audio.PCMFormat{SampleRate: 44100, Channels: 4, SampleFormat: audio.SignedInt16LE}); err == nil {
		t.Fatal("accepted multichannel output")
	}
	if err := o.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Open(strings.NewReader(""), audio.PCMFormat{SampleRate: 44100, Channels: 2, SampleFormat: audio.SignedInt16LE}); err == nil {
		t.Fatal("opened closed output")
	}
}
