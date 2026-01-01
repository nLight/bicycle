package state

import (
	"context"
	"fmt"
	"sync"
)

// Manager provides typed state management operations
type Manager struct {
	backend Backend
	codec   Codec
	mu      sync.RWMutex
}

// NewManager creates a new state manager with the given backend
func NewManager(backend Backend) *Manager {
	return &Manager{
		backend: backend,
		codec:   DefaultCodec(),
	}
}

// NewManagerWithCodec creates a new state manager with custom codec
func NewManagerWithCodec(backend Backend, codec Codec) *Manager {
	return &Manager{
		backend: backend,
		codec:   codec,
	}
}

// Get retrieves and decodes a value
func (m *Manager) Get(ctx context.Context, key string, v interface{}) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, err := m.backend.Get(ctx, key)
	if err != nil {
		return err
	}

	return m.codec.Decode(data, v)
}

// Set encodes and stores a value
func (m *Manager) Set(ctx context.Context, key string, v interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := m.codec.Encode(v)
	if err != nil {
		return fmt.Errorf("failed to encode value: %w", err)
	}

	return m.backend.Set(ctx, key, data)
}

// Delete removes a value
func (m *Manager) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.backend.Delete(ctx, key)
}

// List returns all keys matching a prefix
func (m *Manager) List(ctx context.Context, prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.backend.List(ctx, prefix)
}

// Exists checks if a key exists
func (m *Manager) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, err := m.backend.Get(ctx, key)
	if err == ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetOrDefault retrieves a value or returns a default if not found
func (m *Manager) GetOrDefault(ctx context.Context, key string, v interface{}, defaultVal interface{}) error {
	err := m.Get(ctx, key, v)
	if err == ErrNotFound {
		// Use reflection to copy default value
		data, err := m.codec.Encode(defaultVal)
		if err != nil {
			return err
		}
		return m.codec.Decode(data, v)
	}
	return err
}

// Close closes the underlying backend
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.backend.Close()
}

// Backend returns the underlying backend (useful for testing)
func (m *Manager) Backend() Backend {
	return m.backend
}
