# Objetivo 04 — Preparar el engine para varias zonas

Fecha de cierre y verificación: **25 de septiembre de 2026**.

**Estado: completado y verificado.** El Music Engine ya puede administrar múltiples zonas de reproducción independientes sin depender de la consola.

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

La consola existente puede mantenerse como herramienta de desarrollo, pero deja de ser responsable del ciclo de vida de los reproductores.

## Alcance actual

Este objetivo prepara exclusivamente el runtime multizona del Music Engine.

Todavía no conecta las zonas sincronizadas desde Laravel con estos reproductores.

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

La prueba de integración con audio real está condicionada a:

    MUSIC_ENGINE_TEST_MP3

por lo que únicamente debe considerarse verificada cuando se ejecute proporcionando un MP3 válido.

## Continuidad hacia el siguiente objetivo

El siguiente paso será integrar las zonas recibidas por el agente desde Laravel con `engine.Manager`.

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