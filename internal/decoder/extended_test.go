package decoder

import (
	"bytes"
	"io"
	"testing"
)

func TestMPEG25MonoReturnsStereoPCMAndSupportsSeek(t *testing.T) {
	frame := make([]byte, 576)
	copy(frame, []byte{0xff, 0xe3, 0x88, 0xc0})
	stream, err := OpenBytes(bytes.Repeat(frame, 40))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if stream.SampleRate() != 8000 {
		t.Fatalf("sample rate: %d", stream.SampleRate())
	}
	pcm, err := io.ReadAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(pcm)) != stream.Length() || len(pcm) != 40*576*4 {
		t.Fatalf("mono not expanded to stereo: %d", len(pcm))
	}
	for i := 0; i < len(pcm); i += 4 {
		if !bytes.Equal(pcm[i:i+2], pcm[i+2:i+4]) {
			t.Fatal("mono channels differ")
		}
	}
	if actual, err := stream.Seek(8000*4/2, io.SeekStart); err != nil || actual != 16000 {
		t.Fatalf("seek: %d %v", actual, err)
	}
	var sample [3]byte
	if _, err := io.ReadFull(stream, sample[:]); err != nil {
		t.Fatal(err)
	}
	if actual, err := stream.Seek(0, io.SeekStart); err != nil || actual != 0 {
		t.Fatalf("rewind: %d %v", actual, err)
	}
	repeated, err := io.ReadAll(stream)
	if err != nil || !bytes.Equal(pcm, repeated) {
		t.Fatalf("seek changed PCM: %v", err)
	}
}
