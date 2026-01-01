package swarm

import (
	"context"
	"testing"
	"time"

	"bicycle/state"
)

func newTestRegistry(t *testing.T) *AgentRegistry {
	mb := state.NewMemoryBackend()
	sm := state.NewManager(mb)
	return NewAgentRegistry(sm, "test-project")
}

func TestAgentRegistry_Register(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	agent := &Agent{
		ID:    "agent-1",
		Name:  "Worker 1",
		Role:  RoleWorker,
		Model: "haiku",
	}

	err := r.Register(ctx, agent)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	got, err := r.Get(ctx, "agent-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.Name != "Worker 1" {
		t.Errorf("Name mismatch: got %s", got.Name)
	}
	if got.Status != AgentStatusIdle {
		t.Errorf("Expected idle status, got %s", got.Status)
	}
	if got.Role != RoleWorker {
		t.Errorf("Role mismatch: got %s", got.Role)
	}
}

func TestAgentRegistry_RegisterDuplicate(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	agent := &Agent{ID: "agent-1", Name: "Worker"}
	r.Register(ctx, agent)

	err := r.Register(ctx, agent)
	if err == nil {
		t.Error("Duplicate registration should error")
	}
}

func TestAgentRegistry_GetNotFound(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	_, err := r.Get(ctx, "nonexistent")
	if err == nil {
		t.Error("Get nonexistent should error")
	}
}

func TestAgentRegistry_Update(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	agent := &Agent{ID: "agent-1", Name: "Original"}
	r.Register(ctx, agent)

	agent.Name = "Updated"
	err := r.Update(ctx, agent)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, _ := r.Get(ctx, "agent-1")
	if got.Name != "Updated" {
		t.Errorf("Update didn't persist: got %s", got.Name)
	}
}

func TestAgentRegistry_Heartbeat(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	agent := &Agent{ID: "agent-1", Name: "Worker"}
	r.Register(ctx, agent)

	oldHeartbeat := agent.LastHeartbeat
	time.Sleep(1 * time.Millisecond)

	err := r.Heartbeat(ctx, "agent-1")
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	got, _ := r.Get(ctx, "agent-1")
	if !got.LastHeartbeat.After(oldHeartbeat) {
		t.Error("Heartbeat should update LastHeartbeat")
	}
}

func TestAgentRegistry_SetStatus(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	agent := &Agent{ID: "agent-1", Name: "Worker"}
	r.Register(ctx, agent)

	err := r.SetStatus(ctx, "agent-1", AgentStatusWorking)
	if err != nil {
		t.Fatalf("SetStatus failed: %v", err)
	}

	got, _ := r.Get(ctx, "agent-1")
	if got.Status != AgentStatusWorking {
		t.Errorf("Expected working status, got %s", got.Status)
	}

	// Check status index
	working := r.ListByStatus(AgentStatusWorking)
	if len(working) != 1 {
		t.Errorf("Expected 1 working agent, got %d", len(working))
	}

	idle := r.ListByStatus(AgentStatusIdle)
	if len(idle) != 0 {
		t.Errorf("Expected 0 idle agents, got %d", len(idle))
	}
}

func TestAgentRegistry_AssignTask(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	agent := &Agent{ID: "agent-1", Name: "Worker", Role: RoleWorker}
	r.Register(ctx, agent)

	err := r.AssignTask(ctx, "agent-1", "task-1")
	if err != nil {
		t.Fatalf("AssignTask failed: %v", err)
	}

	got, _ := r.Get(ctx, "agent-1")
	if got.Status != AgentStatusWorking {
		t.Errorf("Expected working status, got %s", got.Status)
	}
	if got.CurrentTaskID != "task-1" {
		t.Errorf("CurrentTaskID mismatch: got %s", got.CurrentTaskID)
	}
}

func TestAgentRegistry_AssignTaskNotIdle(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	agent := &Agent{ID: "agent-1", Name: "Worker"}
	r.Register(ctx, agent)
	r.AssignTask(ctx, "agent-1", "task-1")

	// Try to assign another task
	err := r.AssignTask(ctx, "agent-1", "task-2")
	if err == nil {
		t.Error("Assigning to non-idle agent should error")
	}
}

func TestAgentRegistry_CompleteTask(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	agent := &Agent{ID: "agent-1", Name: "Worker"}
	r.Register(ctx, agent)
	r.AssignTask(ctx, "agent-1", "task-1")

	err := r.CompleteTask(ctx, "agent-1", true)
	if err != nil {
		t.Fatalf("CompleteTask failed: %v", err)
	}

	got, _ := r.Get(ctx, "agent-1")
	if got.Status != AgentStatusIdle {
		t.Errorf("Expected idle status, got %s", got.Status)
	}
	if got.CurrentTaskID != "" {
		t.Error("CurrentTaskID should be cleared")
	}
	if got.TasksCompleted != 1 {
		t.Errorf("TasksCompleted should be 1, got %d", got.TasksCompleted)
	}
	if got.SuccessRate != 1.0 {
		t.Errorf("SuccessRate should be 1.0, got %f", got.SuccessRate)
	}
}

func TestAgentRegistry_CompleteTaskFailed(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	agent := &Agent{ID: "agent-1", Name: "Worker"}
	r.Register(ctx, agent)
	r.AssignTask(ctx, "agent-1", "task-1")

	r.CompleteTask(ctx, "agent-1", false)

	got, _ := r.Get(ctx, "agent-1")
	if got.TasksFailed != 1 {
		t.Errorf("TasksFailed should be 1, got %d", got.TasksFailed)
	}
	if got.SuccessRate != 0.0 {
		t.Errorf("SuccessRate should be 0.0, got %f", got.SuccessRate)
	}
}

func TestAgentRegistry_Unregister(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	agent := &Agent{ID: "agent-1", Name: "Worker"}
	r.Register(ctx, agent)

	err := r.Unregister(ctx, "agent-1")
	if err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}

	_, err = r.Get(ctx, "agent-1")
	if err == nil {
		t.Error("Agent should be unregistered")
	}
}

func TestAgentRegistry_GetIdleWorker(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	// No workers
	worker, err := r.GetIdleWorker(ctx)
	if err != nil {
		t.Fatalf("GetIdleWorker failed: %v", err)
	}
	if worker != nil {
		t.Error("Should return nil when no workers")
	}

	// Add workers
	r.Register(ctx, &Agent{ID: "agent-1", Name: "Worker 1", Role: RoleWorker})
	r.Register(ctx, &Agent{ID: "agent-2", Name: "Worker 2", Role: RoleWorker})
	r.AssignTask(ctx, "agent-1", "task-1") // Make agent-1 busy

	worker, _ = r.GetIdleWorker(ctx)
	if worker == nil {
		t.Fatal("Should return idle worker")
	}
	if worker.ID != "agent-2" {
		t.Errorf("Expected agent-2, got %s", worker.ID)
	}
}

func TestAgentRegistry_GetOrchestrator(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	// No orchestrator
	orch, err := r.GetOrchestrator(ctx)
	if err != nil {
		t.Fatalf("GetOrchestrator failed: %v", err)
	}
	if orch != nil {
		t.Error("Should return nil when no orchestrator")
	}

	// Add orchestrator
	r.Register(ctx, &Agent{ID: "orch-1", Name: "Orchestrator", Role: RoleOrchestrator})

	orch, _ = r.GetOrchestrator(ctx)
	if orch == nil {
		t.Fatal("Should return orchestrator")
	}
	if orch.ID != "orch-1" {
		t.Errorf("Expected orch-1, got %s", orch.ID)
	}
}

func TestAgentRegistry_ListByRole(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	r.Register(ctx, &Agent{ID: "w1", Name: "Worker 1", Role: RoleWorker})
	r.Register(ctx, &Agent{ID: "w2", Name: "Worker 2", Role: RoleWorker})
	r.Register(ctx, &Agent{ID: "o1", Name: "Orchestrator", Role: RoleOrchestrator})

	workers := r.ListByRole(RoleWorker)
	if len(workers) != 2 {
		t.Errorf("Expected 2 workers, got %d", len(workers))
	}

	orchestrators := r.ListByRole(RoleOrchestrator)
	if len(orchestrators) != 1 {
		t.Errorf("Expected 1 orchestrator, got %d", len(orchestrators))
	}
}

func TestAgentRegistry_FindStaleAgents(t *testing.T) {
	r := newTestRegistry(t)
	r.staleThreshold = 10 * time.Millisecond
	ctx := context.Background()

	r.Register(ctx, &Agent{ID: "agent-1", Name: "Worker"})

	// Initially not stale
	stale := r.FindStaleAgents()
	if len(stale) != 0 {
		t.Error("Fresh agent should not be stale")
	}

	// Wait and check again
	time.Sleep(20 * time.Millisecond)

	stale = r.FindStaleAgents()
	if len(stale) != 1 {
		t.Errorf("Expected 1 stale agent, got %d", len(stale))
	}
}

func TestAgentRegistry_Count(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	if r.Count() != 0 {
		t.Error("Empty registry should have 0 count")
	}

	r.Register(ctx, &Agent{ID: "agent-1", Name: "Worker 1"})
	r.Register(ctx, &Agent{ID: "agent-2", Name: "Worker 2"})

	if r.Count() != 2 {
		t.Errorf("Expected 2, got %d", r.Count())
	}
}

func TestAgentRegistry_Persistence(t *testing.T) {
	mb := state.NewMemoryBackend()
	sm := state.NewManager(mb)
	ctx := context.Background()

	// Create registry and register agent
	r1 := NewAgentRegistry(sm, "test-project")
	r1.Register(ctx, &Agent{ID: "agent-1", Name: "Persistent"})

	// Create new registry instance and load
	r2 := NewAgentRegistry(sm, "test-project")
	err := r2.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	agent, err := r2.Get(ctx, "agent-1")
	if err != nil {
		t.Fatalf("Agent should persist: %v", err)
	}
	if agent.Name != "Persistent" {
		t.Error("Agent data should persist")
	}
}

func TestAgentRegistry_DefaultConfig(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	agent := &Agent{ID: "agent-1", Name: "Worker"}
	r.Register(ctx, agent)

	got, _ := r.Get(ctx, "agent-1")

	if got.Config.MaxConcurrentTasks != 1 {
		t.Errorf("Default MaxConcurrentTasks should be 1, got %d", got.Config.MaxConcurrentTasks)
	}
	if got.Config.TaskTimeout != 30*time.Minute {
		t.Errorf("Default TaskTimeout should be 30m, got %v", got.Config.TaskTimeout)
	}
	if got.Config.HeartbeatInterval != 30*time.Second {
		t.Errorf("Default HeartbeatInterval should be 30s, got %v", got.Config.HeartbeatInterval)
	}
}
