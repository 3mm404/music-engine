# Utrack Sound Engine

Motor de audio multizona escrito en Go para Windows x64. Ejecuta la reproducción en el equipo que tiene conectada la salida de audio, sincroniza la configuración con un backend Laravel y expone controles y estado a Utrack Desktop, la aplicación Flutter.

**Estado:** beta para Windows. La versión del código se declara en [cmd/agent/main.go](cmd/agent/main.go) y se comprueba con `engine.exe version`. El paquete de Desktop fija su versión compatible en [windows/engine/VERSION](https://github.com/3mm404/Utrack_Desktop/blob/main/windows/engine/VERSION).

## Qué resuelve

- **Reproducción por zona.** Cada zona tiene su propio reproductor, estado y volumen. Con Oto, las zonas se mezclan en la salida estéreo predeterminada de Windows. Con ASIO, se pueden dirigir a canales físicos separados de un dispositivo compatible.
- **Compatibilidad de audio.** El motor decodifica MP3 MPEG-1, MPEG-2 y MPEG-2.5 y remuestrea en memoria para reproducir pistas de distintas frecuencias sin modificar el archivo original.
- **Operación y diagnóstico.** El agente sincroniza cambios de configuración y comandos, reporta estado mediante heartbeats y conserva un diario local para proteger contra la reejecución de órdenes remotas tras una caída.

## Cómo encajan las piezas

| Componente | Responsabilidad |
| --- | --- |
| Backend Laravel | Entrega configuración, catálogo y comandos al equipo por HTTPS y notifica cambios por WebSocket. |
| Motor Go | Aplica las órdenes, administra los reproductores por zona y convierte el audio a PCM para Oto o ASIO. |
| [Utrack Desktop](https://github.com/3mm404/Utrack_Desktop) | Configura y controla el motor local mediante mensajes JSON por stdin y stdout. |

## Probar una pista local

Desde la raíz del repositorio en Windows, con Go 1.26 o posterior y un MP3 propio. La demo abre una consola local para las zonas A y B y se puede ejecutar sin un servidor Laravel.

```powershell
go run ./tools/local-demo -file "C:/ruta/mi-audio.mp3"
```

Consulta la [guía de la demo](tools/local-demo/README.md) para conocer su alcance. Para ver la interfaz Flutter que controla el motor en una instalación, consulta la [demostración y las capturas de Utrack Desktop](https://github.com/3mm404/Utrack_Desktop#demostración-explícita).

## Compilar y conectar el agente

```powershell
go build -trimpath -o bin/engine.exe ./cmd/agent
.\bin\engine.exe version
.\bin\engine.exe --help
```

Para conectarlo se necesita un servidor HTTPS compatible y una credencial individual de equipo. La [guía de operación beta](BETA.md), escrita para la versión 0.8.8, documenta la configuración de Oto y ASIO, la protección de la credencial con DPAPI y la ubicación de los logs en `%ProgramData%\UtrackSound`. Una vez configurado, `./start-local.ps1` inicia el agente compilado.

## Código y pruebas

| Ruta | Contenido |
| --- | --- |
| `cmd/agent` | CLI, arranque y comunicación local con Desktop. |
| `internal/control` | Sincronización, comandos, estados y descargas de listas. |
| `internal/engine` y `internal/player` | Gestión de zonas y reproducción por zona. |
| `internal/decoder` e `internal/audio` | Decodificación, caché cifrada, mezcla y salidas Oto y ASIO. |
| `internal/localconfig` | Configuración, protección de credenciales y bloqueo de instancia. |

```powershell
go test ./...
```

- [Arquitectura y responsabilidades](docs/ARQUITECTURA_Y_CAPAS.md)
- [Índice de pruebas y ubicación de los casos](tests/README.md)
- [Mediciones de rendimiento y escenarios de fallo](docs/METRICAS.md)
- [Historial del proyecto](docs/historico/README-ANTERIOR.md)
