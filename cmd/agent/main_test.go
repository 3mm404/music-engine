package main

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"
)

func TestBackendSpecificConfiguration(t *testing.T) {
	var out bytes.Buffer
	cfg, err := parseConfig([]string{"--backend", "oto", "--server", "https://example.test"}, &out)
	if err != nil || cfg.BackendName() != "oto" || cfg.Driver != "" || cfg.SampleRate != 0 {
		t.Fatalf("Oto: %+v %v", cfg, err)
	}
	out.Reset()
	_, err = parseConfig([]string{"--backend", "oto", "--help"}, &out)
	if !errors.Is(err, flag.ErrHelp) || strings.Contains(out.String(), "-driver") || strings.Contains(out.String(), "-buffer") || strings.Contains(out.String(), "-sample-rate") {
		t.Fatalf("Oto help: %s %v", out.String(), err)
	}
	if _, err = parseConfig([]string{"--backend", "oto", "--driver", "bad", "--server", "https://example.test"}, &out); err == nil {
		t.Fatal("ASIO flag accepted for Oto")
	}
	cfg, err = parseConfig([]string{"--backend", "asio", "--server", "https://example.test", "--driver", "Test"}, &out)
	if err != nil || cfg.SampleRate != 48000 || cfg.Buffer != 256 {
		t.Fatalf("ASIO: %+v %v", cfg, err)
	}
	if _, err = parseConfig([]string{"--backend", "invalid"}, &out); err == nil {
		t.Fatal("invalid backend accepted")
	}
}
