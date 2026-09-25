package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	base    string
	token   string
	session string
	http    *http.Client
}

type APIError struct {
	Status     int
	Code       string
	RetryAfter time.Duration
}

func (e *APIError) Error() string { return fmt.Sprintf("engine API: HTTP %d (%s)", e.Status, e.Code) }
func terminal(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.Status >= 400 && e.Status < 500 && e.Status != 429
}

func NewClient(server, token string) (*Client, error) {
	u, err := url.Parse(server)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
		return nil, errors.New("ENGINE_SERVER debe ser HTTPS (HTTP solo para localhost), sin credenciales ni query")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("falta ENGINE_TOKEN")
	}
	return &Client{base: strings.TrimRight(server, "/") + "/api/v1/engine", token: token,
		http: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (c *Client) request(ctx context.Context, method, path string, body, target any, envelope bool) error {
	var b bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&b).Encode(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, &b)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if c.session != "" {
		req.Header.Set("X-Engine-Session", c.session)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("engine API: fallo de conexión")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var data struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&data)
		seconds, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		return &APIError{resp.StatusCode, data.Error.Code, time.Duration(seconds) * time.Second}
	}
	if target == nil {
		return nil
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 8<<20))
	if !envelope {
		return dec.Decode(target)
	}
	var data struct {
		Data json.RawMessage `json:"data"`
	}
	if err = dec.Decode(&data); err != nil {
		return err
	}
	return json.Unmarshal(data.Data, target)
}

func (c *Client) Open(ctx context.Context, version string) (Session, error) {
	var s Session
	err := c.request(ctx, "POST", "/sessions", map[string]any{"engine_version": version, "capabilities": []string{"configuration_only"}}, &s, true)
	if err == nil {
		if s.DeviceID == "" || s.SessionID == "" || s.HeartbeatSeconds < 1 || s.PollSeconds < 1 {
			return s, errors.New("sesión inválida")
		}
		c.session = s.SessionID
	}
	return s, err
}

func (c *Client) Configuration(ctx context.Context) (Config, error) {
	var config Config
	err := c.request(ctx, "GET", "/config", nil, &config, true)
	return config, err
}
func (c *Client) Commands(ctx context.Context) ([]Command, error) {
	var commands []Command
	err := c.request(ctx, "GET", "/commands", nil, &commands, true)
	return commands, err
}
func (c *Client) Confirm(ctx context.Context, id string, result Result) error {
	return c.request(ctx, "PUT", "/commands/"+url.PathEscape(id)+"/result", result, nil, true)
}
func (c *Client) Heartbeat(ctx context.Context, report Report) error {
	return c.request(ctx, "POST", "/heartbeat", report, nil, true)
}
