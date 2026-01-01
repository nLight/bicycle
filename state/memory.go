package state

import (
	"context"
	"strings"
	"sync"
)

// MemoryBackend implements Backend using an in-memory map
// Useful for testing and temporary state
type MemoryBackend struct {
	mu     sync.RWMutex
	data   map[string][]byte
	closed bool
}

// NewMemoryBackend creates a new in-memory backend
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{
		data: make(map[string][]byte),
	}
}

// Get retrieves a value from memory
func (mb *MemoryBackend) Get(ctx context.Context, key string) ([]byte, error) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	if mb.closed {
		return nil, ErrClosed
	}

	data, exists := mb.data[key]
	if !exists {
		return nil, ErrNotFound
	}

	// Return a copy to prevent mutation
	result := make([]byte, len(data))
	copy(result, data)
	return result, nil
}

// Set stores a value in memory
func (mb *MemoryBackend) Set(ctx context.Context, key string, value []byte) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	if mb.closed {
		return ErrClosed
	}

	// Store a copy to prevent external mutation
	data := make([]byte, len(value))
	copy(data, value)
	mb.data[key] = data
	return nil
}

// Delete removes a value from memory
func (mb *MemoryBackend) Delete(ctx context.Context, key string) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	if mb.closed {
		return ErrClosed
	}

	delete(mb.data, key)
	return nil
}

// List returns all keys matching a prefix
func (mb *MemoryBackend) List(ctx context.Context, prefix string) ([]string, error) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	if mb.closed {
		return nil, ErrClosed
	}

	var keys []string
	for key := range mb.data {
		if prefix == "" || strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// Close marks the backend as closed
func (mb *MemoryBackend) Close() error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	mb.closed = true
	mb.data = nil
	return nil
}

// Clear removes all data (useful for testing)
func (mb *MemoryBackend) Clear() {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	mb.data = make(map[string][]byte)
}
