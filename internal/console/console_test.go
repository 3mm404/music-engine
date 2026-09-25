package console

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"music-engine/internal/player"
)

func TestCommands(t *testing.T) {
	p := player.New()
	var output bytes.Buffer
	input := strings.NewReader("volume 25\nstate\nvolume NaN\nstop\nhelp\nquit\n")
	if err := Run(context.Background(), p, input, &output); err != nil {
		t.Fatal(err)
	}
	if p.GetState().Volume != 0.25 {
		t.Fatal("volumen incorrecto")
	}
	if !strings.Contains(output.String(), "STOPPED") || !strings.Contains(output.String(), "Error:") {
		t.Fatal(output.String())
	}
}

func TestEOF(t *testing.T) {
	if err := Run(context.Background(), player.New(), strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}
