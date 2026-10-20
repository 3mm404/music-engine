// Package localconfig posee configuración local, bloqueo de instancia y secretos.
// En Windows protege credenciales mediante DPAPI. El token no debe entrar en
// snapshots, parámetros de línea de comandos ni preferencias de Flutter.
// No administra zonas/playlists: esas pertenecen a la configuración del servidor.
package localconfig
