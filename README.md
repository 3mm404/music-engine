# Music Engine — beta Windows

Motor Go para reproducción por zonas, conectado al backend Laravel y a Desktop Flutter. La entrada de producción es `cmd/agent`; la versión actual del código es 0.8.4.

## Leer y modificar el proyecto

1. [Arquitectura y responsabilidades de capas](docs/ARQUITECTURA_Y_CAPAS.md).
2. [Guía de operación beta](BETA.md).
3. [Índice separado de pruebas](tests/README.md).
4. [Demo local de desarrollo](tools/local-demo/README.md).

## Estructura vigente

```text
cmd/agent/          entrada beta: CLI, configuración local y pipe Desktop
internal/control/   aplicación: sincronización, validación y comandos
internal/engine/    administración de zonas y sus reproductores
internal/player/    reproducción de una zona
internal/decoder/   descarga/verificación y decodificación
internal/audio/     contratos, mezcla, routing y backends Oto/ASIO
internal/localconfig/ configuración local, secretos y bloqueo
bin/                ejecutables generados, ignorados por Git
tools/local-demo/   demo independiente de producción y su consola
tests/              índice de pruebas; los archivos Go conservan su paquete
docs/historico/     objetivos y documentación anteriores
```

Los archivos `*_test.go` se conservan junto a sus paquetes para poder acceder a funciones privadas, como requiere su diseño actual. Se ocultan solo en el explorador de VS Code mediante `.vscode/settings.json`; no se excluyen de Git ni de Go. Ver tests/README.md para mostrarlos de nuevo.

## Compilar la beta

```powershell
go build -trimpath -o bin/engine.exe ./cmd/agent
```

Después, `./start-local.ps1` ejecuta `bin/engine.exe run` con la configuración local del motor. Configurar primero según BETA.md. Los secretos, diario y logs de Windows viven en `%ProgramData%/UtrackSound`, no en una carpeta de música ni en el repositorio.

Para Desktop se empaqueta una copia compatible del motor junto a la aplicación Flutter. El flujo de ampliación de APIs está documentado en [Agregar APIs y funciones de Desktop](../Utrack_Desktop/docs/AGREGAR_APIS_Y_FUNCIONES.md).

El historial anterior se conserva en [README anterior](docs/historico/README-ANTERIOR.md); sus versiones, rutas y comandos no deben tomarse como instrucciones vigentes.
