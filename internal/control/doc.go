// Package control coordina configuración, órdenes y estados entre backend,
// Desktop y audio. No implementa drivers ni widgets.
//
// Entrada: RunProfileDesktop (runtime.go). Configuración: runtime_configuration.go.
// Sincronización remota: runtime_sync.go. Ejecución: runtime_commands.go y playback.go.
// Protocolo local: desktop_protocol.go; catálogo: desktop_operations.go.
// Transporte remoto: client.go, engine_api_routes.go y websocket.go.
// Persistencia de órdenes remotas: journal.go. Volumen local: desktop_volume.go.
//
// Los ciclos remotos y comandos locales se despachan serialmente en el bucle
// principal. r.mu protege el estado observado por workers. Las cargas son
// asíncronas: aceptar una orden no significa que el audio ya se reproduzca.
package control
