# Bicycle

A modular Go daemon service with a flexible plugin architecture for multi-channel chat-bot interactions.

## Overview

Bicycle implements a plugin-based architecture that enables simultaneous interaction through multiple channels (Terminal UI, Telegram, WebSocket, REST API) while providing extensible task execution capabilities through executor plugins.

### Key Features

- **Plugin Architecture**: Interface-based plugins with compile-time safety
- **Multi-Channel Communication**: TUI, Telegram, WebSocket, REST API
- **Mode-Aware Commands**: Commands adapt to daemon vs interactive modes
- **Extensible Task Execution**: Pluggable task executors (LLM agent, etc.)
- **Pluggable State Management**: In-memory and file-based backends with typed codec support
- **Message Broker**: Topic-based pub/sub for inter-plugin communication
- **Graceful Lifecycle**: Proper startup, shutdown, and requirement checking
- **Swarm Orchestration**: Multi-agent task orchestration with verification and retry support

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
   - `state_file`: File-based state persistence

4. **Orchestration Plugins**: Coordinate multi-agent workflows
   - `swarm`: Multi-agent task orchestration with verification

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
├── state/                     # State management
│   ├── backend.go            # Backend interface
│   ├── memory.go             # In-memory backend
│   ├── file.go               # File-based backend
│   ├── manager.go            # State manager
│   └── codec.go              # Encoding/decoding
├── swarm/                     # Swarm orchestration
│   ├── types.go              # Core types (Task, Agent, Project)
│   ├── orchestrator.go       # Main orchestrator
│   ├── queue.go              # Task queue management
│   ├── registry.go           # Agent registry
│   ├── worker.go             # Worker agent implementation
│   ├── spawner.go            # Agent spawning
│   ├── verifier.go           # Task verification
│   ├── llm.go                # LLM client interface
│   └── anthropic.go          # Anthropic API integration
└── plugins/                   # Plugin implementations
    ├── state/memory/         # In-memory state plugin
    ├── tui/                  # Terminal UI
    ├── telegram/             # Telegram bot
    ├── websocket/            # WebSocket server
    ├── rest/                 # REST API
    ├── executor/llm/         # LLM executor
    └── swarm/                # Swarm orchestration plugin
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

### Swarm Commands

When the swarm plugin is enabled:

- `/swarm` - Show swarm management help
- `/swarm-create <project-id> [worker-model] [verifier-model]` - Create a new swarm project
- `/swarm-start <project-id> [worker-count]` - Start a project with workers
- `/swarm-stop <project-id>` - Stop a project
- `/swarm-task <project-id> <type> <title> <description>` - Add a task to a project
- `/swarm-status [project-id]` - Show project status or list all projects

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

## Swarm Orchestration

The swarm package provides a multi-agent orchestration system for coordinating LLM-powered workers on complex tasks.

### Concepts

- **Project**: A container for tasks and agents working toward a common goal
- **Task**: A unit of work with acceptance criteria, priority, and retry limits
- **Agent**: A worker (typically LLM-powered) that executes tasks
- **Orchestrator**: Manages task assignment, verification, and agent health
- **Verifier**: Reviews completed tasks and approves or requests revisions

### Workflow

1. Create a project with `/swarm-create`
2. Start the project with `/swarm-start` to spawn worker agents
3. Add tasks with `/swarm-task`
4. The orchestrator automatically:
   - Assigns pending tasks to idle workers
   - Monitors agent health via heartbeats
   - Verifies completed tasks using an LLM verifier
   - Requests revisions if tasks don't meet acceptance criteria
   - Retries failed tasks up to the configured limit

### Agent Roles

- **Worker**: Executes assigned tasks
- **Orchestrator**: Coordinates task assignment and verification
- **Reviewer**: Reviews task output (verification role)
- **Architect**: Plans and breaks down complex tasks (future)

### Task Lifecycle

```
pending → assigned → in_progress → review → completed
                                       ↓
                                   revision → (retry) → assigned
                                       ↓
                                   failed (max retries exceeded)
```

### Configuration

```yaml
plugins:
  swarm:
    enabled: true
    settings:
      verification_interval: 10s
      assignment_interval: 5s
      health_check_interval: 30s
```

### Verification Policy

Projects can configure verification behavior:

```go
VerificationPolicy{
    BatchSize:           1,     // Verify every N completions
    ConfidenceThreshold: 0.8,   // Auto-verify if confidence > threshold
    MaxRetries:          3,     // Retry limit before marking failed
    SkipLabels:          []string{"trivial"},     // Skip verification
    AlwaysVerifyLabels:  []string{"critical"},    // Always verify
}
```

## State Management

The `state` package provides pluggable backends for persistent storage.

### Backends

- **Memory**: In-memory storage (default, non-persistent)
- **File**: JSON-based file storage

### Usage

```go
// Create a file backend
backend, err := state.NewFileBackend("/path/to/state")

// Create a manager with typed operations
manager := state.NewManager(backend)

// Store and retrieve typed values
err = manager.Set(ctx, "key", myStruct)
err = manager.Get(ctx, "key", &result)
```

## Testing

Run the test suite:

```bash
go test ./...
```

Run with race detector:

```bash
go test -race ./...
```

## GitHub Actions

Tests run automatically on pull requests and pushes to main. See `.github/workflows/test.yml`.

## Project Status

This is version 0.2.0. Recent additions include:

- **Swarm Orchestration**: Complete multi-agent task orchestration system with:
  - Task queue with priority scheduling
  - Agent registry with health monitoring
  - LLM-powered verification
  - Automatic retry with feedback
  - Anthropic API integration
- **State Management**: Pluggable state backends (memory and file-based)
- **Unit Tests**: Comprehensive test coverage for core packages
- **CI/CD**: GitHub Actions workflow for automated testing

Future versions will include:

- OpenAI API integration
- Database state backends (SQLite, PostgreSQL)
- Plugin hot-reloading
- Metrics and monitoring
- Web dashboard

## License

(To be determined)

## Contributing

(To be determined)
