package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileBackend implements Backend using the filesystem
// Each key is stored as a separate file in a directory
type FileBackend struct {
	mu      sync.RWMutex
	dir     string
	closed  bool
}

// NewFileBackend creates a new file-based backend
func NewFileBackend(dir string) (*FileBackend, error) {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	return &FileBackend{
		dir: dir,
	}, nil
}

// Get retrieves a value from a file
func (fb *FileBackend) Get(ctx context.Context, key string) ([]byte, error) {
	fb.mu.RLock()
	defer fb.mu.RUnlock()

	if fb.closed {
		return nil, ErrClosed
	}

	path := fb.keyToPath(key)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to read %s: %w", key, err)
	}

	return data, nil
}

// Set stores a value to a file
func (fb *FileBackend) Set(ctx context.Context, key string, value []byte) error {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	if fb.closed {
		return ErrClosed
	}

	path := fb.keyToPath(key)

	// Create parent directories if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", key, err)
	}

	// Write atomically using temp file + rename
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, value, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", key, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) // Clean up temp file
		return fmt.Errorf("failed to finalize %s: %w", key, err)
	}

	return nil
}

// Delete removes a file
func (fb *FileBackend) Delete(ctx context.Context, key string) error {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	if fb.closed {
		return ErrClosed
	}

	path := fb.keyToPath(key)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted, not an error
		}
		return fmt.Errorf("failed to delete %s: %w", key, err)
	}

	return nil
}

// List returns all keys matching a prefix
func (fb *FileBackend) List(ctx context.Context, prefix string) ([]string, error) {
	fb.mu.RLock()
	defer fb.mu.RUnlock()

	if fb.closed {
		return nil, ErrClosed
	}

	var keys []string
	searchDir := fb.dir

	// If prefix contains path separators, search in subdirectory
	if prefix != "" {
		prefixDir := filepath.Dir(fb.keyToPath(prefix))
		if _, err := os.Stat(prefixDir); err == nil {
			searchDir = prefixDir
		}
	}

	err := filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible files
		}
		if info.IsDir() {
			return nil
		}
		// Skip temp files
		if strings.HasSuffix(path, ".tmp") {
			return nil
		}

		key := fb.pathToKey(path)
		if prefix == "" || strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}

	return keys, nil
}

// Close marks the backend as closed
func (fb *FileBackend) Close() error {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	fb.closed = true
	return nil
}

// keyToPath converts a key to a file path
// Keys can use "/" as separators which become subdirectories
func (fb *FileBackend) keyToPath(key string) string {
	// Sanitize key to prevent path traversal
	key = filepath.Clean(key)
	key = strings.TrimPrefix(key, "/")
	key = strings.TrimPrefix(key, "../")

	return filepath.Join(fb.dir, key+".json")
}

// pathToKey converts a file path back to a key
func (fb *FileBackend) pathToKey(path string) string {
	rel, err := filepath.Rel(fb.dir, path)
	if err != nil {
		return path
	}
	// Remove .json extension
	return strings.TrimSuffix(rel, ".json")
}
