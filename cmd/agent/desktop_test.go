package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"music-engine/internal/control"
	"strings"
	"testing"
	"time"
)

func TestDesktopStopsOnCommandAndParentExit(t *testing.T) {
	for _, input := range []string{"stop\n", ""} {
		ctx, cancel := context.WithCancel(context.Background())
		cancelOnDesktopClose(strings.NewReader(input), cancel)
		if ctx.Err() == nil {
			t.Fatal("desktop close did not stop engine")
		}
	}
}

func TestDesktopWaitsUntilStop(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cancelOnDesktopClose(reader, cancel)
	if _, err := writer.Write([]byte("ignored\n")); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil {
		t.Fatal("unrecognized command stopped engine")
	}
	writer.Close()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("EOF did not stop engine")
	}
}

func TestDesktopObserverWritesJSON(t *testing.T) {
	var buffer bytes.Buffer
	observe := desktopObserver(&buffer)
	observe(control.DesktopEvent{Type: "connection", Channel: "sync", Connected: true})
	var event control.DesktopEvent
	if err := json.Unmarshal(buffer.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "connection" || !event.Connected {
		t.Fatal("incorrect desktop event")
	}
}
