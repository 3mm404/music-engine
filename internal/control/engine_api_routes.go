package control

// HTTP endpoints consumed by the engine, not administrative/user endpoints.
// Authentication, session headers and envelopes remain centralized in Client.request.
const (
	engineAPIPrefix  = "/api/v1/engine"
	apiSessions      = "/sessions"
	apiConfiguration = "/config"
	apiCommands      = "/commands"
	apiHeartbeat     = "/heartbeat"
	apiBroadcastAuth = "/broadcasting/auth"
)
