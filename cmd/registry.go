package cmd

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"

	"bicycle/plugin"
)

var (
	// globalRegistry is the global command registry
	globalRegistry = &CommandRegistry{
		commands: make(map[string]*plugin.Command),
	}
)

// CommandRegistry manages command registration and execution
type CommandRegistry struct {
	mu       sync.RWMutex
	commands map[string]*plugin.Command
}

// Register adds a command to the global registry
// This is typically called from plugin init() functions
func Register(cmd *plugin.Command) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	if _, exists := globalRegistry.commands[cmd.Name]; exists {
		panic(fmt.Sprintf("command %s already registered", cmd.Name))
	}

	globalRegistry.commands[cmd.Name] = cmd
	log.Printf("[CommandRegistry] Registered command: /%s", cmd.Name)
}

// GetRegistry returns the global command registry
func GetRegistry() *CommandRegistry {
	return globalRegistry
}

// Get retrieves a command by name
func (cr *CommandRegistry) Get(name string) (*plugin.Command, bool) {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	cmd, exists := cr.commands[name]
	return cmd, exists
}

// All returns all registered commands
func (cr *CommandRegistry) All() []*plugin.Command {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	commands := make([]*plugin.Command, 0, len(cr.commands))
	for _, cmd := range cr.commands {
		commands = append(commands, cmd)
	}

	// Sort by name for consistent output
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Name < commands[j].Name
	})

	return commands
}

// ListCommands returns available commands for the given mode
func (cr *CommandRegistry) ListCommands(mode plugin.Mode) []*plugin.Command {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	var available []*plugin.Command
	for _, cmd := range cr.commands {
		// Skip hidden commands
		if cmd.Hidden {
			continue
		}

		// Check mode compatibility
		if len(cmd.Modes) == 0 || containsMode(cmd.Modes, mode) {
			available = append(available, cmd)
		}
	}

	// Sort by name for consistent output
	sort.Slice(available, func(i, j int) bool {
		return available[i].Name < available[j].Name
	})

	return available
}

// Execute dispatches a command to its handler
func (cr *CommandRegistry) Execute(ctx context.Context, name string, args []string) (*plugin.CommandResult, error) {
	cr.mu.RLock()
	cmd, exists := cr.commands[name]
	cr.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unknown command: %s", name)
	}

	// Check mode compatibility
	mode, ok := ctx.Value("mode").(plugin.Mode)
	if ok && len(cmd.Modes) > 0 && !containsMode(cmd.Modes, mode) {
		return nil, fmt.Errorf("command /%s not available in %s mode", name, mode)
	}

	// Execute the command
	log.Printf("[CommandRegistry] Executing command: /%s with %d arg(s)", name, len(args))
	return cmd.Handler(ctx, args)
}

// Count returns the number of registered commands
func (cr *CommandRegistry) Count() int {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return len(cr.commands)
}

// Clear removes all commands from the registry
// This is primarily useful for testing
func (cr *CommandRegistry) Clear() {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.commands = make(map[string]*plugin.Command)
}

// Helper function to check if a mode is in a slice
func containsMode(modes []plugin.Mode, mode plugin.Mode) bool {
	for _, m := range modes {
		if m == mode {
			return true
		}
	}
	return false
}
