# Go Music Engine: reproducción multizona

**Distribución actual: agente Windows x64 `engine.exe` 0.7.0, selector Oto/ASIO.** Consulta [la guía beta](BETA.md) para elegir Oto (mezcla en la salida del sistema) o ASIO (driver y canales físicos), y configurar servidor y credencial cifrada. Usa `%ProgramData%\UtrackSound` para configuración, diario y logs. No necesita Go en los equipos de prueba. Las instrucciones por variables y referencias a Oto que siguen son históricas; el agente actual se configura con `engine.exe configure`.

El engine administra un `Player` independiente por zona. La consola arranca con **A y B**; el agente obtiene sus zonas desde Laravel. Cada una conserva su canción, volumen y estado. Pausar, detener o cambiar una zona no interrumpe las demás.

## Agente conectado a Laravel (objetivo 05)

`cmd/agent` 0.6.0 integra configuración HTTP/Reverb, URLs firmadas, audio y reportes. Parte detenido; el panel Zonas permite seleccionar canción, pausar, reanudar, detener, siguiente/anterior y ajustar volumen y playlist. Engines conserva sus controles anteriores.

```powershell
$env:ENGINE_SERVER = 'https://tu-servidor'
$env:ENGINE_TOKEN = '<credencial-del-equipo>'
$env:ENGINE_STATE_DIR = 'engine-state'
$env:ENGINE_CHANNEL_MODE = 'stereo' # o mono
go run ./cmd/agent
```

El modo se elige al arrancar y aplica a todas las zonas del equipo. Estéreo conserva L/R; mono mezcla `(L+R)/2` y duplica el resultado en ambos canales. Ambos perfiles mezclan las zonas en la salida predeterminada de Windows; no asignan salidas físicas independientes. El agente admite MP3 MPEG-1 Layer III a **44100 Hz**, sin remuestreo. Laravel refleja el modo negociado en configuración y estados.

Laravel entrega `audio_url` y SHA-256 del contenido. El agente valida la instantánea completa, conserva audio ante cambios de volumen/firma, detiene una zona al cambiar su playlist/contenido y elimina zonas retiradas. Una URL que devuelve 403 provoca una consulta de configuración y un único reintento con la misma canción/versión. La descarga verifica el hash antes de reproducir.

Play/Next/Previous quedan pendientes mientras se carga. Stop y cambios posteriores pueden cancelar esa carga; no se confirma éxito solo por aceptarla. Se conserva el registro durable de órdenes y la reentrega sin repetir acciones. Heartbeat informa carga, recuperación, reproducción, pausa, parada y errores; el panel de zonas muestra canción y modo. No hay autoplay ni avance automático.

`ENGINE_PROFILE=configuration_only` mantiene el modo anterior sin audio. Para una CA privada, `ENGINE_CA_FILE` añade certificados PEM a la confianza del sistema sin desactivar TLS. El backend debe servir HTTPS, configurar `ENGINE_MEDIA_URL` y usar audio privado. Los archivos públicos antiguos se importan explícitamente con `php artisan engine:import-audio`; no se migran automáticamente.

Consulta el [objetivo 06: control desde Laravel](../utrack-fly/objetivos/06-control-completo-desde-laravel.md) y el [cierre del objetivo 05](../utrack-fly/objetivos/05-audio-https-y-buffer.md). El usuario redefinió el 06 como control completo por zonas; reproducción continua y prefetch quedan pendientes de otro objetivo.

### Instalación local conectada

La instalación habitual usa `https://utrack-fly.test` con certificado confiable de Herd. `./start-local.ps1` inicia el agente con `engine-state/connection.json` (server, token, mode), ignorado por Git. No compartas ese archivo. El script no instala un servicio ni arranca automáticamente con Windows; no ejecutes dos agentes con la misma credencial. El diario conserva su bloqueo exclusivo y rechaza una segunda instancia.

El campo opcional `song_id` en una orden `play` selecciona una canción de la playlist actual; `null`/ausente conserva el reinicio de la selección actual. Next/Previous continúan desde esa selección. Laravel requiere engine >= 0.6.0 para enviar canción específica; un ID ajeno falla sin sustituir una carga válida. Los resultados y la configuración aplicada aparecen por zona en el panel. Un plazo vencido sin resultado no implica éxito ni permite afirmar que el audio nunca llegó a ejecutarse.

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

Los IDs son únicos, no vacíos y sensibles a mayúsculas. Las listas y los IDs retornados son copias. Las operaciones de cada zona admiten concurrencia. `Close` es idempotente, espera al monitor, detiene todas las zonas y rechaza nuevas operaciones sobre handles retenidos con `engine.ErrClosed`. `Reconfigure` actualiza la pertenencia de zonas y conserva los reproductores de IDs existentes. El agente gestiona selección y cambios de playlist desde su configuración.

- `cmd/engine/main.go`: crea A/B, inicia pistas y elige consola o modo headless.
- `internal/engine/manager.go`: propiedad, búsqueda por ID, supervisión y cierre de N players.
- `internal/console/console.go`: interfaz opcional del administrador.
- `internal/player/player.go`: reproducción y navegación existentes, reutilizadas sin reimplementación.
- `internal/player/oto.go`: streams independientes sobre contexto Oto compartido.
- `internal/decoder/mp3.go`: MP3 a PCM estéreo int16 little-endian.

`SetVolume(float64)` acepta 0 a 1. `GetState()` devuelve una copia con Status, Track, Index (base cero, -1 fuera de lista), Total, Volume y Error. El volumen inicial es 1. Una pista local inválida conserva la reproducción actual; una carga HTTPS detiene la anterior antes de descargar.

## Límites e integración con Laravel

Las zonas son reproductores lógicos independientes y **se mezclan en la misma salida predeterminada de Windows**. Todavía no se asignan dispositivos físicos por zona. Oto usa un contexto por proceso y la frecuencia del primer MP3; todas las pistas deben tener esa frecuencia. No hay resampling. Pause/Stop pueden dejar sonar brevemente audio ya enviado al dispositivo. Un fallo del dispositivo compartido puede afectar todas las zonas.

`cmd/agent` usa los perfiles `shared_stereo_mp3` / `shared_mono_mp3`, conservando `configuration_only` como opción. No se implementan scheduler ni Dante. Consulta el [contrato API v1.1](../utrack-fly/objetivos/01-contrato-laravel-engine-api-v1.md). El laboratorio WASAPI se conserva como documentación histórica en `docs/`.

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

La consola independiente no consulta configuración: un 403 requiere Play con otra URL. El agente conectado sí obtiene la firma renovada mediante Laravel. Se verificó la cadena completa con Laravel, HTTPS, el binario del agente y Oto reales en un entorno local aislado, tanto mono como estéreo; no se desplegó un servidor de producción.

Ejemplo: `go run ./cmd/engine -file "https://servidor/audio/1?signature=..."`.



