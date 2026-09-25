# Objetivo 02 — Conexión del equipo y sincronización con Laravel

Fecha: **25 de septiembre de 2026**.

Este documento resume el trabajo de las etapas **2. Conectar y registrar el equipo** y **3. Sincronizar mediante WebSocket**. Complementa el [contrato de la API v1](01-contrato-laravel-engine-api-v1.md).

El cierre específico de la etapa 3 y la continuidad hacia el objetivo 4 están en [Objetivo 03 — Sincronizar mediante WebSocket](03-sincronizacion-mediante-websocket.md).

**Estado: objetivo cerrado para el alcance de conexión y sincronización sin audio.** Implementación local de Laravel y Go verificada con pruebas automatizadas, integración real entre Laravel, Reverb, worker y agente Go, y revisión del panel en navegador. La migración está aplicada en la base de datos local. No incluye despliegue en producción ni reproducción de audio.

## 1. Resultado de esta etapa

Laravel administra equipos y sus zonas. Cada equipo tiene un negocio, nombre, dirección del servidor, credencial individual y opción de habilitarlo o deshabilitarlo.

El nuevo agente Go inicia conexiones salientes hacia Laravel, abre una sesión, obtiene sus zonas y reporta versión, revisión aplicada y disponibilidad. Los cambios de volumen y playlist actualizan su estado de configuración, sin abrir un dispositivo de audio.

Reverb transmite avisos por un canal privado del equipo. Ante un aviso o una reconexión, el agente consulta la configuración vigente y después las órdenes pendientes. Las consultas periódicas permiten recuperar cambios aunque un aviso WebSocket se pierda.

## 2. Administración en Laravel

Se añadió el recurso **Engines** al panel Filament, con:

- Alta y edición de equipos; el negocio de un equipo existente es inmutable.
- Dirección del servidor Laravel para configurar `ENGINE_SERVER` en el equipo.
- Habilitación y deshabilitación.
- Emisión o rotación de la credencial, mostrada una sola vez. Laravel guarda únicamente su hash SHA-256.
- Estado **Conectado / Desconectado**, versión y última actividad.
- Revisión enviada, revisión aplicada y error de configuración reportado.
- Cantidad de zonas, órdenes pendientes y resultado de la última orden.
- Envío de una orden `stop` a una zona asignada para comprobar la entrega y confirmación.

El formulario de zonas permite asignar el engine del mismo negocio, seleccionar playlist, cambiar volumen entre 0 y 100 y elegir mono o estéreo. La tabla incorpora valores reportados por el equipo y marca el reporte como desactualizado cuando no está conectado. Las tablas se refrescan cada 10 segundos.

El alta de equipos requiere `is_super_admin`. El acceso delegado a equipos utiliza permisos `ViewAny:Engine`, `View:Engine` y `Update:Engine`, junto con pertenencia al negocio. No se asignan estos permisos automáticamente a usuarios existentes.

## 3. Autenticación, sesión y disponibilidad

El agente usa `Authorization: Bearer <token>` para autenticarse. No existe autorregistro anónimo.

Al abrir sesión informa `engine_version` y la capacidad `configuration_only`. Laravel devuelve la identidad del equipo, un `session_id`, intervalos y los datos públicos necesarios para conectar a Reverb. Las peticiones posteriores incluyen `X-Engine-Session`.

Una sesión nueva invalida la anterior. Un proceso que recibe `409 session_superseded` se detiene para evitar que dos procesos se expulsen repetidamente. Rotar la credencial también invalida la sesión anterior.

El heartbeat tiene un intervalo de 10 segundos y también se envía después de la sincronización. Laravel calcula la última actividad con su propio reloj. Después de 30 segundos sin un reporte nuevo válido, el equipo se muestra desconectado. Se conserva su último estado observado.

Cada reporte tiene una secuencia creciente por sesión. Un reporte antiguo no reemplaza uno nuevo; repetir un reporte idéntico es válido; reutilizar su secuencia con otro contenido devuelve `409 report_conflict`.

## 4. Configuración deseada y estado observado

Laravel es la fuente de la configuración deseada. El estado observado cambia únicamente al recibir un reporte válido del equipo.

Las instantáneas incluyen IDs de zonas, nombres, volumen, modo de canal y playlist con sus canciones ordenadas. Se calcula una huella del contenido y se guarda una revisión creciente cuando cambia la instantánea obtenida por el equipo. Solicitar la misma configuración no crea revisiones nuevas.

Se conservan instantáneas históricas para validar reportes de una revisión anterior. Una revisión fallida no obliga al agente a declarar aplicada parcialmente la nueva configuración. Las zonas eliminadas de una instantánea desaparecen de su estado local.

### Perfil implementado: `configuration_only`

El contrato 01 describe también la futura reproducción. Esta entrega implementa un perfil explícito y limitado:

- La apertura de sesión exige la capacidad `configuration_only`.
- La respuesta de configuración identifica `profile: "configuration_only"`.
- `output` se transmite como `null`; `output_channel` todavía no controla hardware.
- Las canciones incluyen su `song_id`; todavía no se publican URLs ni metadatos de audio.
- El agente reporta zonas `stopped`, sin pista cargada ni posición de reproducción.
- Volumen y playlist representan estado de configuración aplicado en memoria. No prueban una ganancia aplicada a una salida física.
- `stop` se confirma como operación válida sobre este estado detenido. Las acciones de reproducción se rechazan como `unsupported_action`.

Por tanto, esta entrega **no implementa todavía todo el contrato de reproducción del documento 01**. El perfil debe conservarse explícito hasta incorporar y validar el backend de audio.

## 5. WebSocket con Reverb

El canal de cada equipo es `private-engines.{device_id}`. Su autorización se solicita por HTTP con la credencial y la sesión vigentes. Un equipo no puede autorizar el canal de otro.

El evento `engine.changed` funciona como aviso de que debe consultar el servidor; no ejecuta instrucciones directamente. Los eventos se despachan después del commit y se transmiten mediante la cola de Laravel. Es necesario ejecutar el worker además del servidor Reverb.

Los cambios de zona, playlist, canción o relación playlist–canción generan notificaciones. Los eventos de cliente de Reverb están deshabilitados para esta aplicación.

El cliente Go implementa el protocolo compatible con Pusher usado por Reverb: handshake, autorización privada, suscripción, ping/pong y reconexión con espera creciente. La suscripción recuperada dispara una sincronización completa. Como respaldo, consulta configuración y órdenes cada 5 segundos. Este intervalo se puede configurar en Laravel con `ENGINE_POLL_INTERVAL_SECONDS`; las pruebas de integración lo elevan a 300 segundos para demostrar que la actualización procede de WebSocket.

Las notificaciones no sustituyen la persistencia: la configuración actual y las órdenes pendientes permanecen en Laravel aunque el equipo esté desconectado.

## 6. Órdenes y deduplicación

Cada orden tiene ID único, secuencia por equipo, zona, revisión de configuración y vencimiento. Consultarla no la elimina. Laravel conserva las órdenes sin resultado y las vuelve a entregar.

El agente procesa primero configuración y luego órdenes, de forma secuencial. Antes de procesar una orden escribe un registro durable de inicio; antes de confirmarla guarda su resultado y sincroniza el archivo con el disco.

- Una orden terminada que vuelve a llegar solo reenvía el resultado guardado.
- Si el proceso terminó después de registrar el inicio pero antes de guardar el resultado, se informa `execution_unknown`, sin repetir la acción.
- Un registro corrupto impide continuar: no se descarta silenciosamente la información de deduplicación.
- Un bloqueo de archivo impide compartir simultáneamente el directorio de estado entre procesos.
- El registro se separa por servidor e identidad del equipo y no se purga automáticamente.
- Laravel acepta una confirmación repetida idéntica; otro resultado para la misma orden devuelve `409 result_conflict`.

Se validan órdenes vencidas, zonas retiradas y revisiones diferentes. La semántica es entrega al menos una vez con deduplicación durable, no una promesa de ejecución exactamente una vez ante cualquier caída.

## 7. Endpoints implementados

Todos usan el prefijo `/api/v1/engine` y autenticación Bearer.

| Método | Ruta | Función |
| --- | --- | --- |
| POST | `/sessions` | Abrir sesión y reportar versión/capacidad. |
| GET | `/config` | Obtener la instantánea vigente. |
| POST | `/heartbeat` | Reportar disponibilidad y estado aplicado. |
| GET | `/commands` | Consultar hasta 100 órdenes pendientes. |
| PUT | `/commands/{command_id}/result` | Confirmar un resultado terminal. |
| POST | `/broadcasting/auth` | Autorizar el canal privado propio. |

## 8. Archivos principales

| Proyecto | Archivos / directorios |
| --- | --- |
| Laravel: datos | `app/Models/Engine.php`, `EngineCommand.php`, `PlaylistSong.php` y migración `2026_09_25_195349_create_engine_control_tables.php`. |
| Laravel: protocolo | `app/Http/Controllers/EngineController.php`, middleware `AuthenticateEngine`, request `EngineHeartbeatRequest`, `routes/api.php`. |
| Laravel: sincronización | `app/Services/EngineConfiguration.php`, `EngineProtocol.php`, evento `EngineChanged` y observer `EngineConfigurationObserver`. |
| Laravel: panel | `app/Filament/Admin/Resources/Engines/` y cambios en `Zones/ZoneResource.php`. |
| Laravel: Reverb | `config/broadcasting.php`, `config/reverb.php`, `config/engine.php`. |
| Go: agente conectado | `cmd/agent/main.go`. |
| Go: implementación | `internal/control/`: HTTP, tipos del protocolo, runtime, WebSocket, registro durable y bloqueo de archivos. |
| Go: consola de audio existente | `cmd/engine/` se conserva separado del nuevo agente. |
| Pruebas de integración real | `tests/Feature/EngineReverbIntegrationTest.php`, con base SQLite temporal, puertos locales libres y limpieza de procesos/datos al terminar. |

## 9. Preparación y ejecución

### Laravel

PHP 8.5 ya estaba instalado con Herd en este equipo. Se encontró en:

```text
C:\Users\perez\.config\herd\bin\php85\php.exe
```

Se instaló Laravel Boost según `AGENTS.md` y se incorporó `laravel/reverb` 1.12. Composer ajustó las dependencias transitivas de Guzzle para compatibilidad con Reverb. Boost también generó sus instrucciones, skills y configuración del proyecto.

La configuración requiere:

```dotenv
BROADCAST_CONNECTION=reverb
QUEUE_CONNECTION=database
REVERB_APP_ID=<identificador>
REVERB_APP_KEY=<clave-publica>
REVERB_APP_SECRET=<secreto-del-servidor>
REVERB_HOST=127.0.0.1
REVERB_PORT=8080
REVERB_SCHEME=http
ENGINE_WEBSOCKET_URL=ws://127.0.0.1:8080
```

El secreto de Reverb permanece en Laravel; no se configura en el agente. El `.env` local ya contiene las credenciales Reverb y se cambió `BROADCAST_CONNECTION` a `reverb`. No se incluyen esas credenciales en este documento. La migración de engines ya se aplicó localmente; los siguientes comandos se conservan para preparar otras instalaciones y arrancar los servicios.

Desde `utrack-fly`, preparar la base de datos y ejecutar cada proceso en su terminal:

```powershell
$env:PATH = "C:\Users\perez\.config\herd\bin\php85;" + $env:PATH
php artisan migrate
php artisan config:clear
php artisan serve --host=127.0.0.1 --port=8000
```

```powershell
php artisan reverb:start --host=127.0.0.1 --port=8080
```

```powershell
php artisan queue:work --tries=3
```

Crear el equipo en **Engines**, asignar sus zonas y emitir su credencial. También existe `php artisan engine:token <id>` para rotarla desde una terminal administrativa; imprime la credencial nueva una sola vez.

### Agente Go

Desde `music-engine`:

```powershell
$env:ENGINE_SERVER = "http://127.0.0.1:8000"
$env:ENGINE_TOKEN = "<credencial-del-equipo>"
$env:ENGINE_STATE_DIR = "C:\UtrackData\engine-01"
go run ./cmd/agent
```

`ENGINE_SERVER` es la dirección base, sin `/api/v1`. No borrar el directorio de estado para resolver una reconexión: contiene la deduplicación. Cada instancia necesita su propio directorio.

Fuera de localhost, el cliente exige HTTPS para Laravel y WSS para Reverb. La dirección anunciada en `ENGINE_WEBSOCKET_URL` debe ser accesible desde el equipo remoto. No hay reproducción automática en `cmd/agent`.

## 10. Verificaciones realizadas

**Resultado final: 40 pruebas Laravel aprobadas, 182 aserciones, sin pruebas omitidas**, incluyendo la integración real. Todas las migraciones figuran como ejecutadas en la base de datos local.

Se ejecutaron el formateo PHP (`php vendor/bin/pint --dirty --format agent`), la suite de Laravel y la integración real. La prueba de integración es optativa en la ejecución habitual y se activa indicando el binario Go:

```powershell
$env:ENGINE_E2E_BINARY = "C:\Users\perez\Desktop\Utrack\music-engine\bin\agent.exe"
php artisan test --compact
```

Las pruebas cubren autenticación, canal privado propio, rotación de credenciales, sustitución de sesión, disponibilidad por tiempo del servidor, reportes repetidos o fuera de orden, revisiones históricas, cambios de volumen/playlist, aislamiento de órdenes, resultados inmutables, relación de canciones, permisos del panel y migraciones. También se verificaron las columnas de estado observado del listado de zonas y su filtro por pertenencia al negocio.

**Go: `go mod tidy`, `go test ./...`, `go vet ./...` y `go build -o bin/agent.exe ./cmd/agent` completados correctamente.** Las pruebas de Go usan servidores HTTP/WebSocket locales de prueba y comprueban:

- Aplicación atómica de configuración y eliminación de zonas.
- Recuperación de órdenes terminadas o interrumpidas.
- Pérdida de una confirmación y reenvío del mismo resultado.
- Registro corrupto y exclusión entre procesos.
- Órdenes vencidas, revisión incorrecta, zona retirada y acción no soportada.
- Rechazo de URLs inseguras y redirecciones del API de control.
- Autorización privada, ping/pong y reconexión WebSocket.
- Recuperación de configuración después de reconectar, con un intervalo de polling largo para que no oculte un fallo del WebSocket.

### Integración real aprobada

Además de las pruebas con servidores simulados, se ejecutó `EngineReverbIntegrationTest` contra **Laravel, Reverb, worker de cola y el binario Go reales**, usando una base de datos temporal independiente. Comprobó:

1. Autenticación, versión `0.2.0`, heartbeat y suscripción al canal privado.
2. Cambio de volumen de 15 a 65 y sustitución de playlist mediante notificación WebSocket.
3. Coincidencia entre revisión enviada y aplicada.
4. Corte de Reverb y del worker, cambio de volumen a 35 y creación de una orden `stop` mientras el canal estaba desconectado.
5. Reinicio de Reverb y recuperación de configuración y orden pendiente, incluso manteniendo detenido el worker.
6. Reinicio del agente y reenvío del mismo resultado durable ante una confirmación simulada como perdida, sin cambiar su contenido ni fecha.
7. Paso a desconectado tras más de 30 segundos sin heartbeats, conservando el último estado observado.

La consulta de respaldo se configuró a 300 segundos para que no pudiera ocultar un fallo del WebSocket. Los procesos y la base temporal se cerraron y eliminaron al terminar.

### Revisión visual aprobada

Se abrió el panel en un navegador con un usuario de prueba en la instalación aislada. Se verificaron inicio de sesión, equipo conectado, versión, revisiones coincidentes, resultado `succeeded`, volumen y playlist reportados, y formulario de edición con equipo, playlist, modo y volumen correctos. La cuenta de prueba pertenecía únicamente a la base temporal eliminada al finalizar.

## 11. Cierre y límites

Los pendientes del cierre quedaron resueltos: formateo PHP, pruebas del listado de zonas, dependencias y compilación Go, migración local, integración real, revisión visual y suite de Laravel. El binario compilado está en `music-engine/bin/agent.exe`.

El bloqueo previo de la revisión automática por límite de uso ya no impidió la ejecución. Durante la preparación se corrigió el directorio de trabajo del servidor PHP de la prueba; las verificaciones posteriores aprobaron. Los servicios temporales de prueba no quedan ejecutándose. Para conectar equipos de uso diario deben arrancarse los servicios de la sección 9 y emitirse las credenciales correspondientes.

Quedan fuera de esta etapa la descarga y caché de audio, reproducción, salidas físicas multizona, ruteo de canales y Dante. Esos trabajos deberán ampliar el perfil de configuración y cumplir el contrato de reproducción antes de declarar aplicada una salida de audio.
