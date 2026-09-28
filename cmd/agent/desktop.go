package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"music-engine/internal/audio/asio"
	"music-engine/internal/control"
	"music-engine/internal/localconfig"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Only public settings cross the desktop pipe. DPAPI ciphertext stays in Go.
type desktopSettings struct {
	Backend    string `json:"backend"`
	Server     string `json:"server"`
	Driver     string `json:"asio_driver"`
	SampleRate int    `json:"sample_rate"`
	Buffer     int    `json:"buffer_size"`
	Channels   int    `json:"channels"`
	Mode       string `json:"mode"`
}

func publicSettings(c localconfig.Config) desktopSettings {
	return desktopSettings{c.BackendName(), c.Server, c.Driver, c.SampleRate, c.Buffer, c.Channels, c.Mode}
}

func desktopCommand(command string, args []string, input io.Reader, output io.Writer) error {
	if len(args) != 0 {
		return errors.New("el puente no acepta argumentos adicionales")
	}
	dir, err := localconfig.Directory()
	if err != nil {
		return err
	}
	if err := localconfig.Prepare(dir); err != nil {
		return err
	}
	if command == "desktop-configure" {
		lock, err := localconfig.Lock(dir)
		if err != nil {
			return err
		}
		defer lock.Close()
		var request struct {
			Settings desktopSettings `json:"settings"`
			Token    string          `json:"token"`
		}
		decoder := json.NewDecoder(io.LimitReader(input, 65537))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			return errors.New("configuracion del puente invalida")
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return errors.New("se esperaba una sola configuracion")
		}
		s := request.Settings
		cfg := localconfig.Config{Backend: s.Backend, Server: strings.TrimSpace(s.Server), Driver: s.Driver, SampleRate: s.SampleRate, Buffer: s.Buffer, Channels: s.Channels, Mode: s.Mode}
		if strings.TrimSpace(request.Token) == "" {
			_, token, err := localconfig.Load(dir)
			if err != nil {
				return errors.New("ingrese el token del equipo para guardar la configuracion")
			}
			request.Token = token
		}
		if err := localconfig.Save(dir, cfg, request.Token); err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(map[string]any{"saved": true})
	}
	info := map[string]any{"protocol": 1, "version": version, "data_directory": dir, "configured": false, "running": false, "drivers": []string{}}
	lock, lockErr := localconfig.Lock(dir)
	if lockErr == nil {
		lock.Close()
	} else {
		info["running"] = true
	}
	if names, err := asio.AvailableDrivers(); err == nil && names != nil {
		info["drivers"] = names
	} else if err != nil {
		info["driver_error"] = err.Error()
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err == nil {
		cfg, _, err := localconfig.Load(dir)
		if err != nil {
			info["configuration_error"] = err.Error()
		} else {
			info["configured"] = true
			info["settings"] = publicSettings(cfg)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return json.NewEncoder(output).Encode(info)
}

func desktopObserver(output io.Writer) func(control.DesktopEvent) {
	var mu sync.Mutex
	encoder := json.NewEncoder(output)
	return func(event control.DesktopEvent) {
		mu.Lock()
		defer mu.Unlock()
		_ = encoder.Encode(event)
	}
}

// EOF also stops audio if the desktop is closed or crashes.
func cancelOnDesktopClose(input io.Reader, cancel context.CancelFunc) {
	defer cancel()
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "stop" {
			return
		}
	}
}
