package daemon

import (
	"context"
	"sync"
	"testing"
	"time"

	"bicycle/internal/config"
	"bicycle/plugin"
)

// mockPlugin implements plugin.Plugin for testing
type mockPlugin struct {
	name              string
	requirementsError error
	startError        error
	stopError         error
	extensions        []plugin.Extension
	started           bool
	stopped           bool
	mu                sync.Mutex
}

func (m *mockPlugin) Name() string { return m.name }

func (m *mockPlugin) CheckRequirements(ctx context.Context) error {
	return m.requirementsError
}

func (m *mockPlugin) Extensions() []plugin.Extension {
	return m.extensions
}

func (m *mockPlugin) Start(ctx context.Context, broker plugin.MessageBroker) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.startError != nil {
		return m.startError
	}
	m.started = true
	return nil
}

func (m *mockPlugin) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	return m.stopError
}

func (m *mockPlugin) isStarted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.started
}

func (m *mockPlugin) isStopped() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopped
}

// mockExecutor implements plugin.Executor for testing
type mockExecutor struct {
	executeErr    error
	cancelErr     error
	executed      bool
	cancelled     bool
	executionTime time.Duration
	mu            sync.Mutex
}

func (m *mockExecutor) Type() plugin.ExtensionType { return plugin.ExtensionTypeExecutor }
func (m *mockExecutor) Name() string               { return "mock-executor" }
func (m *mockExecutor) SupportsMode(mode plugin.Mode) bool { return true }

func (m *mockExecutor) ExecuteTask(ctx context.Context, task *plugin.Task) error {
	m.mu.Lock()
	m.executed = true
	execTime := m.executionTime
	m.mu.Unlock()

	if execTime > 0 {
		select {
		case <-time.After(execTime):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return m.executeErr
}

func (m *mockExecutor) CancelTask(ctx context.Context, taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancelled = true
	return m.cancelErr
}

func (m *mockExecutor) GetStatus(ctx context.Context) (*plugin.ExecutorStatus, error) {
	return &plugin.ExecutorStatus{
		State:    plugin.ExecutorStateIdle,
		Progress: 0,
		Message:  "idle",
	}, nil
}

func (m *mockExecutor) wasExecuted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.executed
}

func TestDaemon_New(t *testing.T) {
	cfg := config.DefaultConfig()
	d := New(cfg)

	if d == nil {
		t.Fatal("New returned nil")
	}
	if d.GetState() != StateIdle {
		t.Errorf("Expected state Idle, got %s", d.GetState())
	}
	if d.GetConfig() != cfg {
		t.Error("Config mismatch")
	}
	if d.GetBroker() == nil {
		t.Error("Broker should not be nil")
	}
}

func TestDaemon_AddPlugin(t *testing.T) {
	cfg := config.DefaultConfig()
	d := New(cfg)

	mp := &mockPlugin{name: "test-plugin"}
	err := d.AddPlugin(mp)
	if err != nil {
		t.Fatalf("AddPlugin failed: %v", err)
	}

	plugins := d.GetPlugins()
	if len(plugins) != 1 {
		t.Errorf("Expected 1 plugin, got %d", len(plugins))
	}
}

func TestDaemon_AddPluginDuplicate(t *testing.T) {
	cfg := config.DefaultConfig()
	d := New(cfg)

	mp := &mockPlugin{name: "test-plugin"}
	d.AddPlugin(mp)

	err := d.AddPlugin(mp)
	if err == nil {
		t.Error("Adding duplicate plugin should error")
	}
}

func TestDaemon_AddPluginDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Plugins["disabled-plugin"] = config.PluginConfig{Enabled: false}
	d := New(cfg)

	mp := &mockPlugin{name: "disabled-plugin"}
	err := d.AddPlugin(mp)
	if err != nil {
		t.Fatalf("AddPlugin failed: %v", err)
	}

	// Plugin should not be added (it's disabled)
	plugins := d.GetPlugins()
	if len(plugins) != 0 {
		t.Errorf("Disabled plugin should not be added, got %d plugins", len(plugins))
	}
}

func TestDaemon_StartStop(t *testing.T) {
	cfg := config.DefaultConfig()
	d := New(cfg)

	mp := &mockPlugin{name: "test-plugin"}
	d.AddPlugin(mp)

	err := d.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !mp.isStarted() {
		t.Error("Plugin should be started")
	}

	err = d.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if d.GetState() != StateStopped {
		t.Errorf("Expected state Stopped, got %s", d.GetState())
	}
	if !mp.isStopped() {
		t.Error("Plugin should be stopped")
	}
}

func TestDaemon_StartWithFailedRequirements(t *testing.T) {
	cfg := config.DefaultConfig()
	d := New(cfg)

	mp := &mockPlugin{
		name:              "failing-plugin",
		requirementsError: context.DeadlineExceeded,
	}
	d.AddPlugin(mp)

	err := d.Start()
	if err != nil {
		t.Fatalf("Start should succeed even with failed requirements: %v", err)
	}

	// Plugin with failed requirements should be removed
	plugins := d.GetPlugins()
	if len(plugins) != 0 {
		t.Errorf("Plugin with failed requirements should be removed, got %d", len(plugins))
	}

	d.Stop()
}

func TestDaemon_ExecuteTask(t *testing.T) {
	cfg := config.DefaultConfig()
	d := New(cfg)

	executor := &mockExecutor{}
	mp := &mockPlugin{
		name:       "executor-plugin",
		extensions: []plugin.Extension{executor},
	}
	d.AddPlugin(mp)
	d.Start()
	defer d.Stop()

	task := &plugin.Task{
		ID:   "test-task",
		Type: "test",
	}

	err := d.ExecuteTask(context.Background(), task)
	if err != nil {
		t.Fatalf("ExecuteTask failed: %v", err)
	}

	// Wait for execution
	time.Sleep(100 * time.Millisecond)

	if !executor.wasExecuted() {
		t.Error("Executor should have been called")
	}
}

func TestDaemon_ExecuteTaskBusy(t *testing.T) {
	cfg := config.DefaultConfig()
	d := New(cfg)

	executor := &mockExecutor{executionTime: 100 * time.Millisecond}
	mp := &mockPlugin{
		name:       "executor-plugin",
		extensions: []plugin.Extension{executor},
	}
	d.AddPlugin(mp)
	d.Start()

	task1 := &plugin.Task{ID: "task1", Type: "test"}
	task2 := &plugin.Task{ID: "task2", Type: "test"}

	err := d.ExecuteTask(context.Background(), task1)
	if err != nil {
		t.Fatalf("First ExecuteTask failed: %v", err)
	}

	// Wait a moment for state to become Working
	time.Sleep(10 * time.Millisecond)

	// Try to execute another task while busy
	err = d.ExecuteTask(context.Background(), task2)
	if err == nil {
		t.Error("ExecuteTask should fail when daemon is busy")
	}

	// Wait for first task to finish before stopping
	time.Sleep(150 * time.Millisecond)
	d.Stop()
}

func TestDaemon_ExecuteTaskNoExecutor(t *testing.T) {
	cfg := config.DefaultConfig()
	d := New(cfg)

	// No executor plugin
	mp := &mockPlugin{name: "no-executor-plugin"}
	d.AddPlugin(mp)
	d.Start()
	defer d.Stop()

	task := &plugin.Task{ID: "test", Type: "test"}
	err := d.ExecuteTask(context.Background(), task)
	if err == nil {
		t.Error("ExecuteTask should fail without executor")
	}
}

func TestDaemon_Reset(t *testing.T) {
	cfg := config.DefaultConfig()
	d := New(cfg)

	// Use shorter execution time
	executor := &mockExecutor{executionTime: 200 * time.Millisecond}
	mp := &mockPlugin{
		name:       "executor-plugin",
		extensions: []plugin.Extension{executor},
	}
	d.AddPlugin(mp)
	d.Start()

	task := &plugin.Task{ID: "long-task", Type: "test"}
	d.ExecuteTask(context.Background(), task)

	// Wait for state to become Working
	time.Sleep(20 * time.Millisecond)

	if d.GetState() != StateWorking {
		t.Errorf("Expected state Working, got %s", d.GetState())
	}

	err := d.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	if d.GetState() != StateIdle {
		t.Errorf("Expected state Idle after reset, got %s", d.GetState())
	}

	// Wait for task goroutine to complete before stopping
	time.Sleep(250 * time.Millisecond)
	d.Stop()
}

func TestDaemon_ResetNotWorking(t *testing.T) {
	cfg := config.DefaultConfig()
	d := New(cfg)
	d.Start()
	defer d.Stop()

	err := d.Reset(context.Background())
	if err == nil {
		t.Error("Reset should fail when not working")
	}
}

func TestDaemon_GetStatus(t *testing.T) {
	cfg := config.DefaultConfig()
	d := New(cfg)

	mp := &mockPlugin{name: "test-plugin"}
	d.AddPlugin(mp)
	d.Start()
	defer d.Stop()

	status := d.GetStatus(context.Background())
	if status == "" {
		t.Error("GetStatus should return non-empty string")
	}
}

func TestDaemon_SetState(t *testing.T) {
	cfg := config.DefaultConfig()
	d := New(cfg)

	d.SetState(StateWorking)
	if d.GetState() != StateWorking {
		t.Errorf("Expected Working, got %s", d.GetState())
	}

	d.SetState(StateIdle)
	if d.GetState() != StateIdle {
		t.Errorf("Expected Idle, got %s", d.GetState())
	}
}
