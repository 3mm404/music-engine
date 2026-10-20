package player

import (
	"io"
	"sync/atomic"
)

// pcmProgress measures PCM delivered to the output, not download progress.
// MP3 decoding yields stereo int16 PCM: four bytes per frame, including mono mixing.
// Output buffering can put this position slightly ahead of audible sound.
type pcmProgress struct {
	bytes  atomic.Int64
	rate   int
	length int64
}

type progressReader struct {
	source   io.Reader
	progress *pcmProgress
}

func (r *progressReader) Read(buffer []byte) (int, error) {
	n, err := r.source.Read(buffer)
	r.progress.bytes.Add(int64(n))
	return n, err
}

func (p *pcmProgress) times() (position, duration *int) {
	if p.rate <= 0 {
		return nil, nil
	}
	read := p.bytes.Load()
	if p.length >= 0 && read > p.length {
		read = p.length
	}
	ms := int(read * 1000 / (int64(p.rate) * 4))
	position = &ms
	if p.length > 0 {
		total := int(p.length * 1000 / (int64(p.rate) * 4))
		duration = &total
	}
	return
}
