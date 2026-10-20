# Objetivo 06 — Control completo desde Laravel

Alcance vigente del usuario: controles por zona y conexión a la instalación habitual. Reemplaza la previsión histórica de avance automático/prefetch para el objetivo 06.

## Implementación

- Tarjetas por zona con reproducción, canción, volumen solicitado/reportado, playlist, conexión, confirmación de configuración y última orden. Actualización cada 5 segundos. Un reporte desactualizado se identifica expresamente.
- Reproducir permite elegir una canción de la playlist o reiniciar la selección actual. Controles directos para pausar, reanudar, detener, siguiente y anterior.
- Volumen / Playlist guarda los ajustes de esa zona. Volumen conserva la reproducción. Cambiar playlist detiene y selecciona su primera canción; requiere una orden para reproducir.
- Órdenes muestra las 20 últimas del equipo actual y zona: pendientes, confirmadas, fallidas o con plazo vencido sin confirmación. Los fallos incluyen código y mensaje. Guardar/enviar nunca anuncia ejecución exitosa.
- Permisos de actualización de zona y equipo, con pertenencia al negocio; selección de playlist y canción validada en el servidor. Los errores de preparación se muestran en notificaciones y formularios.

## Contrato y compatibilidad

La migración `2026_09_27_005950_add_song_id_to_engine_commands_table` añade un string nullable a las órdenes. `GET /api/v1/engine/commands` incluye `song_id` (string o null). Solo `play` admite una selección explícita y la canción debe pertenecer a la playlist de la revisión enviada. Laravel exige agente >= 0.6.0 para impedir que un agente antiguo ignore el campo y reproduzca otra canción.

El agente 0.6.0 selecciona el índice antes de cargar. Next/Previous parten de ese índice. IDs ajenos o parámetros incompatibles fallan sin sustituir la carga pendiente válida. Se reutilizan revisión, caducidad, registro durable, confirmaciones y descarga HTTPS del objetivo 05.

## Instalación habitual

- Laravel: `https://utrack-fly.test/admin/zones`, enlazado con Herd y certificado confiable. APP_URL actualizado por Herd; medios usan el mismo origen HTTPS.
- Migración aditiva aplicada. PERREO importada a privado mediante el comando existente, con verificación de hash; MP3 44100 Hz válido.
- Engine local Panda Club (id 1), agente 0.6.0, salida predeterminada de Windows, estéreo. Zona Panda Club (1) y nueva Zona de Albercas (2, añadida por el usuario durante la tarea).
- `music-engine/start-local.ps1` usa `engine-state/connection.json`, ignorado por Git, y el binario `bin/agent.exe`. Credencial nunca incluida en documentación. No es un servicio Windows; al reiniciar el equipo hay que iniciar el script. No arrancar una segunda instancia con la misma credencial.
- Se detectaron bloqueos intermitentes SQLite en la instalación habitual. Caché local trasladada a archivos; journal WAL, transacciones IMMEDIATE y busy timeout de 5000 ms. Las opciones de SQLite se leen desde configuración/entorno. No se modificaron datos para resolver los bloqueos.

## Verificación y límites

Suite PHP completa inicial: 71 pruebas aprobadas, 1 integración optativa omitida, 375 aserciones. Suite focalizada de controles/audio: 39 aprobadas. Go test ./... y go vet ./... aprobados. Integración Laravel + HTTPS + binario + Oto con canción específica: aprobada en estéreo y mono (42.21 segundos). Pint aplicado.

La revisión final, evidencia de operación habitual y estado de procesos se registran en [CHECKPOINT](../../CHECKPOINT.md). No se afirma audición física: se verifican reproducción Oto, resultados y heartbeat reales.

Se mantienen los límites del 05: MP3 44100 Hz, descarga completa, salida física compartida, sin remuestreo, sin autoplay/avance automático/prefetch ni servicio Windows. Finalizar una canción deja la zona detenida aunque la orden histórica de reproducción figure confirmada.
