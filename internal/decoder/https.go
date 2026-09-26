package decoder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const MaxAudioBytes = 32 << 20

// HTTPS downloads signed URLs without device credentials. Redirects are rejected
// so authorization query parameters cannot be forwarded to another endpoint.
type HTTPS struct {
	Client   *http.Client
	MaxBytes int64
	Timeout  time.Duration
	Backoff  time.Duration
}

func ValidURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.Fragment == ""
}

func (h HTTPS) Open(ctx context.Context, raw string, recovering func()) (*Stream, error) {
	if !ValidURL(raw) {
		return nil, errors.New("audio requiere una URL HTTPS valida")
	}
	limit := h.MaxBytes
	if limit <= 0 || limit > MaxAudioBytes {
		limit = MaxAudioBytes
	}
	timeout := h.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	backoff := h.Backoff
	if backoff <= 0 {
		backoff = 500 * time.Millisecond
	}
	client := http.Client{}
	if h.Client != nil {
		client = *h.Client
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if attempt > 0 {
			if recovering != nil {
				recovering()
			}
			timer := time.NewTimer(backoff * time.Duration(attempt))
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		data, retry, err := download(ctx, &client, raw, limit, timeout)
		if err == nil {
			return OpenReader(io.NopCloser(bytes.NewReader(data)))
		}
		last = err
		if !retry {
			break
		}
	}
	return nil, last
}

func download(ctx context.Context, client *http.Client, raw string, limit int64, timeout time.Duration) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	req.Header.Set("Accept", "audio/mpeg")
	resp, err := client.Do(req)
	if err != nil {
		return nil, true, errors.New("fallo de transporte o tiempo de espera de audio HTTPS")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode == 408 || resp.StatusCode == 429 || resp.StatusCode >= 500, fmt.Errorf("descarga de audio: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > limit {
		return nil, false, errors.New("audio excede el limite de memoria")
	}
	// Allocate at most limit+1 bytes, without growth allocations or a second copy.
	data := make([]byte, limit+1)
	n := 0
	for n < len(data) {
		read, readErr := resp.Body.Read(data[n:])
		n += read
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, true, errors.New("lectura de audio interrumpida")
		}
	}
	if int64(n) > limit {
		return nil, false, errors.New("audio excede el limite de memoria")
	}
	if resp.ContentLength >= 0 && int64(n) != resp.ContentLength {
		return nil, true, errors.New("descarga de audio incompleta")
	}
	return data[:n], false, nil
}
