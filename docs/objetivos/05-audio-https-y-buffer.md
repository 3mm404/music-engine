# Objetivo 05 — Audio HTTPS integrado con Laravel

Fecha de cierre y verificación: **26 de septiembre de 2026**.

**Estado: implementado y verificado de extremo a extremo en entorno local aislado.** Laravel emite URLs autorizadas, el agente aplica la configuración por zona, reproduce MP3 por HTTPS y reporta el resultado. Se conserva el perfil opcional `configuration_only` para equipos sin audio. El objetivo 06 corresponde a reproducción continua y preparación de la siguiente canción; no se inició aquí.

## Alcance completado

- Archivos nuevos en almacenamiento privado; URLs HTTPS firmadas con caducidad (300 segundos por defecto).
- Autorización de cada descarga por firma, equipo habilitado, sesión vigente, asignación de playlist al negocio/equipo y versión de contenido. Rotar la credencial, abrir otra sesión o retirar la asignación invalida el acceso. No se envía Bearer a la ruta de medios.
- Configuración con `audio_url`, `url_expires_at`, `content_version` SHA-256 y formato MP3. Las URLs se agregan después de calcular la revisión y no se guardan en la instantánea histórica. Renovar una firma no cambia `config_revision`; cambiar los bytes sí.
- Conexión del runtime HTTP/Reverb existente con `engine.Manager` y sus reproductores por zona. No se reimplementaron autenticación, WebSocket ni el registro durable de órdenes.
- Descarga completa antes de reproducir, limitada a 32 MiB de MP3 por zona. Se comprueba el SHA-256 recibido y la frecuencia real antes de abrir la salida.
- Estados `loading`, `recovering`, `playing`, `paused`, `stopped` y `error`, visibles en heartbeat y en el panel de zonas, con canción, modo y error.
- Hasta tres intentos de descarga, 30 segundos por intento y esperas de 500 ms/1 s. Se reintentan transporte, lecturas incompletas, HTTP 408/429/5xx. TLS verifica certificados y se rechazan redirecciones.
- Tras HTTP 403 se obtiene configuración nueva y se vuelve a cargar una sola vez la misma canción y versión. Un segundo 403 u otro fallo terminal produce `audio_download_failed`.
- Órdenes `play`, `pause`, `resume`, `stop`, `next` y `previous`, disponibles desde el panel de equipos y confirmadas mediante la API existente. Volumen y playlists siguen gestionándose por configuración.
- Cancelación al cambiar canción, detener, retirar una zona o cerrar el proceso. Una carga reemplazada no puede iniciar audio.

El máximo de 32 MiB es por zona, más un byte para detectar exceso y los buffers adicionales de HTTP, decoder y Oto. No es un límite global de RAM; no hay streaming progresivo ni caché persistente.

## Ajuste mono / estéreo desde el engine

El usuario solicitó poder elegir el modo en el engine. `cmd/agent` acepta:

```powershell
$env:ENGINE_CHANNEL_MODE = 'stereo' # o 'mono'
```

Se configura antes del arranque; cambiarlo requiere reiniciar el agente. Todas las zonas de ese equipo usan el modo elegido, que se negocia al abrir sesión y se refleja en configuración y reportes.

| Perfil negociado | Salida |
| --- | --- |
| `shared_stereo_mp3` | Conserva izquierda y derecha. Es el predeterminado. |
| `shared_mono_mp3` | Calcula `(L+R)/2` sin desbordamiento y duplica la mezcla en ambos canales. |
| `configuration_only` | Sin audio; conserva compatibilidad con las etapas anteriores. |

Los perfiles de audio usan la salida predeterminada de Windows, `output={"device_id":"default","channels":[1,2]}`. Las zonas se mezclan en ese dispositivo. Mono no significa asignar un canal físico exclusivo por zona. Este perfil compartido se documenta como extensión explícita del contrato; no declara soporte de dispositivos físicos independientes.

La frecuencia del perfil conectado es **44100 Hz**, MP3 MPEG-1 Layer III, fuente de uno o dos canales. No hay remuestreo. El modo elegido en el equipo es la autoridad para la salida compartida; el campo histórico de modo de zona no lo reemplaza.

## Comportamiento de configuración y órdenes

La aplicación de configuración valida toda la instantánea antes de modificar las zonas. Una configuración incompatible conserva la revisión anterior y genera `config_error`. Un cambio de volumen o renovación de URL conserva la reproducción; cambiar playlist, orden, versión de contenido o salida detiene la zona y selecciona la primera canción. Retirar una zona la detiene y elimina. No hay autoplay al configurar o reiniciar.

`play`, `next` y `previous` quedan pendientes durante la descarga. El agente escribe `started` antes de ejecutar y persiste el resultado terminal antes de confirmarlo. La carga no bloquea Stop ni cambios posteriores. Una orden válida de Stop o cambio que reemplaza una carga produce `command_superseded` para la orden anterior. Pause/Resume sin audio cargado fallan con `no_track`.

La reentrega de un ID terminado reenvía el resultado sin reproducir de nuevo. Tras una caída, un registro iniciado sin resultado produce `execution_unknown` y no se repite. Los reinicios parten detenidos. Al terminar la canción se reporta `stopped`, sin avanzar automáticamente.

## Preparación del backend y ejecución

1. Servir Laravel por HTTPS con un certificado confiable. Configurar `ENGINE_MEDIA_URL` con el origen HTTPS de esta aplicación y `ENGINE_AUDIO_URL_SECONDS` si se necesita otra caducidad. Un proxy que termine TLS debe configurar correctamente los proxies de confianza de Laravel; no se desactiva la validación de HTTPS.
2. Cargar MP3 de 44100 Hz y hasta 32 MiB desde el panel. Las cargas nuevas usan `storage/app/private/songs`.
3. Para canciones antiguas en `storage/app/public/songs`, ejecutar explícitamente `php artisan engine:import-audio`. El comando copia al disco privado, verifica SHA-256 y solo entonces retira la copia pública. Un conflicto conserva el original y falla. No se ejecutó sobre los archivos del usuario durante esta implementación.
4. Asignar las playlists y zonas al equipo. Emitir su credencial con el procedimiento existente.
5. Iniciar el agente:

```powershell
$env:ENGINE_SERVER = 'https://tu-servidor'
$env:ENGINE_TOKEN = '<credencial-del-equipo>'
$env:ENGINE_STATE_DIR = 'engine-state'
$env:ENGINE_CHANNEL_MODE = 'stereo' # o mono
.\bin\agent.exe
```

Si la instalación utiliza una CA privada, `ENGINE_CA_FILE` permite añadir sus certificados PEM al conjunto de confianza del sistema para API y audio. No omite la comprobación de certificados ni de nombre del servidor. Las pruebas usan esta opción con un certificado temporal, sin instalarlo globalmente.

`ENGINE_PROFILE=configuration_only` mantiene el modo anterior sin audio. No establecerlo si se desea reproducción. Desde el panel Engines, «Control de audio» envía las órdenes y «Enviar stop» detiene una zona. El panel de zonas muestra el estado observado; encolar no equivale a confirmar ejecución.

El ejecutable de consola `cmd/engine` sigue admitiendo archivos locales y URLs directas. La renovación mediante configuración pertenece al agente conectado, no al reproductor de consola independiente. Las firmas de URLs directas se ocultan en el estado, pero una URL pasada por CLI permanece en los argumentos del proceso.

## Archivos principales

Backend:

- `app/Services/EngineAudio.php`: inspección MP3, versión de contenido, perfiles y URLs firmadas.
- `app/Http/Controllers/EngineAudioController.php` y `routes/api.php`: descarga autorizada.
- `app/Services/EngineConfiguration.php`: instantáneas sin firmas en su hash y órdenes de transporte.
- `EngineController` / `EngineHeartbeatRequest`: negociación y validación de reportes.
- Recursos Filament de canciones, equipos y zonas: cargas privadas, acciones y estados visibles.
- `app/Console/Commands/ImportEngineAudio.php`: importación explícita de archivos públicos antiguos.
- `config/engine.php` / `.env.example`: origen de medios y caducidad.
- `tests/Feature/EngineAudioTest.php`, `EngineAdminTest.php`, `EngineReverbIntegrationTest.php`: cobertura backend y regresión.

Engine:

- `internal/control/playback.go` / `runtime.go`: aplicación por zona, órdenes pendientes, renovación 403, resultados y reportes.
- `internal/control/client.go` / `types.go`: negociación, metadatos y confianza TLS opcional.
- `internal/engine/manager.go`: actualización de zonas conservando las que no cambian.
- `internal/player/player.go` / `mono.go`: descarga cancelable, frecuencia esperada y mezcla mono.
- `internal/decoder/https.go`: límites, errores HTTP tipados y verificación SHA-256.
- `cmd/agent/main.go`: versión 0.5.0 y selección mono/estéreo.
- Pruebas nuevas en `internal/control/playback_test.go`, `laravel_test.go` y `internal/player/mono_test.go`.

## Verificación

- Suite Go, `go vet ./...` y compilación de ambos ejecutables aprobados.
- Pruebas con Oto real, recuperación HTTPS, cancelación y regresiones multizona aprobadas.
- Pruebas PHP de autorización, expiración, renovación sin cambio de revisión, revocación, contenido reemplazado, pertenencia de reportes, importación y órdenes aprobadas. Pint aplicado.
- `TestLaravelAudioEndToEnd`: **aprobada en estéreo y mono**. Usa PHP/Laravel reales, SQLite y archivos aislados, proxy HTTPS con certificado verificado, binario del agente y Oto. Fuerza una URL realmente expirada y verifica renovación, reporte de reproducción, pause/resume/stop/next/previous y volumen sin otra descarga.
- Suite PHP completa: 57 pruebas aprobadas, 264 aserciones y una integración optativa omitida en esa ejecución. La integración Reverb se ejecutó aparte y pasó con 13 aserciones, conservando `configuration_only` para comprobar suscripciones, reconexión y deduplicación.
- Repetición final de la cadena completa con los binarios finales: 38.81 segundos, aprobada en estéreo y mono.
- No se desplegó un servidor de producción ni se cambiaron credenciales, bases de datos o archivos musicales existentes. El detector de carreras sigue pendiente por el compilador C de Windows disponible; no se declara aprobado.

Para repetir la integración completa desde `music-engine`:

```powershell
$env:GOCACHE = Join-Path $env:TEMP 'utrack-go-build'
go build -o bin/agent.exe ./cmd/agent
$env:ENGINE_LARAVEL_ROOT = (Resolve-Path ../utrack-fly).Path
$env:ENGINE_PHP = 'C:/Users/perez/.config/herd/bin/php85/php.exe'
$env:ENGINE_E2E_BINARY = (Resolve-Path bin/agent.exe).Path
$env:MUSIC_ENGINE_TEST_MP3 = (Resolve-Path music/demo.mp3).Path
go test ./internal/control -run TestLaravelAudioEndToEnd -count=1 -v -timeout 120s
```

## Continuidad hacia el objetivo 06

El objetivo 06 será **reproducir playlists continuamente**: avance automático, preparación en memoria de la siguiente canción y políticas ante final de playlist, canciones fallidas, cambios de playlist, pérdida de internet y agotamiento de búfer. Esos comportamientos no se implementaron en el objetivo 05.

Consultar el [CHECKPOINT del workspace](../../CHECKPOINT.md) y el [contrato actualizado](01-contrato-laravel-engine-api-v1.md). Reutilizar el runtime y el registro durable; trabajar por etapas verificables y actualizar el checkpoint al cerrar cada una.
