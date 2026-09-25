package player

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

const outputBuffer = 50 * time.Millisecond

// Oto permite un unico contexto por proceso. Se reutiliza entre reproducciones.
var output struct {
	sync.Mutex
	context   *oto.Context
	rate      int
	attempted bool
	err       error
}

type audioOutput struct {
	context *oto.Context
	player  *oto.Player
}

func openOutput(source io.Reader, rate int) (*audioOutput, error) {
	output.Lock()
	defer output.Unlock()
	if !output.attempted {
		output.attempted = true
		var ready chan struct{}
		output.context, ready, output.err = oto.NewContext(&oto.NewContextOptions{
			SampleRate:   rate,
			ChannelCount: 2,
			Format:       oto.FormatSignedInt16LE,
			BufferSize:   outputBuffer,
		})
		if output.err == nil {
			<-ready
			output.err = output.context.Err()
		}
		output.rate = rate
	}
	if output.err != nil {
		return nil, fmt.Errorf("inicializar Oto: %w", output.err)
	}
	if output.rate != rate {
		return nil, fmt.Errorf("el contexto usa %d Hz y el archivo %d Hz; reinicia el programa para cambiar de frecuencia", output.rate, rate)
	}
	if err := output.context.Err(); err != nil {
		return nil, fmt.Errorf("salida de audio: %w", err)
	}
	return &audioOutput{context: output.context, player: output.context.NewPlayer(source)}, nil
}

func (o *audioOutput) play()         { o.player.Play() }
func (o *audioOutput) pause()        { o.player.PauseAndStopReading() }
func (o *audioOutput) playing() bool { return o.player.IsPlaying() }
func (o *audioOutput) err() error {
	if err := o.context.Err(); err != nil {
		return fmt.Errorf("salida de audio: %w", err)
	}
	return o.player.Err()
}
