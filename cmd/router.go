package cmd

import (
	"context"
	"fmt"
	"strings"

	"bicycle/plugin"
)

// Router handles command parsing and routing
type Router struct {
	registry *CommandRegistry
}

// NewRouter creates a new command router
func NewRouter() *Router {
	return &Router{
		registry: GetRegistry(),
	}
}

// Route parses and routes a command string to the appropriate handler
// Supports formats:
//   - "/command arg1 arg2" (slash prefix)
//   - "command arg1 arg2" (no slash)
func (r *Router) Route(ctx context.Context, input string) (*plugin.CommandResult, error) {
	// Trim whitespace
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("empty command")
	}

	// Parse command and arguments
	cmdName, args := r.parseCommand(input)
	if cmdName == "" {
		return nil, fmt.Errorf("invalid command format")
	}

	// Execute command
	return r.registry.Execute(ctx, cmdName, args)
}

// parseCommand splits a command string into name and arguments
// Handles both "/command" and "command" formats
func (r *Router) parseCommand(input string) (string, []string) {
	// Remove leading slash if present
	input = strings.TrimPrefix(input, "/")

	// Split into tokens
	tokens := strings.Fields(input)
	if len(tokens) == 0 {
		return "", nil
	}

	cmdName := tokens[0]
	args := tokens[1:]

	return cmdName, args
}

// IsCommand checks if a string looks like a command
func (r *Router) IsCommand(input string) bool {
	input = strings.TrimSpace(input)
	return strings.HasPrefix(input, "/")
}

// GetHelp returns help text for all available commands
func (r *Router) GetHelp(mode plugin.Mode) string {
	commands := r.registry.ListCommands(mode)

	if len(commands) == 0 {
		return "No commands available."
	}

	var sb strings.Builder
	sb.WriteString("Available commands:\n\n")

	for _, cmd := range commands {
		sb.WriteString(fmt.Sprintf("/%s", cmd.Name))
		if cmd.Usage != "" {
			sb.WriteString(fmt.Sprintf(" %s", cmd.Usage))
		}
		sb.WriteString("\n")

		if cmd.Description != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", cmd.Description))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// GetCommandHelp returns help text for a specific command
func (r *Router) GetCommandHelp(cmdName string) (string, error) {
	cmd, exists := r.registry.Get(cmdName)
	if !exists {
		return "", fmt.Errorf("unknown command: %s", cmdName)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Command: /%s\n\n", cmd.Name))

	if cmd.Description != "" {
		sb.WriteString(fmt.Sprintf("%s\n\n", cmd.Description))
	}

	if cmd.Usage != "" {
		sb.WriteString(fmt.Sprintf("Usage: /%s %s\n", cmd.Name, cmd.Usage))
	}

	if len(cmd.Modes) > 0 {
		sb.WriteString(fmt.Sprintf("\nAvailable in modes: %v", cmd.Modes))
	}

	return sb.String(), nil
}
