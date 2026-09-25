# Go Music Engine: controles de consola

## Integración con Laravel

El [contrato de la API v1](../utrack-fly/objetivos/01-contrato-laravel-engine-api-v1.md) define equipos, credenciales, configuración de zonas, canciones, órdenes y estado observado. La fuente de verdad está en `objetivos/` del proyecto Laravel (el enlace supone ambos repositorios en la misma carpeta).

El nuevo `cmd/agent` implementa conexión HTTP, heartbeat, sincronización por Reverb y deduplicación durable de órdenes, en modo **configuración sin audio**. Configura `ENGINE_SERVER` (URL base de Laravel), `ENGINE_TOKEN` y `ENGINE_STATE_DIR`, y ejecuta `go run ./cmd/agent`. Consulta [Objetivo 02](../utrack-fly/objetivos/02-conexion-y-sincronizacion-del-engine.md) para la puesta en marcha, alcance y cierre verificado con Laravel, Reverb y Go reales. El programa de consola descrito a continuación se conserva separado.

Player local y modular para Windows:

```text
MP3 → Decoder (go-mp3) → PCM → Player → Oto v3 → Windows Audio
```

## Ejecutar

Requiere Windows y Go 1.26 o posterior. Se conserva la ruta absoluta predeterminada que funciona en esta máquina.

```powershell
cd C:\Users\perez\Desktop\dev\music-engine
go mod download
go run ./cmd/engine
```

Empieza reproduciendo `music/demo.mp3`. Coloca más MP3 en esa carpeta para probar navegación. La lista se carga una vez al iniciar, ordenada por nombre. También puedes seleccionar un archivo inicial y su carpeta:

```powershell
go run ./cmd/engine -file "C:\Mi musica\cancion.mp3"
```

## Comandos

Escribe cada comando y presiona Enter:

```text
pause
resume
next
previous
volume 35
state
stop
play
quit
```

- `play`: reinicia desde el principio la pista seleccionada; `play "C:\Mi musica\otra.mp3"` abre otra ruta, incluso con espacios.
- `pause` / `resume`: conservan la posición. Sin pista cargada devuelven un error.
- `next` / `previous`: cambian e inician la pista; al llegar a un extremo vuelven al otro. Con un solo archivo lo reinician. También funcionan desde pausa o Stop.
- `stop`: detiene y cierra el archivo, conservando la selección y el volumen.
- `volume 0..100`: 0 silencia; 100 es la ganancia original. No modifica el volumen global de Windows.
- `state`: muestra STOPPED, PLAYING o PAUSED, posición en la lista, volumen y ruta.
- `help`: muestra los comandos. `quit`, Ctrl+C o fin de entrada cierran el programa.

Al terminar una canción el estado pasa a STOPPED y la consola permanece abierta. No hay avance automático. Una ruta abierta con `play` fuera de la lista no se agrega: next selecciona la primera y previous la última; el indicador 0/N significa que la selección no pertenece a la lista.

## Código

- `cmd/engine/main.go`: ruta inicial, lista de archivos e inicialización.
- `internal/console/console.go`: lectura y ejecución de comandos.
- `internal/player/player.go`: Play, Pause, Resume, Stop, Next, Previous, SetVolume y GetState. `New(paths...)` recibe una copia de la lista opcional.
- `internal/player/oto.go`: encapsula Oto y el volumen de salida.
- `internal/decoder/mp3.go`: decodifica MP3 a PCM estéreo int16 little-endian.

Desde Go, `SetVolume(float64)` acepta **0 a 1**. `GetState()` devuelve una copia con Status, Track, Index (base cero, -1 fuera de lista), Total, Volume y Error. El volumen inicial es 1 y se conserva al cambiar canciones. Una pista que no puede abrirse o cuya frecuencia es incompatible deja intacta la reproducción actual.

`GetState()` también detecta finalización/errores y libera el archivo. La consola lo consulta periódicamente. `Wait(ctx)` sigue disponible para programas sin consola que necesiten esperar la finalización.

## Límites actuales

Oto mantiene un contexto por proceso y usa la frecuencia del primer MP3. Las canciones de la lista deben tener esa misma frecuencia; si difieren se informa un error sin interrumpir la pista actual. No se ha agregado resampling. Windows determina la salida; Pause/Stop pueden dejar sonar brevemente audio ya enviado al dispositivo.

La consola de audio no se integra todavía con el agente HTTP/WebSocket ni con salidas multizona. No se implementan scheduler ni Dante. El laboratorio WASAPI previo se conserva solo como documentación histórica en `docs/`.

## Verificación

```powershell
go fmt ./...
go mod tidy
go test ./...
go vet ./...
go build -o bin/music-engine.exe ./cmd/engine
```

Pruebas optativas de audio real (usan copias temporales del MP3 para la navegación):

```powershell
$env:MUSIC_ENGINE_TEST_MP3 = (Resolve-Path music/demo.mp3).Path
go test ./internal/player -v -count=1
```

Se comprobaron errores de archivo, lista vacía, volumen inválido, estados, pausa/reanudación, navegación circular, cancelación y la secuencia de comandos de consola. Las pruebas del agente conectado están en `internal/control`; su cliente WebSocket utiliza `github.com/gorilla/websocket`.
