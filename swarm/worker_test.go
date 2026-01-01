package swarm

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestWorkerAgent_New(t *testing.T) {
	client := NewMockLLMClient()
	worker := NewWorkerAgent(WorkerConfig{
		ID:     "test-worker",
		Name:   "Test Worker",
		Model:  ModelHaiku,
		Client: client,
	})

	agent := worker.GetAgent()
	if agent.ID != "test-worker" {
		t.Errorf("Expected ID 'test-worker', got '%s'", agent.ID)
	}
	if agent.Role != RoleWorker {
		t.Errorf("Expected role Worker, got %s", agent.Role)
	}
	if agent.Status != AgentStatusIdle {
		t.Errorf("Expected status Idle, got %s", agent.Status)
	}
}

func TestWorkerAgent_StartStop(t *testing.T) {
	client := NewMockLLMClient()
	worker := NewWorkerAgent(WorkerConfig{
		ID:     "test-worker",
		Name:   "Test Worker",
		Model:  ModelHaiku,
		Client: client,
	})

	ctx := context.Background()

	err := worker.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if worker.GetStatus() != AgentStatusIdle {
		t.Errorf("Expected status Idle after start, got %s", worker.GetStatus())
	}

	err = worker.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if worker.GetStatus() != AgentStatusStopped {
		t.Errorf("Expected status Stopped, got %s", worker.GetStatus())
	}
}

func TestWorkerAgent_ExecuteTask(t *testing.T) {
	client := NewMockLLMClient()
	client.DefaultResp = "Implementation complete. All tests pass."

	var completed bool
	var completedArtifacts []Artifact
	var mu sync.Mutex

	worker := NewWorkerAgent(WorkerConfig{
		ID:     "test-worker",
		Name:   "Test Worker",
		Model:  ModelHaiku,
		Client: client,
		OnComplete: func(agentID, taskID string, artifacts []Artifact) {
			mu.Lock()
			completed = true
			completedArtifacts = artifacts
			mu.Unlock()
		},
	})

	ctx := context.Background()
	worker.Start(ctx)
	defer worker.Stop(ctx)

	task := &Task{
		ID:          "task-1",
		Title:       "Test Task",
		Description: "Implement a test function",
	}

	workerCtx := WorkerContext{
		Task: task,
	}

	err := worker.ExecuteTask(ctx, task, workerCtx)
	if err != nil {
		t.Fatalf("ExecuteTask failed: %v", err)
	}

	// Wait for execution
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if !completed {
		mu.Unlock()
		t.Error("Task should be completed")
		return
	}
	if len(completedArtifacts) == 0 {
		mu.Unlock()
		t.Error("Should have artifacts")
		return
	}
	mu.Unlock()

	// Check LLM was called
	if len(client.Calls) == 0 {
		t.Error("LLM should have been called")
	}
}

func TestWorkerAgent_ExecuteTaskBusy(t *testing.T) {
	client := NewMockLLMClient()
	worker := NewWorkerAgent(WorkerConfig{
		ID:     "test-worker",
		Name:   "Test Worker",
		Model:  ModelHaiku,
		Client: client,
	})

	ctx := context.Background()
	worker.Start(ctx)
	defer worker.Stop(ctx)

	task1 := &Task{ID: "task-1", Title: "Task 1"}
	task2 := &Task{ID: "task-2", Title: "Task 2"}

	worker.ExecuteTask(ctx, task1, WorkerContext{Task: task1})

	// Try to execute while busy
	err := worker.ExecuteTask(ctx, task2, WorkerContext{Task: task2})
	if err == nil {
		t.Error("Should error when worker is busy")
	}
}

func TestWorkerAgent_Cancel(t *testing.T) {
	client := NewMockLLMClient()
	worker := NewWorkerAgent(WorkerConfig{
		ID:     "test-worker",
		Name:   "Test Worker",
		Model:  ModelHaiku,
		Client: client,
	})

	ctx := context.Background()
	worker.Start(ctx)
	defer worker.Stop(ctx)

	task := &Task{ID: "task-1", Title: "Task 1"}
	worker.ExecuteTask(ctx, task, WorkerContext{Task: task})

	// Cancel should not panic
	worker.Cancel()
}

func TestWorkerAgent_Heartbeat(t *testing.T) {
	client := NewMockLLMClient()
	worker := NewWorkerAgent(WorkerConfig{
		ID:     "test-worker",
		Name:   "Test Worker",
		Model:  ModelHaiku,
		Client: client,
	})

	ctx := context.Background()
	worker.Start(ctx)
	defer worker.Stop(ctx)

	oldHeartbeat := worker.GetAgent().LastHeartbeat
	time.Sleep(1 * time.Millisecond)

	worker.Heartbeat()

	newHeartbeat := worker.GetAgent().LastHeartbeat
	if !newHeartbeat.After(oldHeartbeat) {
		t.Error("Heartbeat should update LastHeartbeat")
	}
}

func TestWorkerAgent_IsIdle(t *testing.T) {
	client := NewMockLLMClient()
	worker := NewWorkerAgent(WorkerConfig{
		ID:     "test-worker",
		Name:   "Test Worker",
		Model:  ModelHaiku,
		Client: client,
	})

	ctx := context.Background()
	worker.Start(ctx)
	defer worker.Stop(ctx)

	if !worker.IsIdle() {
		t.Error("Worker should be idle after start")
	}

	task := &Task{ID: "task-1", Title: "Task 1"}
	worker.ExecuteTask(ctx, task, WorkerContext{Task: task})

	if worker.IsIdle() {
		t.Error("Worker should not be idle during execution")
	}
}

func TestWorkerAgent_ProgressCallback(t *testing.T) {
	client := NewMockLLMClient()

	var progressMessages []string
	var mu sync.Mutex

	worker := NewWorkerAgent(WorkerConfig{
		ID:     "test-worker",
		Name:   "Test Worker",
		Model:  ModelHaiku,
		Client: client,
		OnProgress: func(agentID, taskID, message string) {
			mu.Lock()
			progressMessages = append(progressMessages, message)
			mu.Unlock()
		},
	})

	ctx := context.Background()
	worker.Start(ctx)
	defer worker.Stop(ctx)

	task := &Task{ID: "task-1", Title: "Task 1"}
	worker.ExecuteTask(ctx, task, WorkerContext{Task: task})

	// Wait for execution
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if len(progressMessages) == 0 {
		mu.Unlock()
		t.Error("Should have received progress messages")
		return
	}
	mu.Unlock()
}
