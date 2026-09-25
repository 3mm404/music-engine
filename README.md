# Music Engine: laboratorio de audio en Go para Windows

## Alcance actual

Etapa 1, primer paso: obtener IAudioRenderClient desde un IAudioClient inicializado en modo compartido. Es un programa ejecutable de diagnóstico; todavía no reproduce archivos ni genera tonos. Esperaremos tu resultado antes de implementar GetBuffer y ReleaseBuffer.

No se encontró la implementación Go original en la carpeta de trabajo ni en Desktop/dev. Esta base es nueva y utiliza el endpoint predeterminado de reproducción, rol eConsole. No reemplaza código existente ni implementa selección interactiva de endpoints.

## Ejecutar en PowerShell

Requisito: Windows y Go 1.26 o posterior. Las dependencias de Go necesitan acceso a Internet la primera vez si no están en caché.

```powershell
cd C:\Users\perez\Desktop\dev\music-engine
go run ./cmd/engine
```

Para compilar y ejecutar el archivo EXE:

```powershell
go build -o bin/music-engine.exe ./cmd/engine
.\bin\music-engine.exe
```

Ejecuta desde una terminal para leer el resultado antes de que termine el proceso.

## Archivos

- cmd/engine/main.go: programa comentado, con todo el flujo visible.
- go.mod y go.sum: versiones y comprobación de dependencias.
- ETAPA-1.md: conceptos y explicación del cambio nuevo.
- bin/music-engine.exe: ejecutable generado localmente; excluido por .gitignore.

No creamos internal/audio, decoder ni wasapi todavía.

## Validación realizada el 25 de septiembre de 2026

Se ejecutaron gofmt, go mod tidy, go build y go vet. Compilación y análisis correctos. La ejecución real en esta máquina terminó con código 0 y mostró:

```text
Sample rate: 48000 Hz
Channels: 2
Bits por contenedor: 32
Bytes por frame: 8
Buffer: 1126 frames (23.46 ms)
IAudioRenderClient obtenido correctamente
Etapa 1 completada: acceso al servicio. Todavia no se envia PCM ni se reproduce sonido.
```

También imprime el identificador del endpoint. El formato y el tamaño de buffer pueden cambiar con el dispositivo y su configuración. Esta prueba confirma acceso al servicio; no valida todavía escritura PCM ni reproducción.

Si falla, copia el mensaje completo. Si no encuentra un endpoint, revisa que Windows tenga una salida de sonido habilitada y predeterminada.

## Dependencias

- github.com/go-ole/go-ole v1.3.0: inicialización y liberación de COM.
- github.com/moutend/go-wca v0.3.0: bindings explícitos de Windows Core Audio; no es un reproductor de alto nivel.

No se ha incorporado ningún decoder ni servicio HTTP.
