# Bicycle

A modular Go daemon service with a flexible plugin architecture for multi-channel chat-bot interactions.

## Overview

Bicycle implements a plugin-based architecture that enables simultaneous interaction through multiple channels (Terminal UI, Telegram, WebSocket, REST API) while providing extensible task execution capabilities through executor plugins.

### Key Features

- **Plugin Architecture**: Interface-based plugins with compile-time safety
- **Multi-Channel Communication**: TUI, Telegram, WebSocket, REST API
- **Mode-Aware Commands**: Commands adapt to daemon vs interactive modes
- **Extensible Task Execution**: Pluggable task executors (LLM agent, etc.)
- **Pluggable State Management**: In-memory, file-based, or database backends
- **Message Broker**: Topic-based pub/sub for inter-plugin communication
- **Graceful Lifecycle**: Proper startup, shutdown, and requirement checking

## Architecture

### Plugin Categories

1. **Interaction Plugins**: Handle user communication
   - `tui`: Terminal User Interface (bubbletea)
   - `telegram`: Telegram bot integration
   - `websocket`: WebSocket server
   - `rest`: REST API server

2. **Executor Plugins**: Execute tasks
   - `llm`: LLM-based agent (OpenAI, Anthropic, etc.)

3. **State Plugins**: Manage persistent state
   - `state_memory`: In-memory state storage

### Core Components

```
bicycle/
├── main.go                    # Application entry point
├── daemon/                    # Daemon core
│   ├── daemon.go             # Lifecycle management
│   └── broker.go             # Message broker (pub/sub)
├── plugin/                    # Plugin system
│   ├── plugin.go             # Plugin interfaces
│   ├── extension.go          # Extension types
│   ├── registry.go           # Plugin registry
│   └── requirements.go       # Requirement checking
├── cmd/                       # Command system
│   ├── registry.go           # Command registry
│   ├── router.go             # Command routing
│   └── builtin.go            # Built-in commands
├── internal/config/           # Configuration management
└── plugins/                   # Plugin implementations
    ├── state/memory/         # In-memory state
    ├── tui/                  # Terminal UI
    ├── telegram/             # Telegram bot
    ├── websocket/            # WebSocket server
    ├── rest/                 # REST API
    └── executor/llm/         # LLM executor
```

## Getting Started

### Prerequisites

- Go 1.24 or higher
- (Optional) Telegram bot token for Telegram plugin
- (Optional) OpenAI/Anthropic API key for LLM executor

### Building

```bash
go build -o bicycle
```

### Running

#### Interactive Mode (with TUI)

```bash
# Create config file
cp config.example.yaml config.yaml

# Edit config.yaml and set mode to 'interactive'
# Enable TUI plugin

# Run
./bicycle --config config.yaml
```

#### Daemon Mode

```bash
# Edit config.yaml and set mode to 'daemon'
# Enable desired plugins (telegram, websocket, rest)

# Run
./bicycle --config config.yaml
```

### Command-Line Options

```bash
./bicycle --help

Options:
  -config string
        Path to configuration file (default "config.yaml")
  -mode string
        Execution mode (daemon or interactive)
  -version
        Show version information
  -list-plugins
        List registered plugins
```

## Configuration

Configuration is managed via YAML files. See `config.example.yaml` for a complete example.

### Basic Structure

```yaml
# Execution mode: daemon or interactive
mode: daemon

# Daemon configuration
daemon:
  log_level: info
  broker_buffer_size: 100
  publish_timeout: 5

# Plugin configuration
plugins:
  # Plugin name
  plugin_name:
    enabled: true
    settings:
      key: value
```

### Plugin Configuration Examples

#### Telegram Plugin

```yaml
plugins:
  telegram:
    enabled: true
    settings:
      token: "your-bot-token-here"
```

Or set via environment variable:
```bash
export TELEGRAM_TOKEN="your-bot-token-here"
./bicycle
```

#### WebSocket Plugin

```yaml
plugins:
  websocket:
    enabled: true
    settings:
      port: 8080
      host: "0.0.0.0"
```

#### REST API Plugin

```yaml
plugins:
  rest:
    enabled: true
    settings:
      port: 8081
      host: "0.0.0.0"
      auth_token: "optional-secret-token"
```

#### LLM Executor Plugin

```yaml
plugins:
  llm:
    enabled: true
    settings:
      provider: openai  # or anthropic
      model: gpt-4
      api_key: "your-api-key"
```

Or use environment variables:
```bash
export OPENAI_API_KEY="your-api-key"
# or
export ANTHROPIC_API_KEY="your-api-key"
```

## Built-in Commands

All plugins have access to these built-in commands:

- `/help [command]` - Show available commands or help for a specific command
- `/status` - Show daemon status and active plugins
- `/reset` - Stop current task and reset to idle state
- `/plugins` - List all registered plugins
- `/ask <question>` - Ask the LLM executor a question (if LLM plugin is enabled)

## Using the Interaction Plugins

### Terminal UI (TUI)

In interactive mode, the TUI provides a chat-like interface:

1. Type messages or commands
2. Commands start with `/`
3. Press Ctrl+C or Esc to quit

### Telegram Bot

1. Create a bot via @BotFather on Telegram
2. Get your bot token
3. Configure the token in `config.yaml` or via `TELEGRAM_TOKEN` environment variable
4. Enable the telegram plugin
5. Start the daemon
6. Send messages to your bot on Telegram

### WebSocket

Connect to `ws://localhost:8080/ws` and send JSON messages:

```json
{
  "type": "command",
  "payload": "/status"
}
```

Message types:
- `command`: Execute a command
- `chat`: Send a chat message

Receive messages:
```json
{
  "type": "notification",
  "payload": "Task completed successfully"
}
```

### REST API

#### Execute Command
```bash
curl -X POST http://localhost:8081/api/command \
  -H "Content-Type: application/json" \
  -d '{"command": "/status"}'
```

With authentication:
```bash
curl -X POST http://localhost:8081/api/command \
  -H "Authorization: Bearer your-token" \
  -H "Content-Type: application/json" \
  -d '{"command": "/status"}'
```

#### Get Status
```bash
curl http://localhost:8081/api/status
```

#### Health Check
```bash
curl http://localhost:8081/api/health
```

## Developing Plugins

### Plugin Structure

Every plugin must implement the `Plugin` interface:

```go
type Plugin interface {
    Name() string
    CheckRequirements(ctx context.Context) error
    Extensions() []Extension
    Start(ctx context.Context, broker MessageBroker) error
    Stop(ctx context.Context) error
}
```

### Example Plugin

```go
package myplugin

import (
    "context"
    "bicycle/plugin"
)

func init() {
    plugin.Register(NewMyPlugin())
}

type MyPlugin struct {
    broker plugin.MessageBroker
}

func NewMyPlugin() *MyPlugin {
    return &MyPlugin{}
}

func (p *MyPlugin) Name() string {
    return "myplugin"
}

func (p *MyPlugin) CheckRequirements(ctx context.Context) error {
    checker := plugin.NewRequirementChecker("myplugin")
    // Add requirements
    return checker.Check(ctx)
}

func (p *MyPlugin) Extensions() []plugin.Extension {
    return []plugin.Extension{}
}

func (p *MyPlugin) Start(ctx context.Context, broker plugin.MessageBroker) error {
    p.broker = broker
    // Subscribe to messages
    msgCh := broker.Subscribe("myplugin", 100, "notification")
    // Start handlers...
    return nil
}

func (p *MyPlugin) Stop(ctx context.Context) error {
    // Cleanup
    return nil
}
```

### Registering Commands

```go
import "bicycle/cmd"

func init() {
    cmd.Register(&plugin.Command{
        Name:        "mycommand",
        Description: "My custom command",
        Usage:       "[args]",
        Handler:     handleMyCommand,
        Modes:       []plugin.Mode{plugin.ModeDaemon},
    })
}

func handleMyCommand(ctx context.Context, args []string) (*plugin.CommandResult, error) {
    return &plugin.CommandResult{
        Output: "Command executed!",
    }, nil
}
```

### Using the Message Broker

**Publishing messages:**
```go
broker.Publish(ctx, plugin.Message{
    Topic:   "notification",
    Payload: "Hello, world!",
    Source:  "myplugin",
})
```

**Subscribing to messages:**
```go
msgCh := broker.Subscribe("myplugin", 100, "notification", "chat")
for msg := range msgCh {
    // Handle message
}
```

## Message Broker Topics

Standard topics used by the system:

- `notification`: System notifications (broadcasts to all channels)
- `chat`: Chat messages from users
- `response`: Command responses
- `command_result`: Results from command execution

Plugins can define custom topics for their own use.

## Project Status

This is version 0.1.0 - initial implementation. The LLM executor is currently a stub that simulates task execution. Future versions will include:

- Full LLM API integration (OpenAI, Anthropic, etc.)
- File-based and database state plugins
- Additional interaction plugins
- Plugin hot-reloading
- Metrics and monitoring
- Web dashboard

## License

(To be determined)

## Contributing

(To be determined)
