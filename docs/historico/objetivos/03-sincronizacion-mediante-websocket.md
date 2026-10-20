# Objetivo 03 — Sincronizar mediante WebSocket

Fecha de cierre y verificación: **25 de septiembre de 2026**.

**Estado: completado y verificado para el perfil `configuration_only`.** Se puede continuar con el objetivo 04. No existe un bloqueo por tokens o créditos ni una implementación WebSocket pendiente de terminar.

## Resultado alcanzado

- Laravel notifica cambios mediante Reverb en `private-engines.{device_id}`. La autorización usa la credencial y sesión del equipo y rechaza canales ajenos.
- El evento `engine.changed` indica al agente Go que consulte la configuración vigente. El agente aplica volumen, playlist y zonas a su estado de configuración y lo reporta mediante heartbeat.
- Las órdenes pendientes se consultan después de la configuración y se confirman con un resultado terminal.
- Ante una pérdida de conexión, el cliente reconecta con espera creciente. Al recuperar la suscripción obtiene la configuración y las órdenes pendientes, incluso si no recibió sus avisos originales.
- El registro durable evita repetir una orden: reenvía el resultado guardado. Si hubo un inicio sin resultado persistido, informa `execution_unknown` sin repetir la acción.

La implementación se realizó junto con el objetivo 02 y se reutilizó en esta revisión. La arquitectura, endpoints, preparación y comandos de arranque están en [Objetivo 02](02-conexion-y-sincronizacion-del-engine.md). El contrato general está en [Objetivo 01](01-contrato-laravel-engine-api-v1.md).

## Alcance exacto y trabajo pendiente de audio

Cambiar volumen o playlist desde Laravel **actualiza el estado del agente**, pero todavía no cambia el sonido de una salida física. `cmd/agent` no está integrado con el reproductor de consola `cmd/engine`.

El perfil `configuration_only` mantiene las zonas detenidas, sin pista cargada ni posición de reproducción. La orden `stop` es válida para este estado; las acciones de reproducción devuelven `unsupported_action`. No se afirma ejecución exactamente una vez ante cualquier caída: hay entrega repetible con deduplicación durable.

Quedan fuera del cierre de este objetivo la descarga/caché de audio, reproducción física, salidas multizona, scheduler y Dante. Este límite es de alcance, no de tokens, créditos o trabajo interrumpido.

## Verificación del cierre

- **Laravel: 40 pruebas aprobadas, 182 aserciones, ninguna omitida**, habilitando la integración real con `ENGINE_E2E_BINARY`.
- La integración levantó Laravel, Reverb, worker y agente Go reales sobre una base temporal. Comprobó cambio de volumen y playlist, corte y reinicio de Reverb, recuperación de una orden pendiente y reenvío del mismo resultado después de reiniciar el agente.
- El polling de respaldo se elevó a 300 segundos para demostrar que WebSocket y la reconexión producen las actualizaciones.
- **Go:** `go test ./...`, `go vet ./...` y compilación de `cmd/agent` y `cmd/engine` aprobados.
- Se corrigió una carrera en `TestWebSocketPrivateAuthPingAndReconnect`: la prueba ahora espera el pong antes de cancelar la conexión. Aprobó diez ejecuciones consecutivas.

Para repetir desde `music-engine`, en PowerShell:

```powershell
$env:GOCACHE = Join-Path $env:TEMP 'utrack-go-build'
go test ./internal/control -run TestWebSocketPrivateAuthPingAndReconnect -count=10
go test ./...
go vet ./...
go build -o bin/agent.exe ./cmd/agent
go build -o bin/music-engine.exe ./cmd/engine
```

Desde `utrack-fly`:

```powershell
$env:ENGINE_E2E_BINARY = 'C:/Users/perez/Desktop/Utrack/music-engine/bin/agent.exe'
& 'C:/Users/perez/.config/herd/bin/php85/php.exe' artisan test --compact
```

La ruta PHP requiere acceso fuera del sandbox en este entorno. La prueba real limpia sus procesos y base temporal. No deja servicios permanentes iniciados.

## Continuidad hacia el objetivo 04

1. Partir de este cierre y del [CHECKPOINT del workspace](../../CHECKPOINT.md); no reinstalar ni reimplementar Reverb, el cliente privado o la deduplicación.
2. Revisar el enunciado del objetivo 04 antes de comenzar: todavía no está documentado en esta carpeta y este cierre no define su alcance.
3. Reutilizar `music-engine/internal/control/` para sincronización y `internal/player/` para el reproductor existente si el siguiente objetivo requiere audio. Mantener explícito `configuration_only` hasta implementar y verificar cualquier perfil de reproducción nuevo.
4. Conservar `ENGINE_STATE_DIR`: contiene la deduplicación. Mantener pruebas de reconexión y confirmaciones al extender el runtime.
5. Aplicar las reglas de continuidad: etapas pequeñas, verificaciones relevantes y actualización del checkpoint después de cada etapa importante.
