package player

import (
	"encoding/binary"
	"io"
)

// monoReader mixes each int16 stereo frame using int32 to avoid overflow, and
// duplicates the result to stereo PCM for downstream routing.
type monoReader struct {
	source  io.Reader
	frame   [4]byte
	pending []byte
	err     error
}

func (r *monoReader) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		if len(r.pending) == 0 {
			if r.err != nil {
				return n, r.err
			}
			_, r.err = io.ReadFull(r.source, r.frame[:])
			if r.err != nil {
				return n, r.err
			}
			l := int32(int16(binary.LittleEndian.Uint16(r.frame[:2])))
			right := int32(int16(binary.LittleEndian.Uint16(r.frame[2:])))
			v := uint16(int16((l + right) / 2))
			binary.LittleEndian.PutUint16(r.frame[:2], v)
			binary.LittleEndian.PutUint16(r.frame[2:], v)
			r.pending = r.frame[:]
		}
		copied := copy(p[n:], r.pending)
		n += copied
		r.pending = r.pending[copied:]
	}
	return n, nil
}
