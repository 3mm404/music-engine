# Objetivo 08 — Mixer / Router PCM multicanal

**Estado: cumplido mediante pruebas PCM deterministas.** El Engine ahora puede enrutar varias zonas a canales distintos de un único stream PCM multicanal. No se implementaron ASIO ni Dante y no se afirma funcionamiento físico multicanal con Oto.

## Configuración reutilizada

El Engine ya recibe por zona `output.channels` como una lista de enteros dentro de `Output`; por ejemplo, `[1, 2]`. No existe en el Go actual un campo separado llamado `output_channel` ni se añadió otro mecanismo de configuración. `control.applyAudio` propaga la lista existente a `engine.ZoneConfig.OutputChannels`.

Las rutas usan canales de salida numerados desde 1 y en orden: el primer canal recibe L y el segundo R. Una ruta de un canal promedia L/R. Se exige una ruta por cada zona o ninguna, entre 1 y 64, sin canales duplicados ni superpuestos. El modo de cada zona determina si debe asignar uno o dos canales. El perfil mono sigue aceptando solo zonas mono; el perfil estéreo permite zonas mono o estéreo.

## Flujo implementado

```text
Configuración de Engine
  ↓ output.channels por zona
Manager
  ↓ salida lógica por zona
Player independiente
  ↓ PCM int16 little-endian
Mixer / Router compartido
  ↓ un stream intercalado con max(channel asignado) canales
Audio backend inyectado
```

El router conserva la interfaz `audio.Output` del backend. Cada wrapper de zona registra su PCM y conserva estado y ganancia independientes. El router genera frames continuos una vez abierta la salida; zonas pausadas, detenidas, terminadas o sin señal dejan en silencio únicamente sus canales. El fin de un stream no se propaga a los demás.

Sin asignaciones de canales, Manager mantiene la ruta directa anterior a través del backend inyectado. En modo enrutado, las asignaciones deben configurarse antes de abrir una salida directa. Una vez abierto el backend multicanal, no se puede cambiar su cantidad total de canales; una reconfiguración que la aumente o reduzca se rechaza para evitar interpretar PCM con un formato incorrecto.

## Pruebas y límites

`internal/audio/mixer/router_test.go` verifica routing simultáneo estéreo (`[100, 200, 300, 400]`), mono, canales no asignados en silencio, volumen por zona, pausa/reanudación, EOF aislado, lecturas parciales, rutas inválidas/superpuestas, controles concurrentes y cierre limpio. Las pruebas de control y Manager verifican la propagación y validación de `output.channels`.

- `go test -count=1 ./...`: aprobado.
- `go vet ./...`: aprobado.
- `go test -race` no pudo compilarse en el entorno Windows porque el GCC disponible es Cygwin; Go requiere MinGW para ese detector.
- Las pruebas PCM usan un backend de captura; no requieren ASIO, Dante ni un dispositivo físico.

Oto continúa como backend predeterminado. Su capacidad real para abrir más de dos canales depende del dispositivo y del backend del sistema operativo y no se verificó físicamente. El número de canales del backend queda fijo una vez abierto. Selección dinámica de dispositivo, renegociación de canales, ASIO, Dante y Q-SYS pertenecen a objetivos posteriores.
