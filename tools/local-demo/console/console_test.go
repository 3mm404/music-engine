package console

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"music-engine/internal/engine"
)

func TestCommands(t *testing.T) {
	m, err := engine.New([]engine.ZoneConfig{{ID: "A"}, {ID: "B"}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	p, _ := m.Zone("A")
	var output bytes.Buffer
	input := strings.NewReader("volume 25\nstate\nvolume NaN\nstop\nzone B\nvolume 70\nzone missing\nvolume 60\nzones\nhelp\nquit\n")
	if err := Run(context.Background(), m, input, &output); err != nil {
		t.Fatal(err)
	}
	if p.GetState().Volume != 0.25 {
		t.Fatal("volumen incorrecto")
	}
	b, _ := m.Zone("B")
	if b.GetState().Volume != .6 {
		t.Fatal("zona incorrecta despues de ID invalido")
	}
	if err := b.SetVolume(.8); err != nil {
		t.Fatalf("consola cerro el manager: %v", err)
	}
	if !strings.Contains(output.String(), "STOPPED") || !strings.Contains(output.String(), "Error:") {
		t.Fatal(output.String())
	}
}

func TestEOF(t *testing.T) {
	m, err := engine.New([]engine.ZoneConfig{{ID: "A"}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := Run(context.Background(), m, strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}
