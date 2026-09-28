//go:build windows

package main

import (
	"music-engine/internal/localconfig"
	"os"
	"path/filepath"
	"testing"
)

func TestBackendChangeRequiresStoppedEngine(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ProgramData", root)
	dir := filepath.Join(root, "UtrackSound")
	if err := localconfig.Prepare(dir); err != nil {
		t.Fatal(err)
	}
	cfg := localconfig.Config{Server: "https://example.test", Driver: "Test ASIO", SampleRate: 48000, Buffer: 256, Mode: "stereo"}
	if err := localconfig.Save(dir, cfg, "test-token"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "config.json"))
	lock, err := localconfig.Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"backend", "oto"}); err == nil {
		t.Fatal("changed running backend")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "config.json"))
	if string(after) != string(before) {
		t.Fatal("modified locked config")
	}
	lock.Close()
	for _, name := range []string{"oto", "asio"} {
		if err := run([]string{"backend", name}); err != nil {
			t.Fatal(err)
		}
		got, token, err := localconfig.Load(dir)
		if err != nil || got.BackendName() != name || token != "test-token" || got.Driver != "Test ASIO" {
			t.Fatalf("switch: %+v %v", got, err)
		}
	}
}
