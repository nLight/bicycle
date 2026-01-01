package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryBackend_Basic(t *testing.T) {
	mb := NewMemoryBackend()
	ctx := context.Background()

	// Test Set and Get
	err := mb.Set(ctx, "key1", []byte("value1"))
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	data, err := mb.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(data) != "value1" {
		t.Errorf("Expected 'value1', got '%s'", string(data))
	}
}

func TestMemoryBackend_GetNotFound(t *testing.T) {
	mb := NewMemoryBackend()
	ctx := context.Background()

	_, err := mb.Get(ctx, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestMemoryBackend_Delete(t *testing.T) {
	mb := NewMemoryBackend()
	ctx := context.Background()

	mb.Set(ctx, "key1", []byte("value1"))
	err := mb.Delete(ctx, "key1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = mb.Get(ctx, "key1")
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound after delete, got %v", err)
	}
}

func TestMemoryBackend_List(t *testing.T) {
	mb := NewMemoryBackend()
	ctx := context.Background()

	mb.Set(ctx, "prefix:key1", []byte("v1"))
	mb.Set(ctx, "prefix:key2", []byte("v2"))
	mb.Set(ctx, "other:key3", []byte("v3"))

	keys, err := mb.List(ctx, "prefix:")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(keys))
	}
}

func TestMemoryBackend_ListAll(t *testing.T) {
	mb := NewMemoryBackend()
	ctx := context.Background()

	mb.Set(ctx, "key1", []byte("v1"))
	mb.Set(ctx, "key2", []byte("v2"))

	keys, err := mb.List(ctx, "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(keys))
	}
}

func TestMemoryBackend_Close(t *testing.T) {
	mb := NewMemoryBackend()
	ctx := context.Background()

	mb.Set(ctx, "key1", []byte("value1"))
	err := mb.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	_, err = mb.Get(ctx, "key1")
	if err != ErrClosed {
		t.Errorf("Expected ErrClosed, got %v", err)
	}

	err = mb.Set(ctx, "key2", []byte("value2"))
	if err != ErrClosed {
		t.Errorf("Expected ErrClosed on Set, got %v", err)
	}
}

func TestMemoryBackend_Clear(t *testing.T) {
	mb := NewMemoryBackend()
	ctx := context.Background()

	mb.Set(ctx, "key1", []byte("value1"))
	mb.Clear()

	_, err := mb.Get(ctx, "key1")
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound after clear, got %v", err)
	}
}

func TestMemoryBackend_DataIsolation(t *testing.T) {
	mb := NewMemoryBackend()
	ctx := context.Background()

	original := []byte("original")
	mb.Set(ctx, "key", original)

	// Modify original after set
	original[0] = 'X'

	data, _ := mb.Get(ctx, "key")
	if string(data) != "original" {
		t.Error("Set should copy data, not reference it")
	}

	// Modify returned data
	data[0] = 'Y'
	data2, _ := mb.Get(ctx, "key")
	if string(data2) != "original" {
		t.Error("Get should return copy, not reference")
	}
}

func TestFileBackend_Basic(t *testing.T) {
	dir := t.TempDir()
	fb, err := NewFileBackend(dir)
	if err != nil {
		t.Fatalf("NewFileBackend failed: %v", err)
	}
	defer fb.Close()

	ctx := context.Background()

	err = fb.Set(ctx, "key1", []byte("value1"))
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	data, err := fb.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(data) != "value1" {
		t.Errorf("Expected 'value1', got '%s'", string(data))
	}
}

func TestFileBackend_Persistence(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// Write with first instance
	fb1, _ := NewFileBackend(dir)
	fb1.Set(ctx, "persistent", []byte("data"))
	fb1.Close()

	// Read with second instance
	fb2, _ := NewFileBackend(dir)
	defer fb2.Close()

	data, err := fb2.Get(ctx, "persistent")
	if err != nil {
		t.Fatalf("Data should persist: %v", err)
	}
	if string(data) != "data" {
		t.Errorf("Expected 'data', got '%s'", string(data))
	}
}

func TestFileBackend_GetNotFound(t *testing.T) {
	dir := t.TempDir()
	fb, _ := NewFileBackend(dir)
	defer fb.Close()

	_, err := fb.Get(context.Background(), "nonexistent")
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestFileBackend_Delete(t *testing.T) {
	dir := t.TempDir()
	fb, _ := NewFileBackend(dir)
	defer fb.Close()
	ctx := context.Background()

	fb.Set(ctx, "key1", []byte("value1"))
	err := fb.Delete(ctx, "key1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = fb.Get(ctx, "key1")
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound after delete, got %v", err)
	}
}

func TestFileBackend_List(t *testing.T) {
	dir := t.TempDir()
	fb, _ := NewFileBackend(dir)
	defer fb.Close()
	ctx := context.Background()

	fb.Set(ctx, "tasks:task1", []byte("t1"))
	fb.Set(ctx, "tasks:task2", []byte("t2"))
	fb.Set(ctx, "agents:agent1", []byte("a1"))

	keys, err := fb.List(ctx, "tasks:")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("Expected 2 keys with prefix 'tasks:', got %d", len(keys))
	}
}

func TestFileBackend_NestedKeys(t *testing.T) {
	dir := t.TempDir()
	fb, _ := NewFileBackend(dir)
	defer fb.Close()
	ctx := context.Background()

	err := fb.Set(ctx, "level1:level2:level3", []byte("deep"))
	if err != nil {
		t.Fatalf("Set nested key failed: %v", err)
	}

	data, err := fb.Get(ctx, "level1:level2:level3")
	if err != nil {
		t.Fatalf("Get nested key failed: %v", err)
	}
	if string(data) != "deep" {
		t.Errorf("Expected 'deep', got '%s'", string(data))
	}
}

func TestFileBackend_InvalidKey(t *testing.T) {
	dir := t.TempDir()
	fb, _ := NewFileBackend(dir)
	defer fb.Close()
	ctx := context.Background()

	// Keys with path traversal should be sanitized
	err := fb.Set(ctx, "../escape", []byte("bad"))
	if err != nil {
		t.Fatalf("Set with .. should work (sanitized): %v", err)
	}

	// Verify the key was sanitized (stored without the ..)
	keys, _ := fb.List(ctx, "")
	found := false
	for _, k := range keys {
		if k == "escape" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Key should be sanitized to 'escape'")
	}
}

func TestFileBackend_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	fb, _ := NewFileBackend(dir)
	defer fb.Close()
	ctx := context.Background()

	// Write data
	fb.Set(ctx, "atomic", []byte("test"))

	// Verify no temp files remain
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Error("Temp file should not remain after write")
		}
	}
}

func TestFileBackend_Close(t *testing.T) {
	dir := t.TempDir()
	fb, _ := NewFileBackend(dir)
	ctx := context.Background()

	fb.Set(ctx, "key", []byte("value"))
	fb.Close()

	_, err := fb.Get(ctx, "key")
	if err != ErrClosed {
		t.Errorf("Expected ErrClosed after close, got %v", err)
	}
}

func TestJSONCodec_Basic(t *testing.T) {
	codec := &JSONCodec{}

	type TestData struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	original := TestData{Name: "test", Value: 42}
	data, err := codec.Encode(original)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	var decoded TestData
	err = codec.Decode(data, &decoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.Name != original.Name || decoded.Value != original.Value {
		t.Errorf("Decoded data mismatch: got %+v, want %+v", decoded, original)
	}
}

func TestJSONCodec_Slice(t *testing.T) {
	codec := &JSONCodec{}

	original := []string{"a", "b", "c"}
	data, _ := codec.Encode(original)

	var decoded []string
	codec.Decode(data, &decoded)

	if len(decoded) != len(original) {
		t.Fatalf("Length mismatch: got %d, want %d", len(decoded), len(original))
	}
	for i := range original {
		if decoded[i] != original[i] {
			t.Errorf("Element %d mismatch", i)
		}
	}
}

func TestJSONCodec_Map(t *testing.T) {
	codec := &JSONCodec{}

	original := map[string]int{"a": 1, "b": 2}
	data, _ := codec.Encode(original)

	var decoded map[string]int
	codec.Decode(data, &decoded)

	if len(decoded) != len(original) {
		t.Fatalf("Length mismatch")
	}
	for k, v := range original {
		if decoded[k] != v {
			t.Errorf("Key %s mismatch", k)
		}
	}
}

func TestDefaultCodec(t *testing.T) {
	codec := DefaultCodec()
	if codec == nil {
		t.Fatal("DefaultCodec returned nil")
	}
	if _, ok := codec.(*JSONCodec); !ok {
		t.Error("DefaultCodec should return JSONCodec")
	}
}

func TestManager_Basic(t *testing.T) {
	mb := NewMemoryBackend()
	m := NewManager(mb)
	ctx := context.Background()

	type Data struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	// Test Set
	err := m.Set(ctx, "key1", Data{Name: "test", Value: 42})
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Test Get
	var result Data
	err = m.Get(ctx, "key1", &result)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if result.Name != "test" || result.Value != 42 {
		t.Errorf("Data mismatch: got %+v", result)
	}
}

func TestManager_GetNotFound(t *testing.T) {
	mb := NewMemoryBackend()
	m := NewManager(mb)
	ctx := context.Background()

	var result string
	err := m.Get(ctx, "nonexistent", &result)
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestManager_Delete(t *testing.T) {
	mb := NewMemoryBackend()
	m := NewManager(mb)
	ctx := context.Background()

	m.Set(ctx, "key1", "value1")
	err := m.Delete(ctx, "key1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	var result string
	err = m.Get(ctx, "key1", &result)
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound after delete, got %v", err)
	}
}

func TestManager_List(t *testing.T) {
	mb := NewMemoryBackend()
	m := NewManager(mb)
	ctx := context.Background()

	m.Set(ctx, "tasks:1", "task1")
	m.Set(ctx, "tasks:2", "task2")
	m.Set(ctx, "agents:1", "agent1")

	keys, err := m.List(ctx, "tasks:")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(keys))
	}
}

func TestManager_Exists(t *testing.T) {
	mb := NewMemoryBackend()
	m := NewManager(mb)
	ctx := context.Background()

	m.Set(ctx, "exists", "value")

	exists, err := m.Exists(ctx, "exists")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Key should exist")
	}

	exists, err = m.Exists(ctx, "notexists")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("Key should not exist")
	}
}

func TestManager_GetOrDefault(t *testing.T) {
	mb := NewMemoryBackend()
	m := NewManager(mb)
	ctx := context.Background()

	var result string
	err := m.GetOrDefault(ctx, "missing", &result, "default")
	if err != nil {
		t.Fatalf("GetOrDefault failed: %v", err)
	}
	if result != "default" {
		t.Errorf("Expected 'default', got '%s'", result)
	}

	// Set a value and verify it's returned instead of default
	m.Set(ctx, "existing", "actual")
	err = m.GetOrDefault(ctx, "existing", &result, "default")
	if err != nil {
		t.Fatalf("GetOrDefault failed: %v", err)
	}
	if result != "actual" {
		t.Errorf("Expected 'actual', got '%s'", result)
	}
}

func TestManager_Close(t *testing.T) {
	mb := NewMemoryBackend()
	m := NewManager(mb)

	err := m.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Operations after close should fail
	var result string
	err = m.Get(context.Background(), "key", &result)
	if err != ErrClosed {
		t.Errorf("Expected ErrClosed after close, got %v", err)
	}
}

func TestManager_Backend(t *testing.T) {
	mb := NewMemoryBackend()
	m := NewManager(mb)

	if m.Backend() != mb {
		t.Error("Backend() should return the underlying backend")
	}
}

func TestNewManagerWithCodec(t *testing.T) {
	mb := NewMemoryBackend()
	codec := &JSONCodec{}
	m := NewManagerWithCodec(mb, codec)

	if m == nil {
		t.Fatal("NewManagerWithCodec returned nil")
	}
}
