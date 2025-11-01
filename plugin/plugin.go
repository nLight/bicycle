package plugin

import (
	"context"
)

// Mode represents the execution mode of the daemon
type Mode string

const (
	// ModeDaemon represents background daemon mode
	ModeDaemon Mode = "daemon"
	// ModeInteractive represents interactive mode with user input
	ModeInteractive Mode = "interactive"
)

// Plugin represents a loadable plugin that provides extensions
type Plugin interface {
	// Name returns the unique plugin identifier
	Name() string

	// CheckRequirements validates if the plugin can run in the current environment
	CheckRequirements(ctx context.Context) error

	// Extensions returns all extensions this plugin provides
	Extensions() []Extension

	// Start initializes and starts the plugin
	// The broker parameter allows plugins to publish and subscribe to messages
	Start(ctx context.Context, broker MessageBroker) error

	// Stop gracefully shuts down the plugin
	Stop(ctx context.Context) error
}

// MessageBroker defines the interface for pub/sub communication
// This is defined here to avoid circular dependencies
type MessageBroker interface {
	// Subscribe creates a subscription for the given topics
	// Returns a channel that will receive matching messages
	Subscribe(id string, bufSize int, topics ...string) <-chan Message

	// Publish broadcasts a message to all interested subscribers
	Publish(ctx context.Context, msg Message) error

	// Unsubscribe removes a subscription and closes its channel
	Unsubscribe(id string)
}

// Message represents a message in the pub/sub system
type Message struct {
	// Topic is the message category/channel
	Topic string

	// Payload contains the message data
	Payload interface{}

	// Source identifies the originating plugin
	Source string

	// Metadata contains additional message information
	Metadata map[string]interface{}
}
