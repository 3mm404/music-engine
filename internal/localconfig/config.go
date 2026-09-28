// Package localconfig stores settings and a user-bound encrypted credential.
package localconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"music-engine/internal/audio/asio"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Backend    string `json:"backend,omitempty"`
	Server     string `json:"server"`
	Driver     string `json:"asio_driver"`
	SampleRate int    `json:"sample_rate"`
	Buffer     int    `json:"buffer_size"`
	Channels   int    `json:"channels"`
	Mode       string `json:"mode"`
	Credential []byte `json:"credential_dpapi"`
}

func (c Config) BackendName() string {
	if c.Backend == "" {
		return "asio"
	}
	return c.Backend
}
func (c Config) Audio() asio.Config {
	return asio.Config{DriverName: c.Driver, SampleRate: c.SampleRate, BufferSize: c.Buffer, Channels: c.Channels}
}
func (c Config) Validate() error {
	u, err := url.Parse(c.Server)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("server debe ser una URL HTTPS sin credenciales, query ni fragmento")
	}
	if c.Mode != "mono" && c.Mode != "stereo" {
		return errors.New("mode debe ser mono o stereo")
	}
	if c.BackendName() == "oto" {
		return nil
	}
	if c.BackendName() != "asio" {
		return errors.New("backend debe ser oto o asio")
	}
	if strings.TrimSpace(c.Driver) == "" || c.SampleRate <= 0 || c.Buffer <= 0 {
		return errors.New("driver, frecuencia y buffer ASIO son obligatorios")
	}
	if c.Mode != "mono" && c.Mode != "stereo" {
		return errors.New("mode debe ser mono o stereo")
	}
	return c.Audio().Validate()
}
func Directory() (string, error) {
	dir := os.Getenv("ProgramData")
	if dir == "" || !filepath.IsAbs(dir) {
		return "", errors.New("ProgramData no contiene una ruta absoluta")
	}
	return filepath.Join(dir, "UtrackSound"), nil
}
func Save(dir string, c Config, token string) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(token) == "" {
		return errors.New("credencial vacia")
	}
	sealed, err := protect([]byte(token))
	if err != nil {
		return err
	}
	c.Credential = sealed
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "config-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return replace(f.Name(), filepath.Join(dir, "config.json"))
}
func Load(dir string) (Config, string, error) {
	var c Config
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		return c, "", fmt.Errorf("leer configuracion; ejecute engine.exe configure: %w", err)
	}
	if json.Unmarshal(data, &c) != nil {
		return c, "", errors.New("config.json invalido")
	}
	if err := c.Validate(); err != nil {
		return c, "", err
	}
	token, err := unprotect(c.Credential)
	if err != nil {
		return c, "", err
	}
	if strings.TrimSpace(string(token)) == "" {
		return c, "", errors.New("credencial vacia; ejecute configure")
	}
	return c, string(token), nil
}
