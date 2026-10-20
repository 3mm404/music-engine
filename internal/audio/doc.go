// Package audio define Output y Stream, el contrato entre reproducción y hardware.
// backend selecciona una implementación; oto y asio abren las salidas;
// mixer mezcla y enruta canales. Esta capa no consume APIs de negocio ni playlists.
package audio
