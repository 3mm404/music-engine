package control

// Register a local operation here only after its handler is implemented.
// Read capabilities are deliberately not executable commands.
type desktopHandlerKind int

const (
	playbackHandler desktopHandlerKind = iota
	volumeHandler
	seekHandler
	modesHandler
	playlistHandler
	serverHandler
)

type desktopOperation struct {
	name    string
	handler desktopHandlerKind
}

var desktopOperations = []desktopOperation{
	{"play", playbackHandler},
	{"pause", playbackHandler},
	{"resume", playbackHandler},
	{"stop", playbackHandler},
	{"next", playbackHandler},
	{"previous", playbackHandler},
	{"volume", volumeHandler},
	{"seek", seekHandler},
	{"playback_modes", modesHandler},
	{"play_playlist", playlistHandler},
	{"song_preference", serverHandler},
}

func findDesktopOperation(name string) (desktopOperation, bool) {
	for _, operation := range desktopOperations {
		if operation.name == name {
			return operation, true
		}
	}
	return desktopOperation{}, false
}

func DesktopCapabilities() []string {
	capabilities := make([]string, 0, len(desktopOperations)+1)
	for _, operation := range desktopOperations {
		capabilities = append(capabilities, operation.name)
	}
	return append(capabilities, "playlist_read")
}
