package backend

import (
	"testing"

	"music-engine/internal/audio/asio"
	"music-engine/internal/audio/oto"
)

func TestFromEnvironmentDefaultsToASIOAndRejectsInvalidBackend(t *testing.T) {
	t.Setenv("ENGINE_AUDIO_BACKEND", "")
	t.Setenv("ENGINE_ASIO_DRIVER", "Test ASIO")
	output, err := FromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := output.(*asio.Output); !ok {
		t.Fatalf("default backend is %T, want ASIO", output)
	}
	t.Setenv("ENGINE_AUDIO_BACKEND", "alsa")
	if _, err := FromEnvironment(); err == nil {
		t.Fatal("unknown backend accepted")
	}
}

func TestMissingASIODriverDoesNotFallBackToOto(t *testing.T) {
	t.Setenv("ENGINE_AUDIO_BACKEND", "")
	t.Setenv("ENGINE_ASIO_DRIVER", "")
	if output, err := FromEnvironment(); err == nil || output != nil {
		t.Fatalf("missing driver must fail, got %T, %v", output, err)
	}
	t.Setenv("ENGINE_AUDIO_BACKEND", "oto")
	output, err := FromEnvironment()
	if _, ok := output.(*oto.Output); err != nil || !ok {
		t.Fatalf("explicit compatibility backend: %T, %v", output, err)
	}
}

func TestFromEnvironmentBuildsASIOConfiguration(t *testing.T) {
	t.Setenv("ENGINE_AUDIO_BACKEND", "asio")
	t.Setenv("ENGINE_ASIO_DRIVER", "Test ASIO")
	t.Setenv("ENGINE_ASIO_SAMPLE_RATE", "48000")
	t.Setenv("ENGINE_ASIO_BUFFER_SIZE", "256")
	t.Setenv("ENGINE_ASIO_CHANNELS", "8")
	output, err := FromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := output.(*asio.Output); !ok {
		t.Fatalf("backend is %T, want ASIO", output)
	}
}

func TestFromEnvironmentRejectsInvalidASIOValues(t *testing.T) {
	t.Setenv("ENGINE_AUDIO_BACKEND", "asio")
	t.Setenv("ENGINE_ASIO_DRIVER", "Test ASIO")
	t.Setenv("ENGINE_ASIO_SAMPLE_RATE", "not-a-rate")
	if _, err := FromEnvironment(); err == nil {
		t.Fatal("invalid sample rate accepted")
	}
	t.Setenv("ENGINE_ASIO_SAMPLE_RATE", "48000")
	t.Setenv("ENGINE_ASIO_CHANNELS", "65")
	if _, err := FromEnvironment(); err == nil {
		t.Fatal("invalid channel count accepted")
	}
}
