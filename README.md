# Go Music Engine: reproducción multizona

El engine administra un `Player` independiente por zona. Arranca con **A y B**; `engine.New([]ZoneConfig{...})` admite más zonas. Cada una conserva su canción, lista, volumen y estado. Pausar, detener o cambiar una zona no interrumpe las demás.

## Ejecutar

Requiere Windows y Go 1.26 o posterior. Desde `music-engine`:

```powershell
go run ./cmd/engine
# Dos pistas al iniciar:
go run ./cmd/engine -file "music/demo.mp3" -file-b "music/Drums.mp3"
# Sin leer stdin ni crear una consola interactiva:
go run ./cmd/engine -headless -file "music/demo.mp3" -file-b "music/Drums.mp3"
```

Por defecto A reproduce `music/demo.mp3` y B comienza detenida. Las rutas relativas se resuelven desde el directorio de trabajo. Cada zona carga la lista de MP3 de la carpeta de su pista inicial, ordenada por nombre; B usa la carpeta de A cuando no se indica `-file-b`.

El modo `-headless` permanece activo hasta Ctrl+C, incluso con stdin cerrado o después del fin de las canciones. No instala un servicio de Windows. El ejecutable cierra todos los players al salir. Un fallo al abrir una pista inicial cancela el arranque completo y libera los recursos ya abiertos.

## Consola opcional

```text
zones
zone B
play "C:\Mi musica\otra.mp3"
volume 35
pause
resume
next
previous
stop
zone A
state
quit
```

- `zone ID` selecciona la zona de los siguientes comandos; inicialmente A. Un ID inexistente informa error y conserva la selección.
- `zones` muestra el estado de todas las zonas. El prompt muestra la zona seleccionada.
- `play` reinicia la pista seleccionada; `play ruta` abre otro MP3, incluso fuera de la lista.
- `pause` / `resume` conservan la posición. Sin pista cargada devuelven error.
- `next` / `previous` navegan circularmente e inician la pista, también desde pausa o Stop. Fuera de la lista seleccionan la primera/última respectivamente.
- `stop` cierra el archivo de esa zona y conserva su selección y volumen.
- `volume 0..100` modifica solo esa zona, no el volumen global de Windows.
- `state` muestra estado, posición en la lista, volumen, ruta y error de la zona seleccionada.
- `help` muestra los comandos. `quit`, Ctrl+C o EOF terminan el ejecutable interactivo.

La biblioteca de consola solo envía comandos: no supervisa ni cierra players. El propietario del administrador decide cuándo cerrarlo. El monitor del engine detecta EOF/errores y libera archivos cada 100 ms aunque no exista consola. Al finalizar una pista queda STOPPED; no hay avance automático. Un error de una zona queda observable y no cierra las otras.

## Arquitectura y uso desde Go

```text
Engine Manager
  ├── A → Player A → Decoder A → stream Oto A
  └── B → Player B → Decoder B → stream Oto B
                                  ↓
                         contexto Oto compartido
```

```go
m, err := engine.New([]engine.ZoneConfig{
    {ID: "A", Tracks: []string{"music/demo.mp3"}},
    {ID: "B", Tracks: []string{"music/Drums.mp3"}},
    {ID: "C"}, // No existe un límite de dos zonas en el administrador.
})
if err != nil { return err }
defer m.Close()
a, err := m.Zone("A")
if err != nil { return err }
if err := a.SetVolume(0.35); err != nil { return err }
if err := a.Play("music/demo.mp3"); err != nil { return err }
// Mantener vivo el proceso hasta cancelar su contexto.
<-ctx.Done()
```

Los IDs son únicos, no vacíos y sensibles a mayúsculas. Las listas y los IDs retornados son copias. Las operaciones de cada zona admiten concurrencia. `Close` es idempotente, espera al monitor, detiene todas las zonas y rechaza nuevas operaciones sobre handles retenidos con `engine.ErrClosed`. Las zonas se definen al construir el administrador; no se agregan ni eliminan en caliente.

- `cmd/engine/main.go`: crea A/B, inicia pistas y elige consola o modo headless.
- `internal/engine/manager.go`: propiedad, búsqueda por ID, supervisión y cierre de N players.
- `internal/console/console.go`: interfaz opcional del administrador.
- `internal/player/player.go`: reproducción y navegación existentes, reutilizadas sin reimplementación.
- `internal/player/oto.go`: streams independientes sobre contexto Oto compartido.
- `internal/decoder/mp3.go`: MP3 a PCM estéreo int16 little-endian.

`SetVolume(float64)` acepta 0 a 1. `GetState()` devuelve una copia con Status, Track, Index (base cero, -1 fuera de lista), Total, Volume y Error. El volumen inicial es 1. Una pista inválida o con frecuencia incompatible conserva la reproducción actual.

## Límites e integración con Laravel

Las zonas son reproductores lógicos independientes y **se mezclan en la misma salida predeterminada de Windows**. Todavía no se asignan dispositivos físicos por zona. Oto usa un contexto por proceso y la frecuencia del primer MP3; todas las pistas deben tener esa frecuencia. No hay resampling. Pause/Stop pueden dejar sonar brevemente audio ya enviado al dispositivo. Un fallo del dispositivo compartido puede afectar todas las zonas.

`cmd/agent` conserva el perfil **configuration_only**, con HTTP, heartbeat, Reverb y deduplicación durable. El agente aún no se conecta a la reproducción. El reproductor admite descarga HTTPS independiente, sin caché persistente. No se implementan scheduler ni Dante. Consulta el [contrato API v1](../utrack-fly/objetivos/01-contrato-laravel-engine-api-v1.md), el [objetivo 02](../utrack-fly/objetivos/02-conexion-y-sincronizacion-del-engine.md) y el [cierre WebSocket](../utrack-fly/objetivos/03-sincronizacion-mediante-websocket.md). El laboratorio WASAPI se conserva como documentación histórica en `docs/`.

## Verificación

```powershell
$env:GOCACHE = Join-Path $env:TEMP 'utrack-go-build'
go test ./...
go vet ./...
go build -o bin/music-engine.exe ./cmd/engine
go build -o bin/agent.exe ./cmd/agent
# Integración optativa contra el backend real de audio:
$env:MUSIC_ENGINE_TEST_MP3 = (Resolve-Path music/demo.mp3).Path
go test ./internal/engine ./internal/player -v -count=1
```

Las pruebas cubren IDs inválidos y desconocidos, tercera zona, copias de IDs, volumen aislado, cierre concurrente, supervisión sin consola y continuidad ante errores. La integración reproduce dos streams simultáneos y verifica que pausa, navegación, volumen, error de archivo y Stop de A conservan B, y que Stop de B conserva A. La consola prueba selección de zona, IDs incorrectos y EOF.

## Audio HTTPS (objetivo 05)

`-file`, `-file-b` y `play` aceptan URLs HTTPS firmadas que entreguen MP3. Las listas Go también admiten URLs. `play` sin argumento conserva la firma al reiniciar. La consulta de la URL se oculta en estados y no aparece en errores del transporte. Una URL pasada por CLI permanece visible en los argumentos del proceso.

- Autorización mediante URL firmada según el contrato v1, sin enviar el token del equipo. TLS verifica certificados y no se siguen redirecciones.
- Descarga completa antes de reproducir, sin temporales ni caché persistente. Máximo 32 MiB de MP3 por zona, más un byte para detectar exceso y los buffers de HTTP, decoder y Oto. N zonas multiplican el límite; no es un límite global de RAM. No hay streaming progresivo.
- `Play(URL)` acepta la carga y retorna. Consultar `LOADING`, `RECOVERING`, `PLAYING` o `ERROR` con `GetState()`. `Wait` espera también las cargas. La pista anterior se detiene al comenzar la carga remota. Pause/Resume requieren audio cargado.
- Hasta tres intentos, 30 segundos por intento incluidos cuerpo y cabeceras, pausas de 500 ms y 1 s. Reintenta transporte, lecturas incompletas, 408, 429 y 5xx. Otros códigos, exceso de tamaño y MP3 inválido fallan sin repetir.
- Cambio de canción, navegación, Stop y Close cancelan descarga y backoff. Los resultados obsoletos no inician audio. Cada zona conserva volumen y transporte independientes.
- Solo MP3; la frecuencia debe coincidir con el contexto Oto, definido por la primera pista. No hay remuestreo.

El servidor debe entregar URLs autorizadas. Esta entrada no emite ni renueva firmas: un 403 queda en ERROR y requiere Play con una URL renovada. El enlace automático con config, la renovación tras 403 del contrato y las órdenes del agente siguen pendientes; `cmd/agent` conserva `configuration_only`. Se verifica con HTTPS local y Oto real, no con un servidor Laravel desplegado.

Ejemplo: `go run ./cmd/engine -file "https://servidor/audio/1?signature=..."`.
