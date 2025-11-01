 Here is Claude's plan:
╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌
 Implementation Plan for Bicycle Daemon

 Architecture Overview

 Interface-based plugin architecture with three plugin categories:
 1. Interaction plugins: TUI, Telegram, WebSocket, REST API
 2. Executor plugins: LLM agent (main), extensible for future executors
 3. State plugins: In-memory (default), file-based, database

 All plugins communicate via a central message broker with topic-based fan-out.

 Phase 1: Core Infrastructure (Foundation)

 1. Project setup
   - Initialize Go module bicycle
   - Create directory structure following best practices
   - Setup go.mod with initial dependencies
 2. Core plugin interfaces (plugin/)
   - Define Plugin interface (Name, CheckRequirements, Extensions, Start, Stop)
   - Define Extension interface with types (Command, Executor, StateManager)
   - Implement Registry with init-based registration pattern
   - Create RequirementChecker framework
 3. Message broker (daemon/broker.go)
   - Topic-based pub/sub system
   - Subscription management with buffered channels
   - Fan-out with errgroup for concurrent delivery
   - Graceful handling of slow consumers
 4. Command system (cmd/)
   - Define Command struct with mode awareness
   - Implement command registry with init-based registration
   - Command router with execution context
   - Built-in commands: /status, /reset, /help, /plugins

 Phase 2: Configuration & Daemon Core

 5. Configuration management (internal/config/)
   - YAML config file support (using viper or similar)
   - Config structure for plugins, daemon settings
   - Config validation
 6. Daemon implementation (daemon/daemon.go)
   - Lifecycle management (Start, Stop, Restart)
   - Plugin loading and initialization
   - Requirement checking for all plugins
   - Context management with graceful shutdown
   - State machine: Idle, Working, Stopped

 Phase 3: State Plugin (First Plugin)

 7. State plugin interface (plugin/state.go)
   - Define StateManager interface
   - Methods: Get, Set, Delete, Save, Load
 8. In-memory state plugin (plugins/state/memory/)
   - Simple map-based storage
   - Thread-safe with sync.RWMutex
   - Default plugin (always available)

 Phase 4: Interaction Plugins

 9. TUI plugin (plugins/tui/) - bubbletea
   - Chat-style interface like Claude Code
   - Message history view
   - Input prompt with command support
   - Status display (idle/working)
   - Requirement: Interactive mode only
 10. Telegram plugin (plugins/telegram/)
   - Bot setup with token from config
   - Message handler (commands + chat)
   - Fan-out: receive & broadcast messages
   - Requirement: TELEGRAM_TOKEN, daemon mode
 11. WebSocket plugin (plugins/websocket/)
   - WebSocket server on configurable port
   - JSON message protocol
   - Connection management
   - Requirement: Port availability
 12. REST API plugin (plugins/rest/)
   - HTTP server with /command, /status, /messages endpoints
   - JSON request/response
   - Optional authentication
   - Requirement: Port availability

 Phase 5: Executor Plugin (LLM Agent)

 13. Executor plugin interface (plugin/executor.go)
   - Define Executor interface
   - Methods: ExecuteTask, CancelTask, GetStatus
 14. LLM agent plugin (plugins/executor/llm/)
   - Integration with LLM API (OpenAI, Anthropic, etc.)
   - Task queue management
   - Streaming response support
   - Configuration for model, API keys
   - Publishes progress updates via broker

 Phase 6: Integration & Polish

 15. Main application (main.go)
   - Plugin imports with blank imports
   - CLI flags (--config, --mode)
   - Signal handling (graceful shutdown)
   - Startup sequence
 16. Testing
   - Unit tests for core components
   - Mock plugins for testing
   - Integration tests for message flow
   - Example configurations
 17. Documentation
   - README with architecture overview
   - Plugin development guide
   - Configuration examples
   - API documentation

 Key Design Decisions

 - Interface-based plugins: Compile-time safety, excellent performance
 - Init registration: Idiomatic Go pattern, automatic discovery
 - Three plugin types: Interaction, Executor, State (clean separation)
 - Message broker: Central hub for fan-out communication
 - Mode-aware commands: Commands check if they work in current mode
 - YAML config: Structured configuration with validation
 - Bubbletea TUI: Modern, maintainable terminal interface

 Dependencies

 - github.com/charmbracelet/bubbletea - TUI framework
 - github.com/go-telegram-bot-api/telegram-bot-api/v5 - Telegram bot
 - github.com/gorilla/websocket - WebSocket support
 - gopkg.in/yaml.v3 - YAML config parsing
 - golang.org/x/sync/errgroup - Error group for fan-out

 Estimated Implementation Order

 1. Core (Phases 1-2): ~30-40 files
 2. State plugin (Phase 3): ~5-8 files
 3. TUI plugin (Phase 4.1): ~10-12 files
 4. Other interaction plugins (Phase 4.2-4): ~15-20 files each
 5. LLM executor (Phase 5): ~10-15 files
 6. Integration (Phase 6): ~5-10 files

 Total: ~100-150 files for complete implementation
