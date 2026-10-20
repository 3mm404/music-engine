// Package console implementa los comandos interactivos del demo.
package console

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"music-engine/internal/engine"
	"music-engine/internal/player"
)

const help = "Comandos: zone ID | zones | play [ruta] | pause | resume | next | previous | stop | volume 0..100 | state | help | quit"

// Run conserva la consola abierta al terminar una pista. EOF, quit o Ctrl+C salen.
// El llamador posee el Manager: salir de la consola no detiene las zonas.
func Run(ctx context.Context, manager *engine.Manager, input io.Reader, output io.Writer) error {
	ids := manager.IDs()
	if len(ids) == 0 {
		return fmt.Errorf("no hay zonas configuradas")
	}
	selected := ids[0]
	p, _ := manager.Zone(selected)
	lines := make(chan string)
	scanErrors := make(chan error, 1)
	done := make(chan struct{})
	defer close(done)
	go func() {
		scanner := bufio.NewScanner(input)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-done:
				return
			}
		}
		scanErrors <- scanner.Err()
		close(lines)
	}()
	fmt.Fprintln(output, help)
	printState(output, p.GetState())
	fmt.Fprintf(output, "[%s]> ", selected)
	for {
		select {
		case <-ctx.Done():
			return nil
		case line, ok := <-lines:
			if !ok {
				return <-scanErrors
			}
			parts := strings.SplitN(strings.TrimSpace(line), " ", 2)
			if strings.EqualFold(parts[0], "zone") {
				id := ""
				if len(parts) == 2 {
					id = strings.TrimSpace(parts[1])
				}
				zone, err := manager.Zone(id)
				if err != nil {
					fmt.Fprintln(output, "Error:", err)
				} else {
					selected, p = id, zone
					printState(output, p.GetState())
				}
				fmt.Fprintf(output, "[%s]> ", selected)
				continue
			}
			if strings.EqualFold(strings.TrimSpace(line), "zones") {
				for _, id := range ids {
					zone, _ := manager.Zone(id)
					fmt.Fprintf(output, "%s: ", id)
					printState(output, zone.GetState())
				}
				fmt.Fprintf(output, "[%s]> ", selected)
				continue
			}
			quit, err := execute(p, line, output)
			if err != nil {
				fmt.Fprintln(output, "Error:", err)
			}
			if quit {
				return nil
			}
			fmt.Fprintf(output, "[%s]> ", selected)
		}
	}
}

func execute(p *engine.Zone, line string, output io.Writer) (bool, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return false, nil
	}
	parts := strings.SplitN(line, " ", 2)
	command := strings.ToLower(parts[0])
	arg := ""
	if len(parts) == 2 {
		arg = strings.TrimSpace(parts[1])
	}
	if arg != "" && command != "play" && command != "volume" {
		return false, fmt.Errorf("%s no recibe argumentos", command)
	}
	var err error
	switch command {
	case "play":
		path := strings.Trim(arg, "\"")
		err = p.Play(path)
	case "pause":
		err = p.Pause()
	case "resume":
		err = p.Resume()
	case "next":
		err = p.Next()
	case "previous", "prev":
		err = p.Previous()
	case "stop":
		err = p.Stop()
	case "volume":
		var value float64
		value, err = strconv.ParseFloat(arg, 64)
		if err != nil {
			return false, fmt.Errorf("usa volume seguido de un numero entre 0 y 100")
		}
		err = p.SetVolume(value / 100)
	case "state":
	case "help":
		fmt.Fprintln(output, help)
		return false, nil
	case "quit", "exit":
		return true, nil
	default:
		return false, fmt.Errorf("comando desconocido; escribe help")
	}
	if err != nil {
		return false, err
	}
	printState(output, p.GetState())
	return false, nil
}

func printState(output io.Writer, state player.State) {
	fmt.Fprintf(output, "%s | pista %d/%d | volumen %.0f%% | %s\n", state.Status, state.Index+1, state.Total, state.Volume*100, state.Track)
	if state.Error != nil {
		fmt.Fprintln(output, "Error de zona:", state.Error)
	}
}
