package cmd

import (
	"context"
	"testing"

	"bicycle/internal/ctxkeys"
	"bicycle/plugin"
)

func TestRouter_Route(t *testing.T) {
	// Save and restore global registry
	oldCommands := globalRegistry.commands
	globalRegistry.commands = make(map[string]*plugin.Command)
	defer func() { globalRegistry.commands = oldCommands }()

	// Register a test command
	Register(&plugin.Command{
		Name:        "testcmd",
		Description: "Test command",
		Handler: func(ctx context.Context, args []string) (*plugin.CommandResult, error) {
			return &plugin.CommandResult{Output: "executed"}, nil
		},
	})

	router := NewRouter()

	result, err := router.Route(context.Background(), "/testcmd")
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if result.Output != "executed" {
		t.Errorf("Expected output 'executed', got '%s'", result.Output)
	}
}

func TestRouter_RouteWithArgs(t *testing.T) {
	oldCommands := globalRegistry.commands
	globalRegistry.commands = make(map[string]*plugin.Command)
	defer func() { globalRegistry.commands = oldCommands }()

	var receivedArgs []string
	Register(&plugin.Command{
		Name: "argcmd",
		Handler: func(ctx context.Context, args []string) (*plugin.CommandResult, error) {
			receivedArgs = args
			return &plugin.CommandResult{Output: "ok"}, nil
		},
	})

	router := NewRouter()
	router.Route(context.Background(), "/argcmd arg1 arg2 arg3")

	if len(receivedArgs) != 3 {
		t.Fatalf("Expected 3 args, got %d", len(receivedArgs))
	}
	if receivedArgs[0] != "arg1" || receivedArgs[1] != "arg2" || receivedArgs[2] != "arg3" {
		t.Errorf("Args mismatch: %v", receivedArgs)
	}
}

func TestRouter_RouteWithoutSlash(t *testing.T) {
	oldCommands := globalRegistry.commands
	globalRegistry.commands = make(map[string]*plugin.Command)
	defer func() { globalRegistry.commands = oldCommands }()

	Register(&plugin.Command{
		Name: "noslash",
		Handler: func(ctx context.Context, args []string) (*plugin.CommandResult, error) {
			return &plugin.CommandResult{Output: "ok"}, nil
		},
	})

	router := NewRouter()
	result, err := router.Route(context.Background(), "noslash")
	if err != nil {
		t.Fatalf("Route without slash should work: %v", err)
	}
	if result.Output != "ok" {
		t.Error("Command should execute")
	}
}

func TestRouter_RouteUnknownCommand(t *testing.T) {
	oldCommands := globalRegistry.commands
	globalRegistry.commands = make(map[string]*plugin.Command)
	defer func() { globalRegistry.commands = oldCommands }()

	router := NewRouter()
	_, err := router.Route(context.Background(), "/unknown")
	if err == nil {
		t.Error("Should error on unknown command")
	}
}

func TestRouter_RouteEmptyCommand(t *testing.T) {
	router := NewRouter()
	_, err := router.Route(context.Background(), "")
	if err == nil {
		t.Error("Should error on empty command")
	}
}

func TestRouter_IsCommand(t *testing.T) {
	router := NewRouter()

	if !router.IsCommand("/help") {
		t.Error("/help should be a command")
	}
	if !router.IsCommand("  /help") {
		t.Error("  /help should be a command (after trim)")
	}
	if router.IsCommand("help") {
		t.Error("help without slash should not be a command")
	}
	if router.IsCommand("") {
		t.Error("empty string should not be a command")
	}
}

func TestRouter_ModeFiltering(t *testing.T) {
	oldCommands := globalRegistry.commands
	globalRegistry.commands = make(map[string]*plugin.Command)
	defer func() { globalRegistry.commands = oldCommands }()

	Register(&plugin.Command{
		Name:  "daemononly",
		Modes: []plugin.Mode{plugin.ModeDaemon},
		Handler: func(ctx context.Context, args []string) (*plugin.CommandResult, error) {
			return &plugin.CommandResult{Output: "ok"}, nil
		},
	})

	router := NewRouter()

	// Should work in daemon mode
	ctx := context.WithValue(context.Background(), ctxkeys.Mode, plugin.ModeDaemon)
	_, err := router.Route(ctx, "/daemononly")
	if err != nil {
		t.Errorf("Should work in daemon mode: %v", err)
	}

	// Should fail in interactive mode
	ctx = context.WithValue(context.Background(), ctxkeys.Mode, plugin.ModeInteractive)
	_, err = router.Route(ctx, "/daemononly")
	if err == nil {
		t.Error("Should fail in interactive mode")
	}
}

func TestRouter_GetHelp(t *testing.T) {
	oldCommands := globalRegistry.commands
	globalRegistry.commands = make(map[string]*plugin.Command)
	defer func() { globalRegistry.commands = oldCommands }()

	Register(&plugin.Command{
		Name:        "cmd1",
		Description: "First command",
	})
	Register(&plugin.Command{
		Name:        "cmd2",
		Description: "Second command",
		Hidden:      true, // Should not appear in help
	})

	router := NewRouter()
	help := router.GetHelp(plugin.ModeDaemon)

	if help == "" {
		t.Error("Help should not be empty")
	}
	if !contains(help, "cmd1") {
		t.Error("Help should contain cmd1")
	}
	if contains(help, "cmd2") {
		t.Error("Help should not contain hidden cmd2")
	}
}

func TestRouter_GetCommandHelp(t *testing.T) {
	oldCommands := globalRegistry.commands
	globalRegistry.commands = make(map[string]*plugin.Command)
	defer func() { globalRegistry.commands = oldCommands }()

	Register(&plugin.Command{
		Name:        "testhelp",
		Description: "A test command for help",
		Usage:       "<arg1> [arg2]",
	})

	router := NewRouter()
	help, err := router.GetCommandHelp("testhelp")
	if err != nil {
		t.Fatalf("GetCommandHelp failed: %v", err)
	}

	if !contains(help, "testhelp") {
		t.Error("Help should contain command name")
	}
	if !contains(help, "A test command for help") {
		t.Error("Help should contain description")
	}
	if !contains(help, "<arg1> [arg2]") {
		t.Error("Help should contain usage")
	}
}

func TestRouter_GetCommandHelpNotFound(t *testing.T) {
	oldCommands := globalRegistry.commands
	globalRegistry.commands = make(map[string]*plugin.Command)
	defer func() { globalRegistry.commands = oldCommands }()

	router := NewRouter()
	_, err := router.GetCommandHelp("nonexistent")
	if err == nil {
		t.Error("Should error on nonexistent command")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
