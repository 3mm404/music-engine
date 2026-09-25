// Package console implementa los comandos interactivos del demo.
package console

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"music-engine/internal/player"
)

const help = "Comandos: play [ruta] | pause | resume | next | previous | stop | volume 0..100 | state | help | quit"

// Run conserva la consola abierta al terminar una pista. EOF, quit o Ctrl+C salen.
func Run(ctx context.Context, p *player.Player, input io.Reader, output io.Writer) error {
	lines := make(chan string)
	scanErrors := make(chan error, 1)
	done := make(chan struct{})
	defer close(done)
	defer p.Stop()
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
	fmt.Fprint(output, "> ")
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Detecta EOF y errores aunque el usuario no escriba un comando.
			before := p.GetState()
			if before.Error != nil {
				return before.Error
			}
		case line, ok := <-lines:
			if !ok {
				return <-scanErrors
			}
			quit, err := execute(p, line, output)
			if err != nil {
				fmt.Fprintln(output, "Error:", err)
			}
			if quit {
				return nil
			}
			fmt.Fprint(output, "> ")
		}
	}
}

func execute(p *player.Player, line string, output io.Writer) (bool, error) {
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
		if path == "" {
			path = p.GetState().Track
		}
		if path == "" {
			return false, fmt.Errorf("usa play seguido de una ruta MP3")
		}
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
}
