package swarm

import (
	"context"
	"testing"
	"time"
)

func TestAgentSpawner_Spawn(t *testing.T) {
	client := NewMockLLMClient()
	spawner := NewAgentSpawner(SpawnerConfig{
		Client:       client,
		DefaultModel: ModelHaiku,
	})

	ctx := context.Background()
	agent, err := spawner.Spawn(ctx, AgentConfig{})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	if agent == nil {
		t.Fatal("Agent should not be nil")
	}
	if agent.Role != RoleWorker {
		t.Errorf("Expected Worker role, got %s", agent.Role)
	}

	if spawner.Count() != 1 {
		t.Errorf("Expected 1 worker, got %d", spawner.Count())
	}
}

func TestAgentSpawner_SpawnN(t *testing.T) {
	client := NewMockLLMClient()
	spawner := NewAgentSpawner(SpawnerConfig{
		Client:       client,
		DefaultModel: ModelHaiku,
	})

	ctx := context.Background()
	agents, err := spawner.SpawnN(ctx, 3, AgentConfig{})
	if err != nil {
		t.Fatalf("SpawnN failed: %v", err)
	}

	if len(agents) != 3 {
		t.Errorf("Expected 3 agents, got %d", len(agents))
	}
	if spawner.Count() != 3 {
		t.Errorf("Expected 3 workers, got %d", spawner.Count())
	}
}

func TestAgentSpawner_GetWorker(t *testing.T) {
	client := NewMockLLMClient()
	spawner := NewAgentSpawner(SpawnerConfig{
		Client:       client,
		DefaultModel: ModelHaiku,
	})

	ctx := context.Background()
	agent, _ := spawner.Spawn(ctx, AgentConfig{})

	worker, ok := spawner.GetWorker(agent.ID)
	if !ok {
		t.Fatal("Worker should exist")
	}
	if worker.GetAgent().ID != agent.ID {
		t.Error("Worker ID mismatch")
	}

	_, ok = spawner.GetWorker("nonexistent")
	if ok {
		t.Error("Should not find nonexistent worker")
	}
}

func TestAgentSpawner_GetIdleWorker(t *testing.T) {
	client := NewMockLLMClient()
	spawner := NewAgentSpawner(SpawnerConfig{
		Client:       client,
		DefaultModel: ModelHaiku,
	})

	// No workers yet
	worker := spawner.GetIdleWorker()
	if worker != nil {
		t.Error("Should return nil when no workers")
	}

	ctx := context.Background()
	spawner.Spawn(ctx, AgentConfig{})

	worker = spawner.GetIdleWorker()
	if worker == nil {
		t.Error("Should return idle worker")
	}
	if !worker.IsIdle() {
		t.Error("Worker should be idle")
	}
}

func TestAgentSpawner_Stop(t *testing.T) {
	client := NewMockLLMClient()
	spawner := NewAgentSpawner(SpawnerConfig{
		Client:       client,
		DefaultModel: ModelHaiku,
	})

	ctx := context.Background()
	agent, _ := spawner.Spawn(ctx, AgentConfig{})

	err := spawner.Stop(ctx, agent.ID)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if spawner.Count() != 0 {
		t.Error("Worker should be removed")
	}

	_, ok := spawner.GetWorker(agent.ID)
	if ok {
		t.Error("Worker should not exist after stop")
	}
}

func TestAgentSpawner_StopAll(t *testing.T) {
	client := NewMockLLMClient()
	spawner := NewAgentSpawner(SpawnerConfig{
		Client:       client,
		DefaultModel: ModelHaiku,
	})

	ctx := context.Background()
	spawner.SpawnN(ctx, 3, AgentConfig{})

	err := spawner.StopAll(ctx)
	if err != nil {
		t.Fatalf("StopAll failed: %v", err)
	}

	if spawner.Count() != 0 {
		t.Errorf("All workers should be removed, got %d", spawner.Count())
	}
}

func TestAgentSpawner_Restart(t *testing.T) {
	client := NewMockLLMClient()
	client.DefaultResp = "Task completed"

	spawner := NewAgentSpawner(SpawnerConfig{
		Client:       client,
		DefaultModel: ModelHaiku,
	})

	ctx := context.Background()
	agent, _ := spawner.Spawn(ctx, AgentConfig{})

	task := &Task{
		ID:          "task-1",
		Title:       "Test Task",
		Description: "Test description",
	}

	err := spawner.Restart(ctx, agent.ID, task)
	if err != nil {
		t.Fatalf("Restart failed: %v", err)
	}

	// Just verify the call succeeded - the worker runs async
	// Wait for task to complete
	time.Sleep(100 * time.Millisecond)

	// After completion, worker should be back to idle
	worker, _ := spawner.GetWorker(agent.ID)
	if worker.GetStatus() != AgentStatusIdle {
		t.Errorf("Worker should be idle after task, got %s", worker.GetStatus())
	}
}

func TestAgentSpawner_RestartNotFound(t *testing.T) {
	client := NewMockLLMClient()
	spawner := NewAgentSpawner(SpawnerConfig{
		Client:       client,
		DefaultModel: ModelHaiku,
	})

	ctx := context.Background()
	task := &Task{ID: "task-1"}

	err := spawner.Restart(ctx, "nonexistent", task)
	if err == nil {
		t.Error("Should error for nonexistent worker")
	}
}

func TestAgentSpawner_StopNotFound(t *testing.T) {
	client := NewMockLLMClient()
	spawner := NewAgentSpawner(SpawnerConfig{
		Client:       client,
		DefaultModel: ModelHaiku,
	})

	ctx := context.Background()
	err := spawner.Stop(ctx, "nonexistent")
	if err == nil {
		t.Error("Should error for nonexistent worker")
	}
}
