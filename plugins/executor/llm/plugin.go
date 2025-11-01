package llm

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"bicycle/cmd"
	"bicycle/internal/config"
	"bicycle/plugin"
)

// init registers the LLM executor plugin
func init() {
	plugin.Register(NewLLMPlugin())

	// Register LLM-specific commands
	cmd.Register(&plugin.Command{
		Name:        "ask",
		Description: "Ask the LLM agent a question",
		Usage:       "<question>",
		Handler:     handleAsk,
		Modes:       []plugin.Mode{plugin.ModeDaemon, plugin.ModeInteractive},
	})
}

// LLMPlugin provides LLM-based task execution
type LLMPlugin struct {
	broker plugin.MessageBroker
	ctx    context.Context
	mu     sync.RWMutex

	// Executor state
	state       plugin.ExecutorState
	currentTask *plugin.Task
	progress    int
	message     string

	// Configuration
	provider string
	apiKey   string
	model    string
}

// NewLLMPlugin creates a new LLM executor plugin
func NewLLMPlugin() *LLMPlugin {
	return &LLMPlugin{
		state: plugin.ExecutorStateIdle,
	}
}

// Name returns the plugin name
func (p *LLMPlugin) Name() string {
	return "llm"
}

// CheckRequirements validates plugin requirements
func (p *LLMPlugin) CheckRequirements(ctx context.Context) error {
	checker := plugin.NewRequirementChecker("llm")

	// Get configuration
	p.provider, p.apiKey, p.model = p.getConfig(ctx)

	// Require API key
	checker.AddRequired(
		"api_key",
		"LLM API key required",
		func(ctx context.Context) error {
			if p.apiKey == "" {
				return fmt.Errorf("API key not set (check config or environment)")
			}
			return nil
		},
	)

	return checker.Check(ctx)
}

// getConfig retrieves LLM configuration
func (p *LLMPlugin) getConfig(ctx context.Context) (provider, apiKey, model string) {
	// Defaults
	provider = "openai"
	model = "gpt-4"

	// Try config
	if cfg, ok := ctx.Value("config").(*config.Config); ok {
		if prov, ok := cfg.GetPluginSettingString("llm", "provider"); ok {
			provider = prov
		}
		if mdl, ok := cfg.GetPluginSettingString("llm", "model"); ok {
			model = mdl
		}
		if key, ok := cfg.GetPluginSettingString("llm", "api_key"); ok && key != "" {
			apiKey = key
		}
	}

	// Fallback to environment variables
	if apiKey == "" {
		switch provider {
		case "openai":
			apiKey = os.Getenv("OPENAI_API_KEY")
		case "anthropic":
			apiKey = os.Getenv("ANTHROPIC_API_KEY")
		}
	}

	return provider, apiKey, model
}

// Extensions returns the plugin's extensions
func (p *LLMPlugin) Extensions() []plugin.Extension {
	return []plugin.Extension{
		NewLLMExecutorExtension(p),
	}
}

// Start initializes the LLM executor
func (p *LLMPlugin) Start(ctx context.Context, broker plugin.MessageBroker) error {
	p.broker = broker
	p.ctx = ctx

	log.Printf("[LLM] Started (provider: %s, model: %s)", p.provider, p.model)
	return nil
}

// Stop shuts down the LLM executor
func (p *LLMPlugin) Stop(ctx context.Context) error {
	// Cancel any running task
	if p.currentTask != nil {
		p.CancelTask(ctx, p.currentTask.ID)
	}

	log.Printf("[LLM] Stopped")
	return nil
}

// ExecuteTask executes a task using the LLM
func (p *LLMPlugin) ExecuteTask(ctx context.Context, task *plugin.Task) error {
	p.mu.Lock()
	if p.state != plugin.ExecutorStateIdle {
		p.mu.Unlock()
		return fmt.Errorf("executor is busy")
	}
	p.state = plugin.ExecutorStateWorking
	p.currentTask = task
	p.progress = 0
	p.message = "Starting task..."
	p.mu.Unlock()

	log.Printf("[LLM] Executing task: %s (ID: %s)", task.Type, task.ID)

	// Publish start notification
	p.broker.Publish(ctx, plugin.Message{
		Topic:   "notification",
		Payload: fmt.Sprintf("Started task: %s", task.Type),
		Source:  "llm",
	})

	// TODO: Implement actual LLM API calls
	// For now, this is a stub that simulates work
	for i := 0; i < 10; i++ {
		select {
		case <-ctx.Done():
			p.mu.Lock()
			p.state = plugin.ExecutorStateIdle
			p.currentTask = nil
			p.mu.Unlock()
			return ctx.Err()

		case <-time.After(1 * time.Second):
			p.mu.Lock()
			p.progress = (i + 1) * 10
			p.message = fmt.Sprintf("Processing... %d%%", p.progress)
			p.mu.Unlock()

			// Publish progress update
			p.broker.Publish(ctx, plugin.Message{
				Topic:   "notification",
				Payload: p.message,
				Source:  "llm",
			})
		}
	}

	// Complete task
	p.mu.Lock()
	p.state = plugin.ExecutorStateIdle
	p.currentTask = nil
	p.progress = 100
	p.message = "Task completed"
	p.mu.Unlock()

	log.Printf("[LLM] Task completed: %s", task.ID)

	// Publish completion
	p.broker.Publish(ctx, plugin.Message{
		Topic:   "notification",
		Payload: "Task completed successfully",
		Source:  "llm",
	})

	return nil
}

// CancelTask cancels a running task
func (p *LLMPlugin) CancelTask(ctx context.Context, taskID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.currentTask == nil || p.currentTask.ID != taskID {
		return fmt.Errorf("task not found: %s", taskID)
	}

	log.Printf("[LLM] Cancelling task: %s", taskID)

	// TODO: Implement actual cancellation logic
	p.state = plugin.ExecutorStateIdle
	p.currentTask = nil
	p.message = "Task cancelled"

	return nil
}

// GetStatus returns the current executor status
func (p *LLMPlugin) GetStatus(ctx context.Context) (*plugin.ExecutorStatus, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return &plugin.ExecutorStatus{
		State:       p.state,
		CurrentTask: p.currentTask,
		Progress:    p.progress,
		Message:     p.message,
	}, nil
}

// LLMExecutorExtension wraps the LLM plugin as an executor extension
type LLMExecutorExtension struct {
	plugin *LLMPlugin
}

// NewLLMExecutorExtension creates a new LLM executor extension
func NewLLMExecutorExtension(plugin *LLMPlugin) *LLMExecutorExtension {
	return &LLMExecutorExtension{plugin: plugin}
}

// Type returns the extension type
func (e *LLMExecutorExtension) Type() plugin.ExtensionType {
	return plugin.ExtensionTypeExecutor
}

// Name returns the extension name
func (e *LLMExecutorExtension) Name() string {
	return "llm"
}

// SupportsMode checks if the extension supports the given mode
func (e *LLMExecutorExtension) SupportsMode(mode plugin.Mode) bool {
	// LLM executor works in all modes
	return true
}

// Implement Executor interface
func (e *LLMExecutorExtension) ExecuteTask(ctx context.Context, task *plugin.Task) error {
	return e.plugin.ExecuteTask(ctx, task)
}

func (e *LLMExecutorExtension) CancelTask(ctx context.Context, taskID string) error {
	return e.plugin.CancelTask(ctx, taskID)
}

func (e *LLMExecutorExtension) GetStatus(ctx context.Context) (*plugin.ExecutorStatus, error) {
	return e.plugin.GetStatus(ctx)
}

// handleAsk is the command handler for /ask
func handleAsk(ctx context.Context, args []string) (*plugin.CommandResult, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: /ask <question>")
	}

	question := fmt.Sprintf("%v", args)

	// Get daemon from context to execute task
	daemon, ok := ctx.Value("daemon").(interface {
		ExecuteTask(context.Context, *plugin.Task) error
	})
	if !ok {
		return nil, fmt.Errorf("daemon not available in context")
	}

	// Create task
	task := &plugin.Task{
		ID:    fmt.Sprintf("ask-%d", time.Now().Unix()),
		Type:  "llm_query",
		Input: question,
	}

	// Execute task
	if err := daemon.ExecuteTask(ctx, task); err != nil {
		return nil, err
	}

	return &plugin.CommandResult{
		Output: fmt.Sprintf("Processing question: %s", question),
	}, nil
}
