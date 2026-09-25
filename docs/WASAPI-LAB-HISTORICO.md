# Etapa 1: obtener IAudioRenderClient

## Problema que resolvemos

Ya podemos configurar un flujo de audio. Ahora necesitamos obtener la interfaz que nos permitirá entregar PCM a ese flujo. Este paso obtiene la interfaz, sin solicitar memoria ni escribir samples todavía.

```text
Archivo -> Decoder -> PCM -> WASAPI -> Endpoint -> Bocinas
                              |
                              +-- IAudioClient: configura y controla el flujo
                              |
                              +-- GetService(IID_IAudioRenderClient, ...)
                                          |
                                          v
                                  IAudioRenderClient
                                  acceso al buffer de reproducción
```

IAudioClient e IAudioRenderClient son interfaces COM con responsabilidades diferentes sobre el mismo flujo. Obtener la segunda no abre otro dispositivo.

## Antes del código: contratos

GetService es un método de IAudioClient. Nuestro programa lo llama después de Initialize. Recibe el identificador de interfaz solicitado y la dirección de una variable donde guardar el puntero resultante. En go-wca devuelve un error: nil significa éxito. La interfaz se entrega por el segundo argumento, no como valor de retorno.

IAudioRenderClient es la interfaz que permite acceder al buffer de reproducción. No es el buffer ni contiene un archivo musical. Obtenerla no empieza la reproducción. Sus operaciones de escritura se estudiarán en el siguiente paso.

Release es un método de COM que nuestra aplicación llama al terminar de usar una referencia. No recibe argumentos y devuelve el contador de referencias restante, que ignoramos. No es ReleaseBuffer. Los servicios deben liberarse antes de IAudioClient y en el mismo hilo que lo libera.

## Cambio nuevo explicado línea por línea

```go
var renderClient *wca.IAudioRenderClient
if err := audioClient.GetService(wca.IID_IAudioRenderClient, &renderClient); err != nil {
    return fmt.Errorf("obtener IAudioRenderClient: %w", err)
}
defer renderClient.Release()
fmt.Println("IAudioRenderClient obtenido correctamente")
```

1. var declara la variable. El asterisco indica un puntero a la interfaz; comienza en nil. No reserva PCM.
2. audioClient.GetService pide la interfaz. IID_IAudioRenderClient identifica el tipo de servicio, no las bocinas. &renderClient entrega la dirección de nuestra variable para que la llamada la complete. err != nil comprueba el fallo.
3. return abandona run si falla. fmt.Errorf añade contexto; %w conserva el error original.
4. La llave cierra el bloque de error.
5. defer pospone Release hasta salir de run. Los defer se ejecutan en orden inverso: renderClient se libera antes de audioClient.
6. Println confirma que obtuvimos el servicio.

## Base ejecutable que rodea este cambio

El archivo main.go incluye comentarios en cada bloque:

1. main llama a run. Si run falla, muestra el error y termina con código 1. Al hacerlo después del retorno de run, sus defer ya se ejecutaron.
2. runtime.LockOSThread evita que Go cambie esta goroutine de hilo de Windows mientras utiliza COM. UnlockOSThread se ejecuta al final.
3. CoInitializeEx inicializa COM en ese hilo. S_FALSE también es éxito y requiere CoUninitialize.
4. CoCreateInstance obtiene IMMDeviceEnumerator; Release devuelve su referencia al terminar.
5. GetDefaultAudioEndpoint pide la salida predeterminada para el rol eConsole. GetId permite ver qué endpoint se usó.
6. Activate obtiene IAudioClient del endpoint.
7. GetMixFormat devuelve el formato del mezclador. Windows asigna esa memoria; CoTaskMemFree la libera. unsafe.Pointer solo permite pasar esa dirección a la función de liberación.
8. Initialize configura modo compartido con el formato devuelto. La duración solicitada de 200000 unidades de 100 ns equivale a 20 ms; la periodicidad es 0, como corresponde al modo compartido. El tamaño efectivo lo decide Windows.
9. GetBufferSize escribe en bufferFrames la capacidad real, medida en frames, no bytes ni samples individuales.
10. GetService obtiene IAudioRenderClient. El programa muestra el resultado y libera sus recursos.

## Relación con las magnitudes físicas

Para el formato observado de 48000 Hz y 2 canales:

```text
Frame 0 -> sample izquierdo + sample derecho
Frame 1 -> sample izquierdo + sample derecho
Frame 2 -> sample izquierdo + sample derecho

48000 frames = 1 segundo
1 frame = 2 samples
1126 frames / 48000 frames por segundo = 0.023458... segundos
```

El formato observado tiene contenedores de 32 bits, es decir, 4 bytes por sample. NBlockAlign indica 8 bytes por frame; 1126 frames representan 9008 bytes. Es la capacidad, no una afirmación de que el buffer ya contenga audio entregado por nosotros.

Los bits por contenedor no bastan para decidir cómo escribir samples. Un formato de 32 bits podría ser entero o float; WAVE_FORMAT_EXTENSIBLE también requiere examinar el subformato y los bits válidos. Lo estudiaremos antes de escribir PCM.

## Resultado esperado y punto de pausa

Ejecuta los comandos del README. Debes obtener el mensaje IAudioRenderClient obtenido correctamente. No se escucha sonido porque no hemos entregado PCM ni iniciado el flujo.

Comparte la salida de tu ejecución antes de continuar. El siguiente paso se limitará al acceso al buffer, con explicación de cada método antes del código. El tono pertenece a la etapa 2.

## Referencias

- Microsoft, GetService: https://learn.microsoft.com/es-es/windows/win32/api/audioclient/nf-audioclient-iaudioclient-getservice
- Firma Go: https://pkg.go.dev/github.com/moutend/go-wca/pkg/wca#IAudioClient.GetService
- Código fuente del binding: https://github.com/moutend/go-wca
