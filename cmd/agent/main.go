// agent is the distributable Windows audio engine.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"music-engine/internal/audio/asio"
	"music-engine/internal/audio/backend"
	"music-engine/internal/control"
	"music-engine/internal/localconfig"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

var version = "0.8.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	command := "run"
	if len(args) > 0 {
		command, args = args[0], args[1:]
	}
	if command == "desktop-info" || command == "desktop-configure" {
		return desktopCommand(command, args, os.Stdin, os.Stdout)
	}
	desktop := command == "run" && len(args) == 1 && args[0] == "--desktop"
	if desktop {
		args = nil
	}
	if command == "help" || command == "--help" || command == "-h" {
		fmt.Println("UtrackSound " + version + " (Windows x64)\nengine.exe configure --backend oto|asio --server https://servidor\nengine.exe configure --backend oto --help\nengine.exe configure --backend asio --help\nengine.exe backend oto|asio (requiere cerrar el motor; conserva configuracion)\nLa credencial se solicita oculta; no se acepta como argumento.\nengine.exe drivers (solo ASIO)\nengine.exe run (opcion predeterminada)\nengine.exe version\nConfiguracion, diario y logs: %ProgramData%\\UtrackSound")
		return nil
	}
	if command == "version" && len(args) == 0 {
		fmt.Println(version)
		return nil
	}
	if command == "drivers" && len(args) == 0 {
		names, err := asio.AvailableDrivers()
		if err != nil {
			return err
		}
		if len(names) == 0 {
			return errors.New("no hay drivers ASIO x64 instalados")
		}
		for _, name := range names {
			fmt.Println(name)
		}
		return nil
	}
	if command != "configure" && command != "run" && command != "backend" {
		return errors.New("comando desconocido; use engine.exe --help")
	}
	if command == "run" && len(args) != 0 {
		return errors.New("run no acepta argumentos; use configure")
	}
	var cfg localconfig.Config
	if command == "configure" {
		var err error
		cfg, err = parseConfig(args, os.Stdout)
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	if command == "backend" && (len(args) != 1 || (args[0] != "oto" && args[0] != "asio")) {
		return errors.New("use engine.exe backend oto|asio con el motor detenido")
	}
	dir, err := localconfig.Directory()
	if err != nil {
		return err
	}
	if err := localconfig.Prepare(dir); err != nil {
		return err
	}
	lock, err := localconfig.Lock(dir)
	if err != nil {
		return err
	}
	defer lock.Close()
	if command == "backend" {
		cfg, token, err := localconfig.Load(dir)
		if err != nil {
			return err
		}
		cfg.Backend = args[0]
		if err := localconfig.Save(dir, cfg, token); err != nil {
			return err
		}
		fmt.Println(backendNotice(cfg.BackendName()))
		fmt.Println("Backend guardado. Inicie un nuevo proceso con engine.exe run.")
		return nil
	}
	if command == "configure" {
		fmt.Print("Credencial del equipo (oculta): ")
		token, err := localconfig.ReadSecret()
		fmt.Println()
		if err != nil {
			return err
		}
		if err := localconfig.Save(dir, cfg, token); err != nil {
			return err
		}
		fmt.Println("Configuracion guardada. Ejecute engine.exe run con este mismo usuario de Windows.")
		return nil
	}
	cfg, token, err := localconfig.Load(dir)
	if err != nil {
		return err
	}
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(logDir, time.Now().Format("2006-01-02")+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("abrir log: %w", err)
	}
	defer f.Close()
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	defer log.SetOutput(os.Stderr)
	log.Printf("UtrackSound %s; %s", version, backendNotice(cfg.BackendName()))
	if cfg.BackendName() == "asio" {
		log.Printf("driver=%q frecuencia=%d buffer=%d", cfg.Driver, cfg.SampleRate, cfg.Buffer)
		names, err := asio.AvailableDrivers()
		if err != nil {
			return err
		}
		found := false
		for _, name := range names {
			if name == cfg.Driver {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("driver ASIO %q no encontrado; disponibles: %v; no se cambiara a Oto", cfg.Driver, names)
		}
	}
	output, err := backend.New(cfg.BackendName(), cfg.Audio())
	if err != nil {
		log.Printf("Error: %v", err)
		return err
	}
	if closer, ok := output.(interface{ Close() error }); ok {
		defer closer.Close()
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	var observer func(control.DesktopEvent)
	if desktop {
		observer = desktopObserver(os.Stdout)
		go cancelOnDesktopClose(os.Stdin, cancel)
	}
	profile := control.PlaybackProfile
	if cfg.Mode == "mono" {
		profile = control.MonoProfile
	}
	err = control.RunProfileObserved(ctx, cfg.Server, token, version, filepath.Join(dir, "journal"), profile, output, observer)
	if err != nil {
		log.Printf("Error: %v", err)
	}
	return err
}

func backendNotice(name string) string {
	if name == "oto" {
		return "Oto: todas las zonas se mezclan en la salida estereo del sistema; sin canales fisicos independientes. No configure rutas 3-4, 5-6, etc. Audio MP3 44100 Hz. Para cambiar de backend cierre el proceso con Ctrl+C y reinicie."
	}
	return "ASIO: driver seleccionado y canales fisicos segun hardware; sin fallback. Para cambiar de backend cierre el proceso con Ctrl+C y reinicie."
}

func parseConfig(args []string, out io.Writer) (localconfig.Config, error) {
	cfg := localconfig.Config{Backend: "asio"}
	for i, arg := range args {
		if strings.HasPrefix(arg, "--backend=") {
			cfg.Backend = strings.TrimPrefix(arg, "--backend=")
		}
		if strings.HasPrefix(arg, "-backend=") {
			cfg.Backend = strings.TrimPrefix(arg, "-backend=")
		}
		if (arg == "--backend" || arg == "-backend") && i+1 < len(args) {
			cfg.Backend = args[i+1]
		}
	}
	if cfg.Backend != "oto" && cfg.Backend != "asio" {
		return cfg, errors.New("backend debe ser oto o asio")
	}
	fmt.Fprintln(out, backendNotice(cfg.Backend))
	fs := flag.NewFlagSet("configure", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.StringVar(&cfg.Backend, "backend", cfg.Backend, "oto o asio (ASIO si se omite)")
	fs.StringVar(&cfg.Server, "server", "", "URL HTTPS del servidor")
	fs.StringVar(&cfg.Mode, "mode", "stereo", "stereo o mono")
	if cfg.Backend == "asio" {
		fs.StringVar(&cfg.Driver, "driver", "", "nombre exacto del driver ASIO x64")
		fs.IntVar(&cfg.SampleRate, "sample-rate", 48000, "frecuencia ASIO en Hz")
		fs.IntVar(&cfg.Buffer, "buffer", 256, "buffer ASIO en frames")
		fs.IntVar(&cfg.Channels, "channels", 0, "canales ASIO; 0 = automatico")
	}
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if fs.NArg() != 0 {
		return cfg, errors.New("argumentos adicionales no admitidos")
	}
	return cfg, cfg.Validate()
}
