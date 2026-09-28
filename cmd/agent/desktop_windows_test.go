package main

import (
	"bytes"
	"encoding/json"
	"music-engine/internal/localconfig"
	"strings"
	"testing"
)

func TestDesktopConfigurationKeepsCredentialPrivateAndReusable(t *testing.T) {
	t.Setenv("ProgramData", t.TempDir())
	var result bytes.Buffer
	request := `{"settings":{"backend":"oto","server":"https://example.test","mode":"stereo"},"token":"private-test-token"}`
	if err := desktopCommand("desktop-configure", nil, strings.NewReader(request), &result); err != nil {
		t.Fatal(err)
	}
	result.Reset()
	if err := desktopCommand("desktop-info", nil, strings.NewReader(""), &result); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.String(), "private-test-token") || strings.Contains(result.String(), "credential_dpapi") {
		t.Fatal("credential leaked")
	}
	var info map[string]any
	if err := json.Unmarshal(result.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info["configured"] != true || info["protocol"] != float64(1) {
		t.Fatal("missing configuration")
	}
	request = `{"settings":{"backend":"oto","server":"https://example.test","mode":"mono"},"token":""}`
	if err := desktopCommand("desktop-configure", nil, strings.NewReader(request), &result); err != nil {
		t.Fatal(err)
	}
	dir, _ := localconfig.Directory()
	cfg, token, err := localconfig.Load(dir)
	if err != nil || token != "private-test-token" || cfg.Mode != "mono" {
		t.Fatalf("credential not preserved: %v", err)
	}
	lock, err := localconfig.Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := desktopCommand("desktop-configure", nil, strings.NewReader(request), &result); err == nil {
		t.Fatal("configuration changed while engine running")
	}
	result.Reset()
	if err := desktopCommand("desktop-info", nil, strings.NewReader(""), &result); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(result.Bytes(), &info); err != nil || info["running"] != true {
		t.Fatal("running engine not detected")
	}
}
