# Objetivo 09 — Backend ASIO real

**Estado: implementado por software; validación física pendiente.** Se añadió `internal/audio/asio.Output` compatible con `audio.Output`. Oto sigue siendo el backend predeterminado. El backend ASIO abre un solo driver por proceso, recibe PCM signed int16 little-endian intercalado del router y convierte a las salidas Float32 planar que solicita el callback host.

No se modificaron Laravel, Filament, WebSocket, autenticación ni el protocolo. Player, Manager y Mixer conservan sus responsabilidades; el Engine selecciona el backend al arrancar.

## Cómo seleccionar el backend

Oto sigue siendo el valor por defecto, también cuando `ENGINE_AUDIO_BACKEND` no se define:

```powershell
$env:ENGINE_AUDIO_BACKEND = 'oto'
.\start-local.ps1
```

Para ASIO, define las variables en la misma sesión PowerShell antes de iniciar el agente:

```powershell
$env:ENGINE_AUDIO_BACKEND = 'asio'
$env:ENGINE_ASIO_DRIVER = 'Dante Virtual Soundcard (x64)'
$env:ENGINE_ASIO_SAMPLE_RATE = '44100'
$env:ENGINE_ASIO_BUFFER_SIZE = '256'
$env:ENGINE_ASIO_CHANNELS = '8'
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
| `ENGINE_AUDIO_BACKEND` | No | `oto` por defecto; admite `oto` o `asio`. |
| `ENGINE_ASIO_DRIVER` | Para ASIO | Nombre exacto del driver que ASIO registra en Windows. No hay driver predeterminado. |
| `ENGINE_ASIO_SAMPLE_RATE` | No | Frecuencia del dispositivo. Vacía usa la frecuencia PCM de entrada. Si difiere, el backend remuestrea dentro de su productor. |
| `ENGINE_ASIO_BUFFER_SIZE` | No | Tamaño en frames, validado contra límites y granularidad del driver. Vacío usa el tamaño preferido que reporta ASIO. |
| `ENGINE_ASIO_CHANNELS` | No | Canales que se abren en el dispositivo. Vacío usa los canales entregados por el router; debe ser al menos ese número y no superar las salidas disponibles. |

El perfil de audio y `ENGINE_CHANNEL_MODE` siguen teniendo el significado anterior. Cambiar a ASIO no cambia la configuración de zonas ni sus `output.channels`.

## Host, callback y buffering

El host se adaptó de `vmvwebworks/audio_tracker/internal/asio`, código Go puro para Windows amd64 con licencia MIT; se conserva el aviso en `internal/audio/asio/THIRD_PARTY_NOTICES.md` y `THIRD_PARTY_LICENSE.txt`. El módulo no añade una dependencia Go ASIO externa, no usa CGo y no necesita instalar MinGW, MSYS2 ni el ASIO SDK. Sí requiere Windows amd64 y un driver ASIO instalado/registrado. Las llamadas COM del driver se serializan en un hilo OS bloqueado.

El callback ASIO no lee el decoder ni el stream del mixer, no toma mutexes y no realiza logging. Un productor Go fuera del callback convierte y escribe frames en un ring SPSC preasignado y acotado a ocho buffers ASIO por defecto. Si se llena, se descartan los frames más nuevos y se informa el error; si el callback se queda sin datos, llena el resto del callback con silencio y cuenta el underrun. El reset solicitado por el driver se reporta como error y requiere reiniciar el Engine; todavía no hay recuperación automática del dispositivo.

El conversor remuestrea PCM int16 intercalado cuando la frecuencia configurada difiere de la entrada, rellena con cero los canales ASIO superiores a los del PCM, y la capa host convierte a planos Float32/formato nativo del driver. Se validan el formato, frecuencia, cantidad de canales y tamaño de buffer antes de iniciar el callback. Solo se admite un driver ASIO abierto por proceso, de acuerdo con el modelo del host.

## Verificación

- `go test -count=1 ./...`: aprobado.
- `go vet ./...`: aprobado.
- Pruebas deterministas cubren ring buffer, interleaved a planar, underrun silencioso, overflow controlado, remuestreo, canales, configuración, ciclo de vida y concurrencia.
- `CGO_ENABLED=0 go test -count=1 ./internal/audio/asio`: aprobado en Windows amd64; confirma que el backend no depende de CGo.
- El inventario de Windows encontró Ableton Move/Push, SSL ASIO Driver 1-4 y Dante Virtual Soundcard (x64).
- La inicialización de Dante Virtual Soundcard se intentó sin iniciar el stream y falló con `Error connecting to Dante Virtual Soundcard Manager`. SSL ASIO Driver 1 también falló con `No device is connected to the PC.` No se hizo prueba de tono ni se afirma reproducción física.

## Estado físico y siguiente etapa

La implementación por software está disponible, pero el driver no pudo inicializarse porque Dante Virtual Soundcard no está conectada a su Manager y los drivers SSL no detectan dispositivo. Se necesita instalar/iniciar correctamente Dante Virtual Soundcard Manager, disponer de DVS autorizado/operativo y volver a ejecutar la prueba con `ENGINE_AUDIO_BACKEND=asio`, `ENGINE_ASIO_DRIVER='Dante Virtual Soundcard (x64)'`, frecuencia, buffer y canales compatibles. Después debe comprobarse el mapeo de canales y un tono conocido en el dispositivo.

Dante/Q-SYS no se configuró ni validó. Una vez que DVS pueda abrirse y se verifique audio multicanal real, la integración/ruteo Dante hacia Q-SYS corresponde al objetivo físico posterior.
