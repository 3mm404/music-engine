# UtrackSound beta 0.7.0 — Windows x64

Distribuye engine.exe y esta guia. No requiere Go ni PowerShell como lanzador: PowerShell o Terminal solo sirven para introducir comandos. Configuracion, credencial DPAPI, diario y logs siguen en `%ProgramData%\UtrackSound`.

## Elegir salida

### Oto: salida predeterminada de Windows

```powershell
.\engine.exe configure --backend oto --server "https://tu-servidor" --mode stereo
.\engine.exe run
```

No requiere ASIO. Todas las zonas se mezclan en la misma salida estereo del sistema, manteniendo controles de transporte y volumen por zona. No permite seleccionar dispositivos independientes ni asignar canales fisicos multizona. El agente procesa MP3 44100 Hz; la frecuencia del contexto Oto deriva del audio, sin controles ASIO. La ayuda `configure --backend oto --help` muestra solo sus parametros; se rechazan flags ASIO.

La configuracion enviada por Laravel debe usar `output=null` o salida `default` con `[1,2]` para estereo / `[1]` para mono. Estos valores representan la mezcla compartida, no salidas aisladas. Se rechazan rutas 3–4, 5–6, dispositivos distintos y asignaciones no compatibles. Si el panel/servidor exige canales exclusivos entre zonas, debe adaptarse alli para poder guardar multiples zonas compartidas; este cambio del engine no modifica Laravel ni ignora sus rutas.

### ASIO: driver y canales fisicos

```powershell
.\engine.exe drivers
.\engine.exe configure --backend asio --server "https://tu-servidor" --driver "Dante Virtual Soundcard (x64)" --sample-rate 48000 --buffer 256 --mode stereo
.\engine.exe run
```

Driver ASIO x64 instalado, frecuencia y buffer compatibles con el dispositivo. `--channels 0` es automatico; un valor explicito depende de los canales disponibles. Las rutas deben ser distintas entre zonas. La apertura real comprueba capacidad, frecuencia y buffer; algunos errores solo aparecen al reproducir. No se cambia a Oto ante un fallo. Dante requiere su instalacion/licencia y red correspondientes.

## Cambiar de backend

1. Cerrar el motor con Ctrl+C y esperar a que termine el proceso. Detener una cancion desde el panel no basta.
2. Ejecutar `engine.exe backend oto` o `engine.exe backend asio`.
3. Ejecutar `engine.exe run` en un proceso nuevo.

El comando conserva servidor, credencial y parametros ASIO anteriores. Si nunca se configuro ASIO, use `configure --backend asio ...` primero. Un bloqueo impide cambiar/configurar mientras corre el motor. Se cierran reproductores y backend antes de salir. Oto no ofrece cierre de su contexto global: la liberacion completa del dispositivo requiere terminar el proceso, por eso no hay cambio en caliente.

Los archivos antiguos sin `backend` se interpretan como ASIO. `configure` sin selector tambien conserva el comportamiento ASIO anterior. Para instalaciones nuevas indique siempre `--backend`. Configure solicita la credencial oculta y sustituye los parametros por los introducidos; para alternar sin perder parametros use `backend`.

## Conectar cada PC

Cada participante necesita un servidor HTTPS accesible con certificado confiable, acceso a sus URLs de audio/WebSocket y una credencial individual de equipo asociada a su negocio. Una cuenta del panel no sustituye al token. No distribuya credenciales, `connection.json` ni la carpeta de datos. El dominio de desarrollo local no funciona automaticamente desde otras PCs.

La credencial esta cifrada con DPAPI del usuario que configura. Ejecute configure y run con el mismo usuario de Windows; en otra PC se debe configurar de nuevo. Permisos de la carpeta restringidos al usuario, administradores y SYSTEM. No instala servicio ni inicio automatico.

Persistencia: `config.json`, `journal/` y `logs/`. Logs por fecha de arranque, sin purga automatica. Se conserva el diario al cambiar de backend. Para migrar desde engine-state: con el motor cerrado, copiar sus `*.jsonl` a journal antes del primer arranque, sin sobreescribir diarios existentes. No copiar connection.json. ENGINE_CA_FILE sigue disponible para una CA privada.

## Verificacion

Automatizada, sin hardware: compatibilidad de configuraciones antiguas, seleccion y parametros por backend, persistencia DPAPI, bloqueo durante ejecucion, cambio Oto/ASIO conservando credencial, validacion de rutas, rechazo de fallback, cierre de salida, conexion HTTPS de prueba y Play/Pause/Resume/Stop con PCM decodificado y dispositivo simulado. Pruebas existentes de API, diario, mixer y ASIO tambien incluidas.

Pendiente con hardware: iniciar cada backend en un proceso independiente, conectar al servidor real, reproducir y escuchar, pausar/reanudar/parar, probar dos zonas, cerrar el proceso, cambiar backend y volver a reproducir. Oto debe mezclar las zonas en el dispositivo predeterminado; ASIO debe separar los canales segun el ruteo. Probar driver ausente, frecuencia/buffer incompatibles y confirmar que no hay fallback. La suite normal no certifica audio audible ni la conexion al servidor de produccion.

Compilacion del desarrollador (Go 1.26+):

```powershell
go test ./...
go vet ./...
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
go build -trimpath -o bin/engine.exe ./cmd/agent
```

Las pruebas DPAPI requieren un perfil real de Windows. El binario beta no esta firmado.
