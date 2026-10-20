# Objetivo 07 — Desacoplar la salida de audio

**Estado: cumplido.** Refactor interno de `music-engine` para separar el reproductor del backend de audio actual, manteniendo Oto como backend predeterminado. Este objetivo prepara un punto de extensión; no implementa ASIO ni distribucion Dante.

## Alcance implementado

- Se definio `audio.Output` como salida PCM compartida, con `Open` para crear un stream logico por reproduccion.
- `audio.PCMFormat` expresa frecuencia de muestreo, cantidad de canales y formato de muestra (`SignedInt16LE`).
- `audio.Stream` expone las operaciones que requiere Player: reproducir, pausar, consultar actividad y errores, y ajustar volumen.
- La implementacion Oto v3 se movio a `internal/audio/oto`. Conserva un contexto Oto compartido por proceso, buffer de 50 ms y streams Oto para las pistas.
- Player recibe `audio.Output`. Los constructores actuales conservan Oto como valor predeterminado; las variantes `WithOutput` permiten inyectar una implementacion distinta.
- Manager comparte una misma instancia de `audio.Output` entre los reproductores de zona y tambien permite inyectarla mediante `NewWithOutput`, `NewRemoteWithOutput` y `NewRemoteMonoWithOutput`.
- La prueba de volumen ya observa el contrato de audio, en lugar de inspeccionar directamente `oto.Player`.

## Flujo de audio actual

```text
Manager (salida compartida)
  └─ Player por zona
       └─ decoder.Stream (MP3 a PCM int16 little-endian)
            └─ audio.Output / audio.Stream
                 └─ audio/oto.Output
                      └─ contexto Oto compartido y oto.Player por stream
```

El Player conserva la coordinacion de pista, Play/Pause/Resume/Stop, Next/Previous, estado y volumen de zona. La descarga HTTPS y la decodificacion MP3 permanecen separadas de la salida. El modo mono actual mezcla los dos canales MP3 y duplica la muestra resultante en ambos canales del stream estereo; no representa aun un canal fisico mono ruteable.

## Preparacion y limites para ASIO / Dante

La interfaz describe streams PCM y un backend compartido, no una tarjeta de audio independiente por zona. Una etapa futura puede insertar un mixer/router que acepte los streams logicos de las zonas y entregue un unico stream PCM multicanal al backend ASIO.

Todavia no existen mixer, asignacion de canales por zona, configuracion ASIO, seleccion de dispositivo, gestion de ciclo de vida ASIO ni ruteo Dante/Q-SYS. Player actualmente solicita PCM int16 little-endian estereo; la presencia de un campo de canales en el formato no significa que la ruta de reproduccion actual ya produzca multicanal. No se agregaron CGo, SDKs ni dependencias ASIO.

## Verificacion

- `go test -count=1 ./...`: aprobado.
- `go vet ./...`: aprobado.
- Pruebas focalizadas de `internal/player` e `internal/engine`: aprobadas.
- Las pruebas de integracion que requieren audio real se omitieron porque `MUSIC_ENGINE_TEST_MP3` no estaba configurado; por ello, esta verificacion no afirma reproduccion fisica en un dispositivo.
