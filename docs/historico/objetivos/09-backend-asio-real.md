# Objetivo 09 — Backend ASIO real

**Estado: stream real ASIO/DVS validado el 27-09-2026; recepción en red Dante/bocinas pendiente.** `internal/audio/asio.Output` es compatible con `audio.Output`. ASIO es el backend predeterminado de ambos ejecutables. El backend abre un solo driver por proceso, recibe PCM signed int16 little-endian intercalado del router y convierte a las salidas Float32 planar que solicita el callback host.

Player, Manager, Mixer, WebSocket, autenticación y protocolo se conservaron. Fue necesario corregir dos puntos concretos de Laravel: `EngineConfiguration` asignaba `[1,2]` a todas las zonas y `EngineHeartbeatRequest` rechazaba canales superiores a 2. Ahora la instantánea asigna canales consecutivos por orden de ID (pares estéreo o un canal mono) y el heartbeat admite canales 1–64, conservando la comprobación contra la instantánea. Esta asignación es ordinal: retirar zonas o cambiar el perfil puede cambiar los canales; revisar el ruteo y reiniciar el Engine si cambia la cantidad total de canales abiertos.

## Cómo seleccionar el backend

ASIO es el valor por defecto cuando `ENGINE_AUDIO_BACKEND` no se define. Sin driver configurado devuelve error; nunca cae a Oto. Oto permanece como compatibilidad explícita en la factoría. El lanzador local fuerza ASIO:

```powershell
.\start-local.ps1
```

El lanzador usa estos valores si no hay ajustes ASIO definidos; la cantidad de canales se deduce del Router:

```powershell
$env:ENGINE_AUDIO_BACKEND = 'asio'
$env:ENGINE_ASIO_DRIVER = 'Dante Virtual Soundcard (x64)'
$env:ENGINE_ASIO_SAMPLE_RATE = '48000'
$env:ENGINE_ASIO_BUFFER_SIZE = '256'
.\start-local.ps1
```

`start-local.ps1` conserva su lectura de `engine-state/connection.json`; las variables de audio se heredan del proceso PowerShell y no deben añadirse junto con credenciales al archivo de conexión.

`start-local.ps1` ejecuta `bin/agent.exe`; después de actualizar el Engine, recompílalo antes de probar las variables nuevas:

```powershell
$env:CGO_ENABLED = '0'
go build -o bin/agent.exe ./cmd/agent
```

| Variable | Obligatoria | Comportamiento |
|---|---:|---|
| `ENGINE_AUDIO_BACKEND` | No | `asio` por defecto; admite `oto` solo explícitamente. `start-local.ps1` fuerza `asio`. |
| `ENGINE_ASIO_DRIVER` | Para ASIO | Nombre exacto del driver que ASIO registra en Windows. No hay driver predeterminado. |
| `ENGINE_ASIO_SAMPLE_RATE` | No | Frecuencia del dispositivo. Vacía usa la frecuencia PCM de entrada. Si difiere, el backend remuestrea dentro de su productor. |
| `ENGINE_ASIO_BUFFER_SIZE` | No | Tamaño en frames, validado contra límites y granularidad del driver. Vacío usa el tamaño preferido que reporta ASIO. |
| `ENGINE_ASIO_CHANNELS` | No | Canales que se abren en el dispositivo. Vacío usa los canales entregados por el router; debe ser al menos ese número y no superar las salidas disponibles. |

El perfil de audio y `ENGINE_CHANNEL_MODE` siguen teniendo el significado anterior. Cambiar a ASIO no cambia la configuración de zonas ni sus `output.channels`.

## Host, callback y buffering

El host se adaptó de `vmvwebworks/audio_tracker/internal/asio`, código Go puro para Windows amd64 con licencia MIT; se conserva el aviso en `internal/audio/asio/THIRD_PARTY_NOTICES.md` y `THIRD_PARTY_LICENSE.txt`. El módulo no añade una dependencia Go ASIO externa, no usa CGo y no necesita instalar MinGW, MSYS2 ni el ASIO SDK. Sí requiere Windows amd64 y un driver ASIO instalado/registrado. Las llamadas COM del driver se serializan en un hilo OS bloqueado.

El callback ASIO no lee el decoder ni el stream del mixer, no toma mutexes y no realiza logging. Un productor Go fuera del callback convierte y escribe frames en un ring SPSC preasignado y acotado a ocho buffers ASIO por defecto. Si se llena, se descartan los frames más nuevos y se informa el error; si el callback se queda sin datos, llena el resto del callback con silencio y cuenta el underrun. El reset solicitado por el driver se reporta como error y requiere reiniciar el Engine; todavía no hay recuperación automática del dispositivo.

El conversor remuestrea PCM int16 intercalado cuando la frecuencia configurada difiere de la entrada, rellena con cero los canales ASIO superiores a los del PCM, y la capa host convierte a planos Float32/formato nativo del driver. Se validan el formato, frecuencia, cantidad de canales y tamaño de buffer antes de iniciar el callback. Solo se admite un driver ASIO abierto por proceso, de acuerdo con el modelo del host.

Al iniciar, `cmd/agent` registra el backend elegido y la configuración ASIO solicitada. Al abrir el stream, ASIO registra capacidades/configuración efectiva y formato PCM. Durante la reproducción, cada cinco segundos informa frames producidos desde el router, llamadas/frames del callback, frames consumidos, underruns y overflow del ring. `control.applyAudio` registra `output.channels` cuando cambia el routing. El callback no escribe logs.

## Verificación

- `go test -count=1 ./...`: aprobado.
- `go vet ./...`: aprobado.
- Pruebas deterministas cubren ring buffer, interleaved a planar, underrun silencioso, overflow controlado, remuestreo, canales, configuración, ciclo de vida y concurrencia.
- `CGO_ENABLED=0 go test -count=1 ./internal/audio/asio`: aprobado en Windows amd64; confirma que el backend no depende de CGo.
- El inventario de Windows encontró Ableton Move/Push, SSL ASIO Driver 1-4 y Dante Virtual Soundcard (x64). Windows también lista Realtek High Definition Audio, pero no hay un driver ASIO Realtek registrado.
- Una prueba de apertura/cierre de Dante Virtual Soundcard sin iniciar streaming pasó: reportó 48000 Hz actuales, 128 salidas, buffers 32–2048, preferido 64, granularidad potencia de dos; el driver anuncia soporte para 44100 y 48000 Hz. Buffer 64 y ocho canales están dentro de esas capacidades.
- Ableton Move/Push y SSL ASIO Driver 1-4 fallaron al inicializar con `No device is connected to the PC.` La prueba DVS solo inicializó/consultó/cerró el driver: no creó buffers de stream, no ejecutó callback ni reprodujo un tono. No se afirma reproducción física.
- El `bin/agent.exe` de la instalación local tenía timestamp anterior a los cambios diagnósticos de `cmd/agent/main.go` y `internal/audio/asio/output.go`; reconstruirlo antes del siguiente ensayo con `CGO_ENABLED=0 go build -o bin/agent.exe ./cmd/agent`.

## Estado físico y siguiente etapa

La sesión del 27-09-2026 identificó `zone_not_assigned` en el diario: las tres zonas se rechazaban antes de abrir ASIO por compartir `[1,2]`. Tras la corrección, el agente aplicó revisión 12 y las tres órdenes Play de Laravel terminaron en `playing`, sin errores de configuración o zona. La descarga HTTPS y el decoder entregaron PCM a 44100 Hz; ASIO abrió DVS a 48000 Hz y buffer 256 con seis salidas. A los diez segundos consumió 480000 frames, con cero underruns y overflows.

| Zona | output.channels (1-based) | Índices ASIO (0-based) | DVS TX esperado |
|---|---|---|---|
| 2 / Zona de Albercas | 1, 2 | 0, 1 | 1, 2 |
| 3 / Salon POLO | 3, 4 | 2, 3 | 3, 4 |
| 4 / SDFSDF | 5, 6 | 4, 5 | 5, 6 |

La prueba optativa `TestRealDVSStreamingOptional` envía un tono de 440 Hz a -30 dBFS durante diez segundos a través del Router. Abrió ocho canales y comprobó 479200 muestras no silenciosas en cada canal 1–2 y cero en 3–8, 1878 callbacks, 480000 frames consumidos y cero underruns/overflows. Un ensayo anterior registró un underrun inicial; vigilar los incrementos durante reproducción prolongada. Los contadores `nonzero_samples_per_channel` son acumulados desde la apertura: su incremento demuestra señal, no recepción remota.

```powershell
$env:CGO_ENABLED = '0'
# Ejecutar solo con el agente detenido, para liberar el driver.
$env:MUSIC_ENGINE_ASIO_STREAM_TEST = '1'
go test -v -count=1 -timeout 30s ./internal/audio/asio -run '^TestRealDVSStreamingOptional$'
Remove-Item Env:MUSIC_ENGINE_ASIO_STREAM_TEST
```

Logs locales de la validación: `engine-state/asio-validation.stderr.log` y `engine-state/asio-signal.stderr.log`. No compartir `connection.json`. Se reconstruyó `bin/agent.exe`, pasaron `go test -count=1 ./...`, `go vet ./...`, 31 pruebas Laravel de audio/API y Pint.

Dante/Q-SYS no se configuró ni validó. El adaptador Realtek Ethernet está conectado a 100 Mbps; también existe un adaptador virtual VirtualBox. Falta comprobar en DVS la interfaz seleccionada y en Dante Controller el reloj, los TX y la recepción. La apertura automatizada del panel DVS agotó el tiempo de autorización. Los contadores prueban entrega al callback del driver, no paquetes Dante ni sonido en bocinas. El siguiente paso físico es comprobar señal en un receptor y después establecer las suscripciones TX → RX correspondientes.
