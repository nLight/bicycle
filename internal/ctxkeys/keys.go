package ctxkeys

// Key is a typed context key to avoid collisions
type Key string

const (
	// Mode is the context key for the execution mode (plugin.Mode)
	Mode Key = "mode"

	// Daemon is the context key for the daemon instance
	Daemon Key = "daemon"

	// Config is the context key for the configuration (*config.Config)
	Config Key = "config"
)
