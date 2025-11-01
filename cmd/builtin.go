package cmd

import (
	"context"
	"fmt"
	"strings"

	"bicycle/plugin"
)

// init registers built-in commands
func init() {
	Register(&plugin.Command{
		Name:        "help",
		Description: "Show available commands or help for a specific command",
		Usage:       "[command]",
		Handler:     handleHelp,
		Modes:       []plugin.Mode{plugin.ModeDaemon, plugin.ModeInteractive},
	})

	Register(&plugin.Command{
		Name:        "status",
		Description: "Show daemon status and active plugins",
		Usage:       "",
		Handler:     handleStatus,
		Modes:       []plugin.Mode{plugin.ModeDaemon, plugin.ModeInteractive},
	})

	Register(&plugin.Command{
		Name:        "reset",
		Description: "Stop current task and reset to idle state",
		Usage:       "",
		Handler:     handleReset,
		Modes:       []plugin.Mode{plugin.ModeDaemon, plugin.ModeInteractive},
	})

	Register(&plugin.Command{
		Name:        "plugins",
		Description: "List all registered plugins",
		Usage:       "",
		Handler:     handlePlugins,
		Modes:       []plugin.Mode{plugin.ModeDaemon, plugin.ModeInteractive},
	})
}

// handleHelp shows help for all commands or a specific command
func handleHelp(ctx context.Context, args []string) (*plugin.CommandResult, error) {
	router := NewRouter()

	// If specific command requested, show its help
	if len(args) > 0 {
		cmdName := strings.TrimPrefix(args[0], "/")
		helpText, err := router.GetCommandHelp(cmdName)
		if err != nil {
			return nil, err
		}
		return &plugin.CommandResult{Output: helpText}, nil
	}

	// Otherwise show all commands
	mode, ok := ctx.Value("mode").(plugin.Mode)
	if !ok {
		mode = plugin.ModeDaemon // Default to daemon mode
	}

	helpText := router.GetHelp(mode)
	return &plugin.CommandResult{Output: helpText}, nil
}

// handleStatus shows the current daemon status
func handleStatus(ctx context.Context, args []string) (*plugin.CommandResult, error) {
	// Try to get daemon instance from context
	daemon, ok := ctx.Value("daemon").(StatusProvider)
	if !ok {
		return &plugin.CommandResult{
			Output: "Status: Running (daemon context not available)",
		}, nil
	}

	status := daemon.GetStatus(ctx)
	return &plugin.CommandResult{
		Output: status,
		Data:   nil, // Could add structured data here
	}, nil
}

// handleReset resets the daemon to idle state
func handleReset(ctx context.Context, args []string) (*plugin.CommandResult, error) {
	// Try to get daemon instance from context
	daemon, ok := ctx.Value("daemon").(Resettable)
	if !ok {
		return nil, fmt.Errorf("reset not available (daemon context not available)")
	}

	if err := daemon.Reset(ctx); err != nil {
		return nil, fmt.Errorf("reset failed: %w", err)
	}

	return &plugin.CommandResult{
		Output:    "Daemon reset to idle state",
		Broadcast: true, // Broadcast to all channels
	}, nil
}

// handlePlugins lists all registered plugins
func handlePlugins(ctx context.Context, args []string) (*plugin.CommandResult, error) {
	registry := plugin.GetRegistry()
	plugins := registry.All()

	if len(plugins) == 0 {
		return &plugin.CommandResult{
			Output: "No plugins registered",
		}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Registered plugins (%d):\n\n", len(plugins)))

	for i, p := range plugins {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, p.Name()))

		// Show extensions
		extensions := p.Extensions()
		if len(extensions) > 0 {
			sb.WriteString(fmt.Sprintf("   Extensions: "))
			var extNames []string
			for _, ext := range extensions {
				extNames = append(extNames, fmt.Sprintf("%s:%s", ext.Type(), ext.Name()))
			}
			sb.WriteString(strings.Join(extNames, ", "))
			sb.WriteString("\n")
		}
	}

	return &plugin.CommandResult{
		Output: sb.String(),
	}, nil
}

// StatusProvider interface for getting daemon status
type StatusProvider interface {
	GetStatus(ctx context.Context) string
}

// Resettable interface for resetting daemon state
type Resettable interface {
	Reset(ctx context.Context) error
}
