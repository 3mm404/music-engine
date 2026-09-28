package control

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"music-engine/internal/audio"
	"music-engine/internal/decoder"
	"music-engine/internal/engine"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type simulatedOutput struct {
	shared bool
	closed atomic.Bool
}

func (o *simulatedOutput) SharedStereo() bool { return o.shared }
func (o *simulatedOutput) Close() error       { o.closed.Store(true); return nil }
func (o *simulatedOutput) Open(source io.Reader, f audio.PCMFormat) (audio.Stream, error) {
	// Decode/read actual PCM, but do not open an operating-system audio device.
	b := make([]byte, 4)
	if _, err := io.ReadFull(source, b); err != nil {
		return nil, err
	}
	return &simulatedStream{}, nil
}

type simulatedStream struct{ playing atomic.Bool }

func (s *simulatedStream) Play()             { s.playing.Store(true) }
func (s *simulatedStream) Pause()            { s.playing.Store(false) }
func (s *simulatedStream) IsPlaying() bool   { return s.playing.Load() }
func (s *simulatedStream) SetVolume(float64) {}
func (s *simulatedStream) Err() error        { return nil }

func TestBackendTransportWithoutHardware(t *testing.T) {
	for _, shared := range []bool{true, false} {
		name := "asio-contract"
		if shared {
			name = "oto-contract"
		}
		t.Run(name, func(t *testing.T) {
			frame := make([]byte, 417)
			copy(frame, []byte{0xff, 0xfb, 0x90, 0x00})
			data := bytes.Repeat(frame, 20)
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				switch req.URL.Path {
				case "/api/v1/engine/config":
					json.NewEncoder(w).Encode(map[string]any{"data": audioConfig(server.URL+"/audio", data)})
				case "/api/v1/engine/commands":
					json.NewEncoder(w).Encode(map[string]any{"data": []Command{}})
				case "/audio":
					w.Write(data)
				default:
					w.Write([]byte(`{"data":{}}`))
				}
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-token")
			if err != nil {
				t.Fatal(err)
			}
			client.http = server.Client()
			client.profile = PlaybackProfile
			output := &simulatedOutput{shared: shared}
			manager, err := engine.NewRemoteWithOutput(output, decoder.HTTPS{Client: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			defer manager.Close()
			journal, err := OpenJournal(t.TempDir(), server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer journal.Close()
			r := &Runtime{client: client, audio: manager, journal: journal, deviceID: "1", sharedStereo: shared}
			if err := r.cycle(context.Background()); err != nil {
				t.Fatal(err)
			}
			result := r.execute(Command{ZoneID: "10", Revision: 1, Action: "play", ExpiresAt: time.Now().Add(time.Minute)})
			if result.Outcome != "pending" {
				t.Fatalf("play: %+v", result)
			}
			awaitAudio(t, r, "playing")
			for _, step := range []struct{ action, state string }{{"pause", "paused"}, {"resume", "playing"}, {"stop", "stopped"}} {
				result = r.execute(Command{ZoneID: "10", Revision: 1, Action: step.action, ExpiresAt: time.Now().Add(time.Minute)})
				if result.Outcome != "succeeded" {
					t.Fatalf("%s: %+v", step.action, result)
				}
				awaitAudio(t, r, step.state)
			}
			if err := manager.Close(); err != nil {
				t.Fatal(err)
			}
			if !output.closed.Load() {
				t.Fatal("backend was not closed")
			}
		})
	}
}
