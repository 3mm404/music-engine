package control

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type frame struct {
	Event   string          `json:"event"`
	Channel string          `json:"channel,omitempty"`
	Data    json.RawMessage `json:"data"`
}

func dataObject(data json.RawMessage, target any) error {
	var s string
	if json.Unmarshal(data, &s) == nil {
		data = []byte(s)
	}
	return json.Unmarshal(data, target)
}

func (c *Client) watch(ctx context.Context, config WebSocketConfig, wake chan<- struct{}, fatal chan<- error) {
	for attempt := 0; ctx.Err() == nil; attempt++ {
		started := time.Now()
		err := c.subscribe(ctx, config, wake)
		if time.Since(started) > 30*time.Second {
			attempt = 0
		}
		if terminal(err) {
			select {
			case fatal <- err:
			case <-ctx.Done():
			}
			return
		}
		if !wait(ctx, delay(attempt, err)) {
			return
		}
	}
}

func (c *Client) subscribe(ctx context.Context, config WebSocketConfig, wake chan<- struct{}) error {
	u, err := url.Parse(config.URL)
	if err != nil || u.Host == "" || u.User != nil || config.Key == "" || config.Channel == "" || (u.Scheme != "wss" && !(u.Scheme == "ws" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
		return &APIError{Status: 422, Code: "invalid_websocket_configuration"}
	}
	u.Path = "/app/" + config.Key
	u.RawQuery = "protocol=7&client=music-engine&version=0.2.0&flash=false"
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return errors.New("WebSocket no disponible")
	}
	defer conn.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()
	conn.SetReadLimit(1 << 20)
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	var first frame
	if err = conn.ReadJSON(&first); err != nil {
		return err
	}
	if first.Event != "pusher:connection_established" {
		return errors.New("handshake Reverb inválido")
	}
	var hello struct {
		SocketID        string `json:"socket_id"`
		ActivityTimeout int    `json:"activity_timeout"`
	}
	if err = dataObject(first.Data, &hello); err != nil || hello.SocketID == "" {
		return errors.New("socket_id ausente")
	}
	var auth struct {
		Auth string `json:"auth"`
	}
	err = c.request(ctx, "POST", "/broadcasting/auth", map[string]string{"socket_id": hello.SocketID, "channel_name": config.Channel}, &auth, false)
	if err != nil {
		return err
	}
	var writeMu sync.Mutex
	write := func(event string, data any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteJSON(map[string]any{"event": event, "data": data})
	}
	if err = write("pusher:subscribe", map[string]string{"auth": auth.Auth, "channel": config.Channel}); err != nil {
		return err
	}
	timeout := time.Duration(max(hello.ActivityTimeout, 30)) * time.Second
	// Pusher's activity timeout requires the client to ping on an idle socket.
	// Serialize writes with protocol pong and subscribe messages.
	go func() {
		ticker := time.NewTicker(timeout / 2)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				if write("pusher:ping", map[string]any{}) != nil {
					conn.Close()
					return
				}
			}
		}
	}()
	for {
		_ = conn.SetReadDeadline(time.Now().Add(timeout + 15*time.Second))
		var f frame
		if err = conn.ReadJSON(&f); err != nil {
			return err
		}
		switch f.Event {
		case "pusher:ping":
			if err = write("pusher:pong", map[string]any{}); err != nil {
				return err
			}
		case "pusher:error", "pusher:subscription_error":
			return errors.New("Reverb rechazó la suscripción")
		case "pusher_internal:subscription_succeeded", "engine.changed":
			if f.Channel == config.Channel {
				select {
				case wake <- struct{}{}:
				default:
				}
			}
		}
	}
}
