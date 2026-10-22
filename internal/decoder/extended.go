package decoder

import (
	"errors"
	"io"
	"math"

	mp3pcm "github.com/tphakala/go-mp3/pcm"
)

// The established decoder remains the main path. This fallback also supports
// MPEG-2.5, without native libraries or an external transcoder.
type extendedDecoder struct {
	decoder          *mp3pcm.Decoder
	rate, channels   int
	length, position int64
	pending          [4]byte
	pendingAt        int
}

func openExtended(source io.Reader) (*extendedDecoder, error) {
	d, err := mp3pcm.NewDecoder(source)
	if err != nil {
		return nil, err
	}
	samples, err := d.SeekToSample(math.MaxInt64)
	if err != nil {
		return nil, err
	}
	if _, err = d.SeekToSample(0); err != nil {
		return nil, err
	}
	info := d.Info()
	return &extendedDecoder{decoder: d, rate: info.SampleRate, channels: info.Channels, length: samples * 4, pendingAt: 4}, nil
}
func (d *extendedDecoder) SampleRate() int { return d.rate }
func (d *extendedDecoder) Length() int64   { return d.length }
func (d *extendedDecoder) Read(buffer []byte) (int, error) {
	if d.channels == 2 {
		n, err := d.decoder.Read(buffer)
		d.position += int64(n)
		return n, err
	}
	written := 0
	for written < len(buffer) {
		if d.pendingAt == 4 {
			var mono [2]byte
			if _, err := io.ReadFull(d.decoder, mono[:]); err != nil {
				return written, err
			}
			copy(d.pending[:2], mono[:])
			copy(d.pending[2:], mono[:])
			d.pendingAt = 0
		}
		n := copy(buffer[written:], d.pending[d.pendingAt:])
		written += n
		d.pendingAt += n
		d.position += int64(n)
	}
	return written, nil
}
func (d *extendedDecoder) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += d.position
	case io.SeekEnd:
		offset += d.length
	default:
		return d.position, errors.New("origen seek inválido")
	}
	if offset < 0 || offset > d.length {
		return d.position, errors.New("posición fuera del audio")
	}
	sample, err := d.decoder.SeekToSample(offset / 4)
	if err != nil {
		return d.position, err
	}
	d.position = sample * 4
	d.pendingAt = 4
	return d.position, nil
}
