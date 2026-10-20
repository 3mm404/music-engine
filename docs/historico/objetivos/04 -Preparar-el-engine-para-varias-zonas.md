# Objetivo 04 — Preparar el engine para varias zonas

Fecha de cierre y verificación: **26 de septiembre de 2026**.

**Estado: completado y verificado para reproducción local.** El ejecutable usa dos zonas A/B y el administrador admite N zonas independientes sin depender de la consola. El agente conectado a Laravel conserva el perfil `configuration_only`.

## Resultado alcanzado

- Se agregó un `Manager` encargado de administrar múltiples zonas.
- Cada zona mantiene su propio `Player`.
- Cada zona tiene canción, volumen y estado independientes.
- Las acciones `Play`, `Pause`, `Resume`, `Stop`, `Next` y `Previous` se ejecutan sobre una zona específica.
- Detener o modificar una zona no afecta la reproducción de las demás.
- La arquitectura admite N zonas, no únicamente dos.
- El ciclo de vida de los reproductores ya no depende de `console.Run()`.
- `Manager.Close()` detiene todos los reproductores y evita que una zona cerrada vuelva a iniciar reproducción.
- Las operaciones de cada zona están protegidas independientemente mediante sincronización.
- `cmd/engine` utiliza el administrador; la consola permite seleccionar zona mediante `zone ID` y consultar todas mediante `zones`.
- El modo `-headless` reproduce sin leer stdin y mantiene el proceso activo hasta su cancelación.

La estructura resultante es:

    Engine / Manager
      ├── Zone A → Player A
      ├── Zone B → Player B
      └── Zone N → Player N

## Implementación

La administración multizona se encuentra principalmente en:

    internal/engine/

El `Manager` mantiene las zonas mediante:

    map[string]*Zone

Cada `Zone` encapsula su propio reproductor:

    Zone
      └── playback
            └── player.Player

Esto permite que cada reproductor mantenga su propio estado sin exponer directamente la implementación interna del `Player`.

### Archivos creados o modificados

Rutas relativas al repositorio `music-engine`:

| Archivo | Cambio |
| --- | --- |
| `internal/engine/manager.go` | Administrador, handles por zona, supervisión y cierre de todos los players. |
| `internal/engine/manager_test.go` | Pruebas de aislamiento, ciclo de vida, monitor y reproducción simultánea. |
| `cmd/engine/main.go` | Sustitución del player único por A/B; flags `-file-b` y `-headless`; cierre del administrador. |
| `internal/console/console.go` | Selección de zona, listado de estados y comandos dirigidos al player correspondiente. |
| `internal/console/console_test.go` | Selección A/B, ID desconocido, volumen independiente y EOF. |
| `README.md` | Instrucciones de uso, arquitectura, verificaciones y límites. |

Se reutilizaron `internal/player` y el backend Oto existentes. No se modificaron dependencias, código PHP, migraciones ni el contrato remoto. El [CHECKPOINT del workspace](../../CHECKPOINT.md) registra el estado de continuidad.

## Independencia entre zonas

Se verificó que una zona pueda modificar su volumen o transporte sin modificar el estado de otra.

Ejemplo conceptual:

    Zone A
      Track: song-a.mp3
      Volume: 0.05
      State: PLAYING

    Zone B
      Track: song-b.mp3
      Volume: 0.08
      State: PLAYING

Una operación como:

    Zone A → Stop

no debe modificar:

    Zone B → PLAYING

## Ejecución sin consola

El `Manager` mantiene un monitor interno que consulta periódicamente el estado de los reproductores.

Por lo tanto, el funcionamiento del runtime multizona no depende de:

    console.Run()

El monitor consulta el estado cada 100 ms y libera archivos al detectar EOF o errores. Un error de una zona queda observable sin finalizar las otras. No hay avance automático al terminar una canción.

La consola es una interfaz opcional. Salir de `console.Run()` no cierra los players; el ejecutable, como propietario del administrador, llama a `Manager.Close()` al terminar. Los handles retenidos rechazan nuevas operaciones después del cierre con `engine.ErrClosed`.

### Puesta en marcha

Desde `music-engine`, en PowerShell:

```powershell
# A reproduce demo.mp3; B comienza detenida.
go run ./cmd/engine

# Dos pistas al iniciar.
go run ./cmd/engine -file "music/demo.mp3" -file-b "music/Drums.mp3"

# Reproducción sin consola interactiva.
go run ./cmd/engine -headless -file "music/demo.mp3" -file-b "music/Drums.mp3"
```

Las rutas relativas se resuelven desde el directorio de trabajo. Cada zona carga la lista de MP3 de la carpeta de su pista inicial; si no se indica `-file-b`, B usa la lista de A. Los IDs distinguen mayúsculas y deben ser únicos y no vacíos. Se pueden configurar más zonas al construir `engine.New([]engine.ZoneConfig{...})`; no hay altas o bajas en caliente.

Ejemplo en consola:

```text
zone B
play music/Drums.mp3
volume 35
zone A
stop
zones
```

El resultado esperado es A detenida y B reproduciendo con volumen 35%. Los comandos existentes `play`, `pause`, `resume`, `next`, `previous`, `stop`, `volume` y `state` operan sobre la zona seleccionada. Un ID desconocido informa error y conserva la selección anterior.

En modo interactivo, `quit`, EOF o Ctrl+C terminan el ejecutable y cierran todas las zonas. En modo `-headless`, stdin cerrado no termina el proceso; este espera Ctrl+C incluso después del fin de las pistas. No instala un servicio de Windows.

## Alcance actual

Este objetivo prepara exclusivamente el runtime multizona del Music Engine.

Todavía no conecta las zonas sincronizadas desde Laravel con estos reproductores.

Las zonas son independientes a nivel de reproducción, pero **Oto mezcla sus streams en la misma salida predeterminada de Windows**. No representan todavía dispositivos físicos separados. El contexto compartido usa la frecuencia del primer MP3 y rechaza pistas incompatibles sin reemplazar la pista actual. No hay remuestreo. Un fallo del dispositivo compartido puede afectar todas las zonas y el audio ya enviado a Windows puede sonar brevemente después de Stop.

Quedan fuera de este objetivo:

- Integración `cmd/agent` → `engine.Manager`.
- Descarga y caché de canciones.
- Resolución de archivos desde Laravel/S3.
- Asignación de canales de salida físicos.
- Dante.
- Q-SYS.
- Scheduler.

## Verificación del cierre

Desde `music-engine` se ejecutó:

    go test ./...
    go vet ./...

Resultado:

    PASS

Los paquetes principales relacionados con reproducción y multizona aprobaron:

    ok music-engine/internal/engine
    ok music-engine/internal/player

Las pruebas verifican:

- rechazo de IDs de zona inválidos o duplicados;
- aislamiento de zonas;
- volumen independiente;
- cierre concurrente seguro;
- imposibilidad de reiniciar una zona después de cerrar el Manager;
- funcionamiento del monitor sin consola;
- aislamiento de errores entre zonas;
- soporte para reproducción independiente de dos Players.

La prueba de integración con audio real **sí se ejecutó con `MUSIC_ENGINE_TEST_MP3` apuntando a `music/demo.mp3` y aprobó**. Sin esa variable, la suite habitual omite las pruebas optativas de hardware.

Comandos completos de cierre, desde `music-engine`:

```powershell
$env:GOCACHE = Join-Path $env:TEMP 'utrack-go-build'
go test ./...
go vet ./...
go build -o bin/music-engine.exe ./cmd/engine
go build -o bin/agent.exe ./cmd/agent
$env:MUSIC_ENGINE_TEST_MP3 = (Resolve-Path music/demo.mp3).Path
go test ./internal/engine ./internal/player -v -count=1
```

Todos aprobados. En la ejecución final, `internal/engine` terminó en 1.662 s y `internal/player` en 1.154 s, sin omisiones en esos paquetes con la variable definida.

La integración inicia dos streams reales y comprueba que pausa, reanudación, navegación, volumen, Stop y archivo inválido en A conservan B reproduciendo. También comprueba que Stop de B conserva A y que Close detiene ambos. Verifica estados y funcionamiento contra Oto real, no separación acústica entre dispositivos.

Se comprobó además el binario con `-headless`, dos pistas y stdin cerrado: permaneció activo después de 1.5 segundos. El proceso de prueba se terminó y liberó al finalizar; esta comprobación no prueba la entrega del evento Ctrl+C de Windows. Una sesión de consola canalizada mostró A `STOPPED` al 100% y B `PLAYING` al 10% después de detener A.

### Limitación de verificación

`go test -race ./internal/engine ./internal/console` no pudo compilar `runtime/cgo`: el GCC disponible es Cygwin y Go necesita un compilador compatible con Windows nativo, como MinGW. No se instaló ni cambió el compilador. La prueba concurrente normal aprobó, pero la ejecución con detector de carreras queda pendiente de ese requisito del entorno.

## Continuidad hacia el siguiente objetivo

Si el siguiente objetivo solicita audio remoto, el paso recomendado es integrar las zonas recibidas por el agente desde Laravel con `engine.Manager`, definiendo primero la resolución local de canciones. Esta integración no está iniciada y requiere su propio alcance y verificación.

La dirección esperada será:

    Laravel
       │
       ▼
    Reverb / API
       │
       ▼
    Go Agent
       │
       ▼
    engine.Manager
       │
       ├── Zone A → Player A
       ├── Zone B → Player B
       └── Zone N → Player N

Se debe reutilizar la sincronización implementada en los objetivos 02 y 03 y el runtime multizona implementado en este objetivo.

No se debe reimplementar Reverb, la autenticación del equipo, la deduplicación de comandos ni `player.Player`.
