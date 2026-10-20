package control

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"
)

// wait permite interrumpir la espera al cerrar el proceso; false significa cancelación.
func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// delay limita la espera progresiva y respeta Retry-After para no saturar el servidor.
func delay(attempt int, err error) time.Duration {
	d := time.Second * time.Duration(1<<min(attempt, 5))
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	d += time.Duration(rand.Float64() * 0.2 * float64(d))
	var api *APIError
	if errors.As(err, &api) && api.RetryAfter > d {
		d = api.RetryAfter
	}
	return d
}
