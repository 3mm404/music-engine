# Rendimiento y recuperación

## Línea base reproducible del mezclador PCM

Medición local del 6 de noviembre de 2026 sobre el router de la revisión `b220eaa` en Windows x64, AMD Ryzen 5 5600H y Go 1.27.2. Cada operación lee un bloque de **256 cuadros** del router con fuentes PCM estéreo sintéticas a **44.1 kHz** y una salida simulada. Los tiempos son la mediana y el rango de los promedios de tres ejecuciones.

| Zonas activas | Mediana por bloque | Rango por bloque | Bytes asignados por bloque | Asignaciones por bloque |
| --- | ---: | ---: | ---: | ---: |
| 2 | 27.15 µs | 26.74–27.64 µs | 0 B | 0 |
| 8 | 68.08 µs | 66.87–71.74 µs | 0 B | 0 |

```powershell
go version
go test -run '^$' -bench '^BenchmarkRoutePCM$' -benchmem -count=3 ./internal/audio/mixer
```

Un bloque de 256 cuadros a 44.1 kHz representa unos 5.80 ms de audio. El benchmark mide el tiempo transcurrido en el router y las asignaciones Go durante la lectura, con las zonas ya configuradas. La latencia audible, la memoria residente del proceso y las interrupciones durante sesiones largas requieren medición con el agente, la carga y el dispositivo reales.

## Estabilidad y fallos reproducibles

| Escenario | Resultado que exige la prueba |
| --- | --- |
| Retirar rutas y volver a configurarlas | La salida entrega silencio durante el cambio y recupera el ruteo. |
| Una zona llega al final de su fuente | La otra zona continúa reproduciendo. |
| El callback ASIO se queda sin cuadros | La salida entrega silencio. |
| El proceso recupera el diario o se pierde una confirmación | La orden ya procesada no se ejecuta de nuevo. |
| Se corta la conexión WebSocket | El agente se reconecta y recupera cambios de configuración. |

```powershell
go test -count=10 -run 'Test(ClearingRoutes|MonoRouting)' ./internal/audio/mixer
go test -count=10 -run 'Test(CallbackUnderrun|FrameRingUnderrun)' ./internal/audio/asio
go test -count=1 -run 'Test(JournalRecovers|LostAcknowledgment|RuntimeRestoresChanges)' ./internal/control
```

## Medición con hardware

Usa un Windows x64 y registra versión de Engine, modelo del equipo y dispositivo, driver, frecuencia, tamaño del búfer, cantidad de zonas y origen del audio. Conserva el mismo MP3 y configuración en cada comparación.

1. **Latencia audible.** Registra la orden local y el primer audio capturado por loopback con un reloj común. Separa arranques con MP3 en caché de arranques con descarga. Ejecuta al menos 30 intentos por condición y reporta mediana, p95 y fallos.
2. **Estabilidad y memoria.** Reproduce durante 60 minutos. Guarda cada cinco segundos el conjunto de trabajo y los bytes privados de `engine.exe`. Reporta mediana, pico, diferencia entre inicio y fin, reinicios y cortes de audio observados.
3. **Recuperación.** Durante esa sesión, interrumpe la conexión al servidor y restablécela. Registra el tiempo hasta recuperar un estado actualizado, la continuidad de audio y si se repite una orden. Prueba Oto y ASIO en sesiones separadas.
