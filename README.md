# Go Music Engine para Windows

Demo de reproducción local: abre un MP3, lo decodifica progresivamente y reproduce hasta el final. Oto se encarga del sistema de audio; nuestro código no utiliza WASAPI directamente.

```text
MP3 → Decoder → PCM → Player → Oto v3 → Windows Audio → Bocinas
```

## Ejecutar

Requiere Windows y Go 1.26 o posterior. Desde PowerShell:

```powershell
cd C:\Users\perez\Desktop\dev\music-engine
go mod download
```

Coloca tu archivo en **music/demo.mp3**. No se distribuye música de terceros con el proyecto. Ejecuta desde la raíz:

```powershell
go run ./cmd/engine
```

También puedes indicar otra ruta:

```powershell
go run ./cmd/engine -file "C:\ruta\cancion.mp3"
```

Ctrl+C detiene la reproducción y libera el archivo. Al terminar normalmente se muestra `Reproduccion finalizada.`. Para generar el ejecutable:

```powershell
go build -o bin/music-engine.exe ./cmd/engine
.\bin\music-engine.exe
```

## Organización

- `cmd/engine/main.go`: configura la ruta y Ctrl+C, crea el Player e inicia la prueba.
- `internal/decoder/mp3.go`: go-mp3 transforma MP3 a PCM signed int16 little-endian, estéreo, a la frecuencia del archivo. No conoce Oto.
- `internal/player/player.go`: controla Play, Pause, Resume, Stop y Wait; administra el archivo y la sincronización.
- `internal/player/oto.go`: encapsula el contexto y el reproductor Oto v3, el formato PCM y los errores de salida.
- `music/`: coloca aquí demo.mp3; los MP3 se excluyen de Git.

Dependencias directas: [Oto v3](https://pkg.go.dev/github.com/ebitengine/oto/v3) y [go-mp3](https://pkg.go.dev/github.com/hajimehoshi/go-mp3). go.mod/go.sum fijan las versiones. Oto recibe PCM; no decodifica MP3.

## Contrato del Player

| Método | Comportamiento |
|---|---|
| `Play(path) error` | Abre e inicia en segundo plano; reemplaza la pista anterior si el nuevo archivo y la salida se preparan correctamente. |
| `Pause() error` | Pausa y conserva posición y datos pendientes. |
| `Resume() error` | Continúa una pista pausada. |
| `Stop() error` | Detiene y cierra el archivo; repetirlo es válido. Para volver al inicio usa Play. |
| `Wait(ctx) error` | Mantiene el proceso vivo hasta finalizar o detenerse; cancelar el contexto detiene la reproducción. |

Pause/Resume sin pista devuelven ErrNoTrack. El CLI solo expone reproducción y Ctrl+C; los cuatro controles están implementados para usarlos desde Go. Los errores durante reproducción se devuelven mediante Wait. El archivo se cierra únicamente después de que Oto termina cualquier lectura pendiente.

## Límites de este demo

Oto permite un solo contexto por proceso: se crea con la frecuencia del primer MP3 y se reutiliza. Otro MP3 con distinta frecuencia devuelve un error claro; reinicia el proceso para reproducirlo. No hay resampler propio ni selector de dispositivos. Utiliza la salida configurada en Windows antes de iniciar.

Wait deja 100 ms al final para la cola del dispositivo; es un margen práctico, no una confirmación exacta del instante acústico. Pause/Stop pueden dejar sonar brevemente audio ya enviado al dispositivo.

Más adelante Windows Audio se direccionará hacia Dante Virtual Soundcard y Q-SYS. Esta versión no implementa esa integración ni APIs, scheduler o zonas. `docs/WASAPI-LAB-HISTORICO.md` conserva únicamente las notas del laboratorio anterior.

## Verificación

```powershell
go fmt ./...
go mod tidy
go test ./...
go vet ./...
go build -o bin/music-engine.exe ./cmd/engine
```

Prueba optativa de controles contra el dispositivo real (produce sonido):

```powershell
$env:MUSIC_ENGINE_TEST_MP3 = (Resolve-Path music/demo.mp3).Path
go test ./internal/player -run TestPlaybackIntegration -v -count=1
```

Sin esa variable, la prueba de audio se omite y las pruebas de errores sí se ejecutan. La validación automática no confirma que el usuario escuche las bocinas: revisa la salida y el volumen en Windows si no hay sonido.
