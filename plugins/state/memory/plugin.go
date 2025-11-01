package memory

import (
	"context"
	"fmt"
	"log"
	"sync"

	"bicycle/plugin"
)

// init registers the memory state plugin
func init() {
	plugin.Register(NewMemoryStatePlugin())
}

// MemoryStatePlugin provides in-memory state storage
type MemoryStatePlugin struct {
	mu    sync.RWMutex
	state map[string]interface{}
}

// NewMemoryStatePlugin creates a new memory state plugin
func NewMemoryStatePlugin() *MemoryStatePlugin {
	return &MemoryStatePlugin{
		state: make(map[string]interface{}),
	}
}

// Name returns the plugin name
func (p *MemoryStatePlugin) Name() string {
	return "state_memory"
}

// CheckRequirements validates plugin requirements
func (p *MemoryStatePlugin) CheckRequirements(ctx context.Context) error {
	// Memory state has no external requirements
	return nil
}

// Extensions returns the plugin's extensions
func (p *MemoryStatePlugin) Extensions() []plugin.Extension {
	return []plugin.Extension{
		NewMemoryStateExtension(p),
	}
}

// Start initializes the plugin
func (p *MemoryStatePlugin) Start(ctx context.Context, broker plugin.MessageBroker) error {
	log.Printf("[MemoryState] Started")
	return nil
}

// Stop gracefully shuts down the plugin
func (p *MemoryStatePlugin) Stop(ctx context.Context) error {
	log.Printf("[MemoryState] Stopped")
	return nil
}

// Get retrieves a value by key
func (p *MemoryStatePlugin) Get(ctx context.Context, key string) (interface{}, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	val, exists := p.state[key]
	if !exists {
		return nil, fmt.Errorf("key not found: %s", key)
	}

	return val, nil
}

// Set stores a value by key
func (p *MemoryStatePlugin) Set(ctx context.Context, key string, value interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.state[key] = value
	log.Printf("[MemoryState] Set: %s", key)

	return nil
}

// Delete removes a value by key
func (p *MemoryStatePlugin) Delete(ctx context.Context, key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.state, key)
	log.Printf("[MemoryState] Deleted: %s", key)

	return nil
}

// Save persists state (no-op for memory plugin)
func (p *MemoryStatePlugin) Save(ctx context.Context) error {
	// Memory state is not persistent
	log.Printf("[MemoryState] Save called (no-op for memory plugin)")
	return nil
}

// Load loads state (no-op for memory plugin)
func (p *MemoryStatePlugin) Load(ctx context.Context) error {
	// Memory state starts empty
	log.Printf("[MemoryState] Load called (no-op for memory plugin)")
	return nil
}

// MemoryStateExtension wraps the memory state plugin as an extension
type MemoryStateExtension struct {
	plugin *MemoryStatePlugin
}

// NewMemoryStateExtension creates a new memory state extension
func NewMemoryStateExtension(plugin *MemoryStatePlugin) *MemoryStateExtension {
	return &MemoryStateExtension{plugin: plugin}
}

// Type returns the extension type
func (e *MemoryStateExtension) Type() plugin.ExtensionType {
	return plugin.ExtensionTypeState
}

// Name returns the extension name
func (e *MemoryStateExtension) Name() string {
	return "memory"
}

// SupportsMode checks if the extension supports the given mode
func (e *MemoryStateExtension) SupportsMode(mode plugin.Mode) bool {
	// Memory state works in all modes
	return true
}

// Implement StateManager interface
func (e *MemoryStateExtension) Get(ctx context.Context, key string) (interface{}, error) {
	return e.plugin.Get(ctx, key)
}

func (e *MemoryStateExtension) Set(ctx context.Context, key string, value interface{}) error {
	return e.plugin.Set(ctx, key, value)
}

func (e *MemoryStateExtension) Delete(ctx context.Context, key string) error {
	return e.plugin.Delete(ctx, key)
}

func (e *MemoryStateExtension) Save(ctx context.Context) error {
	return e.plugin.Save(ctx)
}

func (e *MemoryStateExtension) Load(ctx context.Context) error {
	return e.plugin.Load(ctx)
}
