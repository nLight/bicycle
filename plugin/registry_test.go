package plugin

import (
	"context"
	"testing"
)

// mockTestPlugin is a minimal plugin for testing
type mockTestPlugin struct {
	name       string
	extensions []Extension
}

func (m *mockTestPlugin) Name() string                                          { return m.name }
func (m *mockTestPlugin) CheckRequirements(ctx context.Context) error           { return nil }
func (m *mockTestPlugin) Extensions() []Extension                               { return m.extensions }
func (m *mockTestPlugin) Start(ctx context.Context, broker MessageBroker) error { return nil }
func (m *mockTestPlugin) Stop(ctx context.Context) error                        { return nil }

func TestRegistry_RegisterAndGet(t *testing.T) {
	// Create fresh registry for testing
	registry := &Registry{
		plugins: make(map[string]Plugin),
	}

	p := &mockTestPlugin{name: "test-plugin"}
	registry.registerPlugin(p)

	got, exists := registry.Get("test-plugin")
	if !exists {
		t.Fatal("Plugin should exist after registration")
	}
	if got.Name() != "test-plugin" {
		t.Errorf("Expected plugin name 'test-plugin', got '%s'", got.Name())
	}
}

func TestRegistry_GetNotFound(t *testing.T) {
	registry := &Registry{
		plugins: make(map[string]Plugin),
	}

	_, exists := registry.Get("nonexistent")
	if exists {
		t.Error("Should not find nonexistent plugin")
	}
}

func TestRegistry_All(t *testing.T) {
	registry := &Registry{
		plugins: make(map[string]Plugin),
	}

	p1 := &mockTestPlugin{name: "plugin-a"}
	p2 := &mockTestPlugin{name: "plugin-b"}
	p3 := &mockTestPlugin{name: "plugin-c"}

	registry.registerPlugin(p1)
	registry.registerPlugin(p2)
	registry.registerPlugin(p3)

	all := registry.All()
	if len(all) != 3 {
		t.Errorf("Expected 3 plugins, got %d", len(all))
	}
}

func TestRegistry_Count(t *testing.T) {
	registry := &Registry{
		plugins: make(map[string]Plugin),
	}

	if registry.Count() != 0 {
		t.Error("Empty registry should have count 0")
	}

	registry.registerPlugin(&mockTestPlugin{name: "p1"})
	registry.registerPlugin(&mockTestPlugin{name: "p2"})

	if registry.Count() != 2 {
		t.Errorf("Expected count 2, got %d", registry.Count())
	}
}

func TestRegistry_Clear(t *testing.T) {
	registry := &Registry{
		plugins: make(map[string]Plugin),
	}

	registry.registerPlugin(&mockTestPlugin{name: "p1"})
	registry.registerPlugin(&mockTestPlugin{name: "p2"})

	registry.Clear()

	if registry.Count() != 0 {
		t.Errorf("Expected 0 after clear, got %d", registry.Count())
	}
}

func TestRegistry_Names(t *testing.T) {
	registry := &Registry{
		plugins: make(map[string]Plugin),
	}

	registry.registerPlugin(&mockTestPlugin{name: "alpha"})
	registry.registerPlugin(&mockTestPlugin{name: "beta"})

	names := registry.Names()
	if len(names) != 2 {
		t.Errorf("Expected 2 names, got %d", len(names))
	}
}

// internal helper to register without using the global registry
func (r *Registry) registerPlugin(p Plugin) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins[p.Name()] = p
}
