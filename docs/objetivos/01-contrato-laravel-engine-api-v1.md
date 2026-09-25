# Contrato Laravel ↔ Music Engine

Versión del documento: **1.0.0**, 25 de septiembre de 2026.
Estado: **contrato de referencia; implementación parcial de conexión y configuración sin audio descrita en [Objetivo 02](02-conexion-y-sincronizacion-del-engine.md)**. El perfil `configuration_only` no implementa todavía el contrato completo de reproducción de este documento.
Este archivo es la fuente de verdad compartida. Los cambios de contrato deben revisarse junto con ambos proyectos.

## 1. Responsabilidades y transporte

Laravel conserva la configuración deseada, asigna equipos a negocios, autoriza zonas, publica playlists y encola órdenes. El engine obtiene esa configuración y reporta lo que realmente pudo aplicar y reproducir. Un HTTP exitoso al encolar una orden no significa que se haya ejecutado.

El engine inicia todas las conexiones HTTPS hacia Laravel mediante `/api/v1`. No requiere un puerto entrante en el equipo. Primera versión: polling; WebSocket y streaming de audio quedan fuera de este contrato.

- Solicitudes/respuestas JSON UTF-8, `Accept: application/json` y `Content-Type: application/json` cuando haya cuerpo.
- IDs: strings opacos no vacíos, incluso si Laravel usa bigint. Ejemplo: `"42"`. No son nombres ni rutas.
- Fechas: UTC, RFC 3339, ejemplo `2026-09-25T19:00:00Z`.
- Volumen: entero de 0 a 100. El adaptador Go convierte a `float64(volume)/100`.
- Duraciones y posiciones: enteros no negativos en milisegundos.
- Campos documentados obligatorios salvo indicación de opcionales; `null` solo donde se especifica. Arreglos vacíos se transmiten como `[]`.
- Una respuesta exitosa usa `{"data": ...}`; los errores usan el sobre de la sección 8.

## 2. Equipo y credencial

Cada equipo pertenece a un negocio y tiene una credencial individual. El alta y entrega inicial de la credencial ocurren administrativamente, fuera de estos endpoints. No se permite autorregistro anónimo.

| Campo | Tipo / regla | Autoridad |
| --- | --- | --- |
| `device_id` | string estable | Laravel |
| `business_id` | string; un negocio por equipo | Laravel |
| credencial | token opaco en `Authorization: Bearer <token>` | Laravel emite/revoca |
| `engine_version` | string de versión del binario, p. ej. `0.1.0` | Engine |
| `last_seen_at` | fecha o null si nunca conectado | Laravel, hora de recepción de heartbeat válido |
| `session_id` | string opaco nuevo por arranque, entregado al abrir sesión | Laravel |

La credencial no aparece en respuestas, URLs ni logs; Laravel almacena su hash y el engine la conserva fuera del repositorio. Rotarla invalida la anterior. Cada petición se limita al equipo autenticado, sus zonas y las canciones de sus playlists; conocer un ID no concede acceso. Un equipo no cambia su negocio con el payload.

`POST /engine/sessions` abre una sesión con `{"engine_version":"0.1.0"}`. Responde 201:

```json
{
  "data": {
    "device_id": "dev-01",
    "business_id": "1",
    "session_id": "session-01",
    "last_seen_at": null,
    "heartbeat_interval_seconds": 10,
    "poll_interval_seconds": 5
  }
}
```

Todas las demás peticiones llevan `X-Engine-Session: <session_id>`. Abrir otra sesión invalida la anterior; sus peticiones reciben 409 `session_superseded`. Tras ese error el proceso se detiene y requiere intervención, evitando que dos procesos se expulsen continuamente. Un reinicio legítimo abre una sesión nueva, parte detenido y no reanuda audio automáticamente.

## 3. Configuración deseada: zona, playlist y canción

`GET /engine/config` devuelve una instantánea completa de las zonas asignadas al equipo. `config_revision` es un entero positivo, creciente por equipo; aumenta ante cambios de zona, asignación, playlist, orden, formato o contenido de audio. La renovación de una URL firmada no cambia la revisión.

```json
{
  "data": {
    "device_id": "dev-01",
    "config_revision": 1,
    "zones": [
      {
        "zone_id": "10",
        "name": "Lobby",
        "volume": 65,
        "channel_mode": "stereo",
        "output": {"device_id": "windows-output-01", "channels": [1, 2]},
        "playlist": {
          "playlist_id": "20",
          "name": "Jazz",
          "songs": [
            {
              "song_id": "30",
              "audio_url": "https://media.example.test/songs/30.mp3",
              "url_expires_at": null,
              "content_version": "1",
              "format": {
                "container": "mp3",
                "codec": "mp3",
                "mime_type": "audio/mpeg",
                "sample_rate_hz": 44100,
                "channels": 2,
                "duration_ms": 180000,
                "bit_rate_bps": 192000
              }
            }
          ]
        }
      }
    ]
  }
}
```

Reglas normativas:

- `playlist` puede ser null; `songs` puede estar vacío. En ambos casos se detiene la zona y `play`, `next` y `previous` fallan con `empty_playlist`.
- El orden del arreglo `songs` es el orden de reproducción; Laravel ordena por `playlist_song.position`, luego por `songs.id`. Una canción no se repite dentro de una playlist en v1.
- `channel_mode`: `mono` o `stereo`. `output` puede ser null (zona sin salida, detenida). Su `device_id` identifica una salida local, distinto del ID del equipo; debe corresponder a una salida disponible en ese engine.
- Canales físicos numerados desde **1**: mono exige un canal; estéreo exige dos distintos en orden izquierda/derecha. Mono mezcla `(L+R)/2`; una fuente mono se duplica en estéreo. No se permite compartir un canal entre zonas del mismo dispositivo de salida y equipo en v1.
- `format.channels` describe la fuente (1 o 2), no la salida. `sample_rate_hz` es positivo. `duration_ms` y `bit_rate_bps` pueden ser null si se desconocen. MP3 es el único formato previsto para esta primera implementación; WAV requiere ampliar el soporte del engine antes de asignarlo.
- `audio_url` es HTTPS descargable sin enviar la credencial del equipo al servidor de medios. Puede ser una URL firmada; `url_expires_at` es fecha o null. No se transmiten rutas locales de Laravel.
- `content_version` cambia cada vez que cambian los bytes. Caché identificada por `(song_id, content_version)`, nunca solo por URL. Una descarga incompleta no reemplaza un archivo válido.
- Ante URL caducada (403), el engine vuelve a obtener config y reintenta una vez con la URL renovada; si falla informa `audio_download_failed`. Laravel debe renovar URLs conservando IDs, contenido y revisión.
- Un cambio de volumen se aplica sin reiniciar la pista. Cambiar playlist, su contenido/orden o salida detiene la zona y selecciona la primera canción, sin reproducirla hasta recibir una orden. Una zona retirada de la instantánea se detiene y elimina localmente.
- El engine valida y prepara toda la instantánea antes de aplicarla. Si no puede aplicarla, conserva la revisión previa y reporta `config_error`; nunca declara aplicada una revisión parcial. Una salida/capacidad no soportada produce `unsupported_output` o `unsupported_format`, sin sustitución silenciosa.
- No hay autoplay, scheduler ni avance automático al terminar una canción en v1. Al finalizar pasa a `stopped` conservando la selección.

## 4. Control y entrega de órdenes

Laravel crea las órdenes desde su capa autenticada de administración; esa API de usuarios está fuera de este contrato de equipo. Cada orden tiene ID único y secuencia creciente por equipo. Solo se encolan acciones para zonas asignadas y para la revisión actual.

`GET /engine/commands` devuelve 200 con `data` como arreglo de hasta 100 órdenes pendientes, ordenadas por `sequence`; vacío significa que no hay trabajo. Consultarlas no las confirma ni elimina.

```json
{
  "data": [
    {
      "command_id": "cmd-01",
      "sequence": 1,
      "zone_id": "10",
      "config_revision": 1,
      "action": "play",
      "created_at": "2026-09-25T19:00:00Z",
      "expires_at": "2026-09-25T19:01:00Z"
    }
  ]
}
```

| Acción | Semántica |
| --- | --- |
| `play` | Reproduce desde el inicio la selección actual; primera canción si no hay selección. |
| `pause` | Pausa conservando posición; si ya está pausado, éxito sin cambio. Sin pista cargada falla `no_track`. |
| `resume` | Reanuda desde la posición conservada; si ya reproduce, éxito sin cambio. Sin pista cargada falla `no_track`. |
| `stop` | Detiene y libera audio, conserva selección y volumen; repetida no cambia nada. |
| `next` / `previous` | Selecciona y reproduce la siguiente/anterior circularmente; con una canción la reinicia. |

El volumen y el ruteo se cambian exclusivamente mediante configuración, evitando dos fuentes de autoridad. No se aceptan parámetros adicionales de ejecución ni rutas arbitrarias en las órdenes v1.

El engine ejecuta secuencialmente. Primero consulta/aplica config, luego consume órdenes. Una orden para revisión distinta de la aplicada se rechaza con `config_revision_mismatch`; una zona retirada con `zone_not_assigned`; una orden vencida con `command_expired`. No se ejecutan automáticamente órdenes fallidas al corregir la configuración: se crea una nueva.

Cada resultado se confirma con `PUT /engine/commands/{command_id}/result`:

```json
{
  "outcome": "succeeded",
  "completed_at": "2026-09-25T19:00:02Z",
  "error": null
}
```

`outcome` es `succeeded` o `failed`; para fallo, `error` contiene `code` y `message` strings no vacíos. Respuesta 200: `{"data":{"command_id":"cmd-01","outcome":"succeeded"}}`. El resultado es terminal e inmutable: reenviar exactamente el mismo cuerpo responde 200; otro resultado para ese ID responde 409 `result_conflict`. Laravel valida propiedad y pertenencia de la orden al equipo.

La entrega es **al menos una vez**. El engine mantiene un registro durable por `command_id`: escribe `started` antes de tocar el player y el resultado antes de confirmarlo. Reentregar un ID terminado solo reenvía el resultado; no vuelve a reproducir ni saltar una pista. Tras un reinicio, un registro `started` sin resultado se cierra como fallo `execution_unknown`, sin repetir la acción. Esto evita prometer ejecución exactamente una vez ante una caída durante una acción. Laravel conserva órdenes sin resultado para reentrega, incluso vencidas, hasta recibir su resultado de expiración. El engine conserva deduplicación mientras Laravel pueda reentregar una orden; v1 no purga automáticamente esos registros.

## 5. Estado observado y última conexión

`POST /engine/heartbeat` cada 10 segundos y tras cambios de estado envía una instantánea completa del estado local. Un solo envío a la vez; `report_sequence` empieza en 1 por sesión y aumenta. Laravel ignora reportes más antiguos y acepta reintentos idénticos sin sustituir datos más nuevos; misma secuencia con otro cuerpo produce 409 `report_conflict`.

```json
{
  "report_sequence": 1,
  "observed_at": "2026-09-25T19:00:03Z",
  "applied_config_revision": 1,
  "config_error": null,
  "zones": [
    {
      "zone_id": "10",
      "state": "playing",
      "playlist_id": "20",
      "song_id": "30",
      "position_ms": null,
      "volume": 65,
      "channel_mode": "stereo",
      "output": {"device_id": "windows-output-01", "channels": [1, 2]},
      "error": null
    }
  ]
}
```

Respuesta 200: `{"data":{"last_seen_at":"2026-09-25T19:00:03Z"}}`, calculada por Laravel, nunca copiada del reloj del engine. Tras 30 segundos sin heartbeat válido se considera el equipo desconectado; se conserva el último estado y se muestra como desactualizado. **Desconectado no significa detenido**.

- `applied_config_revision`: entero no negativo; 0 antes de la primera aplicación. `config_error`: null o `{ "revision": 2, "code": "unsupported_output", "message": "Salida no disponible" }`; identifica la revisión deseada que falló.
- `zones` reporta las zonas de la revisión realmente aplicada, incluso si una revisión nueva falló. Laravel debe conservar el contexto de revisiones emitidas para validar estos reportes; no interpreta zonas omitidas como detenidas. Antes de aplicar la primera revisión se envía `[]`.
- `playlist_id`, `song_id` y `position_ms` pueden ser null. La posición es null si el backend no la mide; no se inventa. Tras stop se conserva `song_id` como selección, con posición 0. `output` y `channel_mode` pueden ser null si todavía no existe salida aplicada.
- `volume`, modo y salida reflejan valores aplicados, nunca una copia optimista de lo deseado.
- `error` es null o `{ "code": "audio_device_failed", "message": "Salida no disponible" }`. Si falla una orden pero la pista anterior sigue sonando, su estado sigue siendo `playing`; el fallo pertenece al resultado de esa orden.

| Estado | Significado |
| --- | --- |
| `playing` | El backend está reproduciendo. |
| `paused` | Pista cargada y posición conservada. |
| `stopped` | Sin reproducción activa; incluye inicio, stop y fin de pista. |
| `loading` | Preparando/descargando audio para una reproducción solicitada. |
| `error` | Un fallo impide reproducir; requiere `error` no null. |

Transiciones: `stopped → loading → playing`, `playing ↔ paused`, cualquier estado → `stopped` por stop, `playing → stopped` por fin y `loading/playing/paused → error` por fallo que interrumpe audio. Una nueva orden válida permite `error → loading → playing`. Descargar en segundo plano sin interrumpir una pista no cambia su estado a `loading`.

## 6. Ciclo de sincronización y desconexión

1. Abrir sesión; iniciar heartbeat y obtener configuración completa.
2. Validar/aplicar configuración; reportar revisión aplicada o error.
3. Cada 5 segundos consultar config y luego órdenes; no solapar ciclos.
4. Registrar/ejecutar cada orden, guardar resultado, confirmarlo y reportar estado. El heartbeat continúa durante descargas.
5. Sin red, continuar la reproducción local actual y conservar resultados pendientes. No inventar órdenes ni iniciar automáticamente la siguiente canción.
6. Al reconectar, obtener config antes de consumir órdenes; reenviar resultados persistidos. Un proceso que no reinició conserva su sesión.

Timeout HTTP de control: 10 segundos. Reintentar fallos de red/5xx con espera exponencial de 1, 2, 4, 8, 16 y máximo 30 segundos, con variación aleatoria de hasta 20%. Un 429 respeta `Retry-After` en segundos. Un 401 exige corregir/renovar la credencial y suspende polling; 403 exige corregir permisos. Errores de validación no se reintentan sin corregir el payload. El audio local no se detiene por pérdida de red o autenticación. El reloj del equipo debe estar sincronizado para evaluar vencimientos.

## 7. Resumen de endpoints

Todas las rutas de esta tabla llevan el prefijo `/api/v1` y autenticación Bearer.

| Método | Ruta | Resultado |
| --- | --- | --- |
| POST | `/engine/sessions` | 201 identidad, sesión y tiempos de consulta |
| GET | `/engine/config` | 200 configuración completa |
| GET | `/engine/commands` | 200 arreglo ordenado de pendientes |
| PUT | `/engine/commands/{command_id}/result` | 200 confirmación idempotente |
| POST | `/engine/heartbeat` | 200 recepción y última conexión |

## 8. Errores y compatibilidad

```json
{
  "error": {
    "code": "validation_failed",
    "message": "El volumen debe estar entre 0 y 100.",
    "details": [{"field": "zones.0.volume", "message": "Fuera de rango."}],
    "request_id": "request-01"
  }
}
```

`details` siempre es un arreglo (puede estar vacío); `request_id` permite correlación sin exponer tokens. Códigos HTTP: 400 JSON inválido, 401 credencial inválida/revocada, 403 acceso denegado, 404 recurso inexistente o fuera del ámbito, 409 conflicto de sesión/secuencia/resultado, 422 validación, 429 límite, 500/503 fallo temporal. Los errores de ejecución de audio se entregan como resultados `failed`, no como errores HTTP del envío de un resultado válido.

En `/api/v1` solo se añaden campos opcionales; consumidores ignoran campos desconocidos y proveedores no exigen esos campos a clientes anteriores. No se renombran/eliminan campos ni se cambian tipos, unidades, nulabilidad, semántica o valores de enums existentes. Una acción desconocida se rechaza como `unsupported_action`, nunca se ejecuta por aproximación. Ampliar acciones, formatos o estados exige negociación explícita de capacidades en una evolución acordada o `/api/v2`; no se asume soporte por la versión del binario. Cambios incompatibles requieren `/api/v2`, convivencia durante la migración y fecha de retiro acordada. La versión de este documento es independiente de `engine_version`.

## 9. Correspondencia con los proyectos y trabajo pendiente

| Existente | Adaptación requerida |
| --- | --- |
| Laravel: `zones.id`, `playlist_id`, `volume` | Serializar IDs como strings; validar negocio, rango y asignación al equipo. |
| Laravel: `output_channel` string nullable | Modelar salida y canales estructurados; no asumir que `"1-2"` identifica hardware. Añadir modo mono/estéreo. |
| Laravel: `playlist_song.position` | Publicar canciones en el orden definido. |
| Laravel: `songs.file_path` | Resolver URL descargable y extraer formato/versionar contenido. La UI admite WAV; bloquear su asignación al engine MP3. |
| Laravel sin modelo de equipo ni rutas API de engine | Añadir equipos, tokens, sesiones, revisiones, órdenes/resultados, reportes y autorización. |
| Go: `PLAYING`, `PAUSED`, `STOPPED` | Traducir a minúsculas; adaptador incorpora `loading` y `error` cuando corresponda. |
| Go: volumen 0..1, paths locales | Convertir porcentaje; descargar y cachear URLs antes de llamar al player. |
| Go: contexto Oto con frecuencia del primer MP3 | Validar compatibilidad de frecuencia antes de aplicar; no hay resampling actual. |
| Go sin HTTP, zonas, ruteo ni persistencia de órdenes | Añadir cliente, registro durable, gestión de zonas y backend capaz de seleccionar canales. |

Definir mono/estéreo y canales en este contrato no implica que Oto actual soporte ruteo multizona o Dante. Esa capacidad debe implementarse y comprobarse antes de declarar aplicada tal configuración.

## 10. Criterios para verificar la implementación futura

- Un token solo obtiene configuración y órdenes de su propio equipo/negocio.
- Un cambio deseado no altera el estado observado hasta recibir un reporte válido.
- Volumen 65 se aplica como 0.65; canales `[1,2]` preservan izquierda/derecha.
- Reentregar `next` y repetir su confirmación no salta dos canciones; caída entre acción y persistencia produce `execution_unknown`.
- Órdenes vencidas, revisiones diferentes y salidas no soportadas devuelven errores explícitos.
- Revisión fallida conserva configuración aplicada previa; reportes fuera de orden no retroceden el estado.
- Playlist vacía y zona eliminada detienen audio; fin de pista no avanza automáticamente.
- Se renueva una URL caducada sin confundirla con un cambio de contenido.
- Desconexión conserva audio y muestra estado desactualizado; reconexión sincroniza antes de ejecutar órdenes.

El alcance de esta entrega es el contrato documentado. Estos escenarios son criterios de aceptación, no pruebas de integración ya ejecutadas.
