package plugin

import "context"

// ExtensionType represents the type of extension
type ExtensionType string

const (
	// ExtensionTypeCommand represents a command extension
	ExtensionTypeCommand ExtensionType = "command"
	// ExtensionTypeExecutor represents a task executor extension
	ExtensionTypeExecutor ExtensionType = "executor"
	// ExtensionTypeState represents a state management extension
	ExtensionTypeState ExtensionType = "state"
	// ExtensionTypeInteraction represents an interaction channel extension
	ExtensionTypeInteraction ExtensionType = "interaction"
)

// Extension represents a capability provided by a plugin
type Extension interface {
	// Type returns the extension type
	Type() ExtensionType

	// Name returns the extension identifier
	Name() string

	// SupportsMode checks if the extension works in the given mode
	SupportsMode(mode Mode) bool
}

// Command represents a command that can be executed
type Command struct {
	// Name is the command identifier (e.g., "status", "reset")
	Name string

	// Description is a short description of what the command does
	Description string

	// Usage shows how to use the command
	Usage string

	// Handler is the function that executes the command
	Handler CommandHandler

	// Modes lists the modes in which this command is available
	Modes []Mode

	// Hidden indicates if the command should be hidden from help
	Hidden bool
}

// CommandHandler processes a command and returns a result
type CommandHandler func(ctx context.Context, args []string) (*CommandResult, error)

// CommandResult contains the result of command execution
type CommandResult struct {
	// Output is the text output to display
	Output string

	// Data contains structured data (for API responses)
	Data interface{}

	// Broadcast indicates if this result should be sent to all channels
	Broadcast bool
}

// CommandExtension wraps a command as an extension
type CommandExtension struct {
	command *Command
}

// NewCommandExtension creates a new command extension
func NewCommandExtension(cmd *Command) *CommandExtension {
	return &CommandExtension{command: cmd}
}

// Type returns the extension type
func (c *CommandExtension) Type() ExtensionType {
	return ExtensionTypeCommand
}

// Name returns the command name
func (c *CommandExtension) Name() string {
	return c.command.Name
}

// SupportsMode checks if the command supports the given mode
func (c *CommandExtension) SupportsMode(mode Mode) bool {
	if len(c.command.Modes) == 0 {
		return true // If no modes specified, available in all modes
	}
	for _, m := range c.command.Modes {
		if m == mode {
			return true
		}
	}
	return false
}

// Command returns the underlying command
func (c *CommandExtension) Command() *Command {
	return c.command
}

// Executor defines the interface for task execution
type Executor interface {
	Extension

	// ExecuteTask starts executing a task
	ExecuteTask(ctx context.Context, task *Task) error

	// CancelTask cancels a running task
	CancelTask(ctx context.Context, taskID string) error

	// GetStatus returns the current execution status
	GetStatus(ctx context.Context) (*ExecutorStatus, error)
}

// Task represents a task to be executed
type Task struct {
	// ID is the unique task identifier
	ID string

	// Type indicates what kind of task this is
	Type string

	// Input contains the task input data
	Input interface{}

	// Options contains task-specific options
	Options map[string]interface{}
}

// ExecutorStatus represents the current state of an executor
type ExecutorStatus struct {
	// State is the current executor state
	State ExecutorState

	// CurrentTask is the task currently being executed (if any)
	CurrentTask *Task

	// Progress indicates task completion percentage (0-100)
	Progress int

	// Message contains a status message
	Message string
}

// ExecutorState represents the state of a task executor
type ExecutorState string

const (
	// ExecutorStateIdle indicates the executor is idle
	ExecutorStateIdle ExecutorState = "idle"
	// ExecutorStateWorking indicates the executor is working on a task
	ExecutorStateWorking ExecutorState = "working"
	// ExecutorStateError indicates the executor encountered an error
	ExecutorStateError ExecutorState = "error"
)

// StateManager defines the interface for state persistence
type StateManager interface {
	Extension

	// Get retrieves a value by key
	Get(ctx context.Context, key string) (interface{}, error)

	// Set stores a value by key
	Set(ctx context.Context, key string, value interface{}) error

	// Delete removes a value by key
	Delete(ctx context.Context, key string) error

	// Save persists the current state (for file/db-based implementations)
	Save(ctx context.Context) error

	// Load loads the state from persistent storage
	Load(ctx context.Context) error
}
