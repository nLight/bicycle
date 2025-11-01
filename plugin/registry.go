package plugin

import (
	"fmt"
	"log"
	"sync"
)

var (
	// globalRegistry is the global plugin registry
	globalRegistry = &Registry{
		plugins: make(map[string]Plugin),
	}
)

// Registry manages plugin registration and retrieval
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
}

// Register adds a plugin to the global registry
// This is typically called from plugin init() functions
func Register(p Plugin) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	name := p.Name()
	if _, exists := globalRegistry.plugins[name]; exists {
		panic(fmt.Sprintf("plugin %s already registered", name))
	}

	globalRegistry.plugins[name] = p
	log.Printf("[Registry] Registered plugin: %s", name)
}

// GetRegistry returns the global plugin registry
func GetRegistry() *Registry {
	return globalRegistry
}

// Get retrieves a plugin by name
func (r *Registry) Get(name string) (Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.plugins[name]
	return p, exists
}

// All returns all registered plugins
func (r *Registry) All() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugins := make([]Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		plugins = append(plugins, p)
	}
	return plugins
}

// Names returns the names of all registered plugins
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.plugins))
	for name := range r.plugins {
		names = append(names, name)
	}
	return names
}

// Count returns the number of registered plugins
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.plugins)
}

// Clear removes all plugins from the registry
// This is primarily useful for testing
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.plugins = make(map[string]Plugin)
}
