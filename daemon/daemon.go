package daemon

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"bicycle/internal/config"
	"bicycle/plugin"
)

// State represents the daemon's current state
type State string

const (
	// StateIdle indicates the daemon is idle
	StateIdle State = "idle"
	// StateWorking indicates the daemon is working on a task
	StateWorking State = "working"
	// StateStopped indicates the daemon has been stopped
	StateStopped State = "stopped"

	// DefaultShutdownTimeout is the default timeout for graceful shutdown
	DefaultShutdownTimeout = 10 * time.Second
)

// Daemon represents the main daemon instance
type Daemon struct {
	mu      sync.RWMutex
	state   State
	config  *config.Config
	broker  *Broker
	plugins map[string]plugin.Plugin
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// Current task information
	currentTask *plugin.Task
	executor    plugin.Executor
}

// New creates a new daemon instance
func New(cfg *config.Config) *Daemon {
	ctx, cancel := context.WithCancel(context.Background())

	return &Daemon{
		state:   StateIdle,
		config:  cfg,
		broker:  NewBroker(),
		plugins: make(map[string]plugin.Plugin),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// AddPlugin adds a plugin to the daemon
func (d *Daemon) AddPlugin(p plugin.Plugin) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	name := p.Name()

	// Check if plugin is enabled in config
	if !d.config.IsPluginEnabled(name) {
		log.Printf("[Daemon] Plugin %s is disabled in config, skipping", name)
		return nil
	}

	if _, exists := d.plugins[name]; exists {
		return fmt.Errorf("plugin %s already added", name)
	}

	d.plugins[name] = p
	log.Printf("[Daemon] Added plugin: %s", name)

	return nil
}

// Start starts the daemon and all registered plugins
func (d *Daemon) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.state != StateIdle {
		return fmt.Errorf("daemon already started")
	}

	log.Println("[Daemon] Starting daemon...")

	// Create context with mode
	ctx := context.WithValue(d.ctx, "mode", d.config.Mode)
	ctx = context.WithValue(ctx, "daemon", d)
	ctx = context.WithValue(ctx, "config", d.config)

	// Configure broker
	d.broker.SetPublishTimeout(time.Duration(d.config.Daemon.PublishTimeout) * time.Second)

	// Start plugins
	for name, p := range d.plugins {
		log.Printf("[Daemon] Checking requirements for plugin: %s", name)

		// Check requirements
		if err := p.CheckRequirements(ctx); err != nil {
			log.Printf("[Daemon] Plugin %s requirements failed: %v", name, err)
			log.Printf("[Daemon] Skipping plugin: %s", name)
			delete(d.plugins, name)
			continue
		}

		// Start plugin
		log.Printf("[Daemon] Starting plugin: %s", name)
		if err := p.Start(ctx, d.broker); err != nil {
			log.Printf("[Daemon] Failed to start plugin %s: %v", name, err)
			delete(d.plugins, name)
			continue
		}

		// Check for executor extension
		for _, ext := range p.Extensions() {
			if ext.Type() == plugin.ExtensionTypeExecutor {
				if executor, ok := ext.(plugin.Executor); ok {
					d.executor = executor
					log.Printf("[Daemon] Registered executor from plugin: %s", name)
				}
			}
		}

		log.Printf("[Daemon] Started plugin: %s", name)
	}

	log.Printf("[Daemon] Started with %d active plugin(s)", len(d.plugins))

	return nil
}

// Stop stops the daemon and all plugins
func (d *Daemon) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.state == StateStopped {
		return nil
	}

	log.Println("[Daemon] Stopping daemon...")

	// Cancel context
	d.cancel()

	// Stop all plugins
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for name, p := range d.plugins {
		log.Printf("[Daemon] Stopping plugin: %s", name)
		if err := p.Stop(ctx); err != nil {
			log.Printf("[Daemon] Error stopping plugin %s: %v", name, err)
		}
	}

	// Close broker
	d.broker.Close()

	// Wait for goroutines
	d.wg.Wait()

	d.state = StateStopped
	log.Println("[Daemon] Stopped")

	return nil
}

// Reset resets the daemon to idle state
func (d *Daemon) Reset(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.state != StateWorking {
		return fmt.Errorf("daemon is not working")
	}

	log.Println("[Daemon] Resetting to idle state...")

	// Cancel current task if there's an executor
	if d.executor != nil && d.currentTask != nil {
		if err := d.executor.CancelTask(ctx, d.currentTask.ID); err != nil {
			log.Printf("[Daemon] Error cancelling task: %v", err)
		}
	}

	d.currentTask = nil
	d.state = StateIdle

	log.Println("[Daemon] Reset to idle state")

	return nil
}

// GetState returns the current daemon state
func (d *Daemon) GetState() State {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state
}

// SetState sets the daemon state
func (d *Daemon) SetState(state State) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.state = state
	log.Printf("[Daemon] State changed to: %s", state)
}

// GetStatus returns a status string for the daemon
func (d *Daemon) GetStatus(ctx context.Context) string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	status := fmt.Sprintf("Daemon Status:\n")
	status += fmt.Sprintf("  State: %s\n", d.state)
	status += fmt.Sprintf("  Mode: %s\n", d.config.Mode)
	status += fmt.Sprintf("  Active Plugins: %d\n", len(d.plugins))

	if d.state == StateWorking && d.currentTask != nil {
		status += fmt.Sprintf("  Current Task: %s (ID: %s)\n", d.currentTask.Type, d.currentTask.ID)

		// Get executor status if available
		if d.executor != nil {
			if execStatus, err := d.executor.GetStatus(ctx); err == nil {
				status += fmt.Sprintf("  Progress: %d%%\n", execStatus.Progress)
				if execStatus.Message != "" {
					status += fmt.Sprintf("  Message: %s\n", execStatus.Message)
				}
			}
		}
	}

	return status
}

// GetBroker returns the message broker
func (d *Daemon) GetBroker() *Broker {
	return d.broker
}

// GetConfig returns the daemon configuration
func (d *Daemon) GetConfig() *config.Config {
	return d.config
}

// GetPlugins returns all active plugins
func (d *Daemon) GetPlugins() []plugin.Plugin {
	d.mu.RLock()
	defer d.mu.RUnlock()

	plugins := make([]plugin.Plugin, 0, len(d.plugins))
	for _, p := range d.plugins {
		plugins = append(plugins, p)
	}
	return plugins
}

// ExecuteTask executes a task using the registered executor
func (d *Daemon) ExecuteTask(ctx context.Context, task *plugin.Task) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.state != StateIdle {
		return fmt.Errorf("daemon is not idle (current state: %s)", d.state)
	}

	if d.executor == nil {
		return fmt.Errorf("no executor available")
	}

	d.currentTask = task
	d.state = StateWorking

	log.Printf("[Daemon] Executing task: %s (ID: %s)", task.Type, task.ID)

	// Execute in background
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()

		if err := d.executor.ExecuteTask(ctx, task); err != nil {
			log.Printf("[Daemon] Task execution failed: %v", err)
			// Publish error message
			d.broker.Publish(ctx, plugin.Message{
				Topic:   "notification",
				Payload: fmt.Sprintf("Task failed: %v", err),
				Source:  "daemon",
			})
		} else {
			log.Printf("[Daemon] Task completed successfully")
			// Publish completion message
			d.broker.Publish(ctx, plugin.Message{
				Topic:   "notification",
				Payload: "Task completed successfully",
				Source:  "daemon",
			})
		}

		// Reset state
		d.mu.Lock()
		d.state = StateIdle
		d.currentTask = nil
		d.mu.Unlock()
	}()

	return nil
}
