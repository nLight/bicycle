package state

import (
	"context"
	"errors"
)

// Common errors
var (
	ErrNotFound = errors.New("key not found")
	ErrClosed   = errors.New("backend is closed")
)

// Backend defines the interface for state persistence
type Backend interface {
	// Get retrieves a value by key
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores a value by key
	Set(ctx context.Context, key string, value []byte) error

	// Delete removes a value by key
	Delete(ctx context.Context, key string) error

	// List returns all keys matching a prefix
	List(ctx context.Context, prefix string) ([]string, error)

	// Close releases any resources held by the backend
	Close() error
}

// TypedBackend provides type-safe operations on top of Backend
type TypedBackend struct {
	backend Backend
	codec   Codec
}

// Codec handles encoding/decoding of values
type Codec interface {
	Encode(v interface{}) ([]byte, error)
	Decode(data []byte, v interface{}) error
}

// NewTypedBackend creates a new TypedBackend
func NewTypedBackend(backend Backend, codec Codec) *TypedBackend {
	return &TypedBackend{
		backend: backend,
		codec:   codec,
	}
}

// Get retrieves and decodes a value
func (tb *TypedBackend) Get(ctx context.Context, key string, v interface{}) error {
	data, err := tb.backend.Get(ctx, key)
	if err != nil {
		return err
	}
	return tb.codec.Decode(data, v)
}

// Set encodes and stores a value
func (tb *TypedBackend) Set(ctx context.Context, key string, v interface{}) error {
	data, err := tb.codec.Encode(v)
	if err != nil {
		return err
	}
	return tb.backend.Set(ctx, key, data)
}

// Delete removes a value
func (tb *TypedBackend) Delete(ctx context.Context, key string) error {
	return tb.backend.Delete(ctx, key)
}

// List returns keys matching a prefix
func (tb *TypedBackend) List(ctx context.Context, prefix string) ([]string, error) {
	return tb.backend.List(ctx, prefix)
}

// Close closes the underlying backend
func (tb *TypedBackend) Close() error {
	return tb.backend.Close()
}
