package swarm

import (
	"context"
	"fmt"
	"testing"
	"time"

	"bicycle/state"
)

// TestIntegration_FullTaskLifecycle tests the complete task flow
func TestIntegration_FullTaskLifecycle(t *testing.T) {
	ctx := context.Background()

	// Setup mock client
	client := NewMockLLMClient()
	client.DefaultResp = "Implementation complete with all acceptance criteria met."

	// Create state manager
	backend := state.NewMemoryBackend()
	stateManager := state.NewManager(backend)

	project := &Project{
		ID:                "test-project",
		Name:              "Integration Test",
		Status:            ProjectStatusActive,
		WorkerModel:       "haiku",
		OrchestratorModel: "opus",
		VerificationPolicy: VerificationPolicy{
			BatchSize:  1,
			MaxRetries: 2,
		},
	}

	// Use auto-verifier for predictable results
	verifier := &AutoVerifier{AlwaysApprove: true, Feedback: "Looks good"}

	spawner := NewAgentSpawner(SpawnerConfig{
		Client:       client,
		DefaultModel: ModelHaiku,
	})

	orch := NewOrchestrator(stateManager, nil, project, verifier, spawner, OrchestratorConfig{
		VerificationInterval: 100 * time.Millisecond,
		AssignmentInterval:   50 * time.Millisecond,
		HealthCheckInterval:  1 * time.Second,
	})

	// Wire spawner to orchestrator
	spawner = NewAgentSpawner(SpawnerConfig{
		Client:       client,
		DefaultModel: ModelHaiku,
		Orchestrator: orch,
	})
	orch.SetSpawner(spawner)

	// Start orchestrator
	if err := orch.Start(ctx); err != nil {
		t.Fatalf("Failed to start orchestrator: %v", err)
	}
	defer orch.Stop(ctx)

	// Spawn workers
	agents, err := spawner.SpawnN(ctx, 2, AgentConfig{})
	if err != nil {
		t.Fatalf("Failed to spawn workers: %v", err)
	}

	// Register agents
	for _, agent := range agents {
		orch.GetRegistry().Register(ctx, agent)
	}

	// Add task
	task := &Task{
		ID:          "integration-task-1",
		Type:        "implement",
		Title:       "Test Task",
		Description: "A task for integration testing",
		Priority:    PriorityHigh,
		AcceptanceCriteria: []string{
			"Code compiles",
			"Tests pass",
		},
	}

	if err := orch.AddTask(ctx, task); err != nil {
		t.Fatalf("Failed to add task: %v", err)
	}

	// Wait for task to complete
	deadline := time.Now().Add(5 * time.Second)
	var finalTask *Task
	for time.Now().Before(deadline) {
		queue := orch.GetQueue()
		tasks := queue.ListByStatus(TaskStatusCompleted)
		if len(tasks) > 0 {
			finalTask = tasks[0]
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if finalTask == nil {
		// Check what status the task is in
		queue := orch.GetQueue()
		counts := queue.CountByStatus()
		t.Fatalf("Task did not complete. Status counts: %+v", counts)
	}

	if finalTask.Status != TaskStatusCompleted {
		t.Errorf("Expected completed status, got %s", finalTask.Status)
	}
}

// TestIntegration_TaskRejectionRetry tests rejection and retry flow
func TestIntegration_TaskRejectionRetry(t *testing.T) {
	ctx := context.Background()

	client := NewMockLLMClient()
	client.DefaultResp = "Implementation complete"

	backend := state.NewMemoryBackend()
	stateManager := state.NewManager(backend)

	project := &Project{
		ID:                "retry-project",
		Name:              "Retry Test",
		Status:            ProjectStatusActive,
		WorkerModel:       "haiku",
		OrchestratorModel: "opus",
		VerificationPolicy: VerificationPolicy{
			BatchSize:  1,
			MaxRetries: 3,
		},
	}

	// First reject, then approve
	rejectCount := 0
	verifier := &testVerifier{
		verifyFunc: func(ctx context.Context, task *Task) (bool, string, error) {
			rejectCount++
			if rejectCount == 1 {
				return false, "Missing test coverage", nil
			}
			return true, "All criteria met", nil
		},
	}

	spawner := NewAgentSpawner(SpawnerConfig{
		Client:       client,
		DefaultModel: ModelHaiku,
	})

	orch := NewOrchestrator(stateManager, nil, project, verifier, spawner, OrchestratorConfig{
		VerificationInterval: 100 * time.Millisecond,
		AssignmentInterval:   50 * time.Millisecond,
		HealthCheckInterval:  1 * time.Second,
	})

	spawner = NewAgentSpawner(SpawnerConfig{
		Client:       client,
		DefaultModel: ModelHaiku,
		Orchestrator: orch,
	})
	orch.SetSpawner(spawner)

	if err := orch.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer orch.Stop(ctx)

	// Spawn worker
	agents, _ := spawner.SpawnN(ctx, 1, AgentConfig{})
	for _, agent := range agents {
		orch.GetRegistry().Register(ctx, agent)
	}

	// Add task
	task := &Task{
		ID:          "retry-task",
		Type:        "implement",
		Title:       "Retry Test Task",
		Description: "Should be rejected then approved",
		Priority:    PriorityNormal,
		MaxRetries:  3,
	}

	if err := orch.AddTask(ctx, task); err != nil {
		t.Fatalf("Failed to add task: %v", err)
	}

	// Wait for completion
	deadline := time.Now().Add(5 * time.Second)
	var finalTask *Task
	for time.Now().Before(deadline) {
		queue := orch.GetQueue()
		tasks := queue.ListByStatus(TaskStatusCompleted)
		if len(tasks) > 0 {
			finalTask = tasks[0]
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if finalTask == nil {
		t.Fatal("Task did not complete")
	}

	if finalTask.RetryCount < 1 {
		t.Errorf("Expected at least 1 retry, got %d", finalTask.RetryCount)
	}
}

// TestIntegration_MaxRetriesExhausted tests failure after max retries
func TestIntegration_MaxRetriesExhausted(t *testing.T) {
	ctx := context.Background()

	client := NewMockLLMClient()
	client.DefaultResp = "Always incomplete work."

	backend := state.NewMemoryBackend()
	stateManager := state.NewManager(backend)

	project := &Project{
		ID:                "fail-project",
		Name:              "Failure Test",
		Status:            ProjectStatusActive,
		WorkerModel:       "haiku",
		OrchestratorModel: "opus",
		VerificationPolicy: VerificationPolicy{
			BatchSize:  1,
			MaxRetries: 2,
		},
	}

	// Always reject
	verifier := &AutoVerifier{AlwaysApprove: false, Feedback: "Needs more work"}

	spawner := NewAgentSpawner(SpawnerConfig{
		Client:       client,
		DefaultModel: ModelHaiku,
	})

	orch := NewOrchestrator(stateManager, nil, project, verifier, spawner, OrchestratorConfig{
		VerificationInterval: 100 * time.Millisecond,
		AssignmentInterval:   50 * time.Millisecond,
		HealthCheckInterval:  1 * time.Second,
	})

	spawner = NewAgentSpawner(SpawnerConfig{
		Client:       client,
		DefaultModel: ModelHaiku,
		Orchestrator: orch,
	})
	orch.SetSpawner(spawner)

	if err := orch.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer orch.Stop(ctx)

	agents, _ := spawner.SpawnN(ctx, 1, AgentConfig{})
	for _, agent := range agents {
		orch.GetRegistry().Register(ctx, agent)
	}

	task := &Task{
		ID:          "fail-task",
		Type:        "implement",
		Title:       "Will Fail Task",
		Description: "This task will exhaust retries",
		Priority:    PriorityNormal,
		MaxRetries:  2,
	}

	if err := orch.AddTask(ctx, task); err != nil {
		t.Fatalf("Failed to add task: %v", err)
	}

	// Wait for failure
	deadline := time.Now().Add(5 * time.Second)
	var finalTask *Task
	for time.Now().Before(deadline) {
		queue := orch.GetQueue()
		tasks := queue.ListByStatus(TaskStatusFailed)
		if len(tasks) > 0 {
			finalTask = tasks[0]
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if finalTask == nil {
		queue := orch.GetQueue()
		counts := queue.CountByStatus()
		t.Fatalf("Task did not fail. Status counts: %+v", counts)
	}

	if finalTask.Status != TaskStatusFailed {
		t.Errorf("Expected failed status, got %s", finalTask.Status)
	}

	if finalTask.RetryCount < 2 {
		t.Errorf("Expected 2 retries before failure, got %d", finalTask.RetryCount)
	}
}

// TestIntegration_MultipleTasks tests parallel task processing
func TestIntegration_MultipleTasks(t *testing.T) {
	ctx := context.Background()

	client := NewMockLLMClient()
	client.DefaultResp = "Task completed successfully."

	backend := state.NewMemoryBackend()
	stateManager := state.NewManager(backend)

	project := &Project{
		ID:                "multi-project",
		Name:              "Multi Task Test",
		Status:            ProjectStatusActive,
		WorkerModel:       "haiku",
		OrchestratorModel: "opus",
		VerificationPolicy: VerificationPolicy{
			BatchSize:  5,
			MaxRetries: 1,
		},
	}

	verifier := &AutoVerifier{AlwaysApprove: true, Feedback: "All good"}

	spawner := NewAgentSpawner(SpawnerConfig{
		Client:       client,
		DefaultModel: ModelHaiku,
	})

	orch := NewOrchestrator(stateManager, nil, project, verifier, spawner, OrchestratorConfig{
		VerificationInterval: 100 * time.Millisecond,
		AssignmentInterval:   50 * time.Millisecond,
		HealthCheckInterval:  1 * time.Second,
	})

	spawner = NewAgentSpawner(SpawnerConfig{
		Client:       client,
		DefaultModel: ModelHaiku,
		Orchestrator: orch,
	})
	orch.SetSpawner(spawner)

	if err := orch.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer orch.Stop(ctx)

	// Spawn multiple workers
	agents, _ := spawner.SpawnN(ctx, 3, AgentConfig{})
	for _, agent := range agents {
		orch.GetRegistry().Register(ctx, agent)
	}

	// Add multiple tasks
	taskCount := 5
	for i := 0; i < taskCount; i++ {
		task := &Task{
			ID:          fmt.Sprintf("multi-task-%d", i),
			Type:        "implement",
			Title:       fmt.Sprintf("Task %d", i),
			Description: fmt.Sprintf("Task number %d", i),
			Priority:    PriorityNormal,
		}
		if err := orch.AddTask(ctx, task); err != nil {
			t.Fatalf("Failed to add task %d: %v", i, err)
		}
	}

	// Wait for all tasks to complete
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		queue := orch.GetQueue()
		counts := queue.CountByStatus()
		completed := counts[TaskStatusCompleted]
		if completed >= taskCount {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	queue := orch.GetQueue()
	counts := queue.CountByStatus()
	if counts[TaskStatusCompleted] < taskCount {
		t.Errorf("Expected %d completed tasks, got %d. Counts: %+v",
			taskCount, counts[TaskStatusCompleted], counts)
	}
}

// TestIntegration_ChatHistory tests chat message recording
func TestIntegration_ChatHistory(t *testing.T) {
	ctx := context.Background()

	client := NewMockLLMClient()
	client.DefaultResp = "Working on it!"

	backend := state.NewMemoryBackend()
	stateManager := state.NewManager(backend)

	project := &Project{
		ID:                "chat-project",
		Name:              "Chat Test",
		Status:            ProjectStatusActive,
		WorkerModel:       "haiku",
		OrchestratorModel: "opus",
	}

	verifier := &AutoVerifier{AlwaysApprove: true, Feedback: "LGTM"}

	spawner := NewAgentSpawner(SpawnerConfig{
		Client:       client,
		DefaultModel: ModelHaiku,
	})

	orch := NewOrchestrator(stateManager, nil, project, verifier, spawner, OrchestratorConfig{
		VerificationInterval: 100 * time.Millisecond,
		AssignmentInterval:   50 * time.Millisecond,
		HealthCheckInterval:  1 * time.Second,
	})

	spawner = NewAgentSpawner(SpawnerConfig{
		Client:       client,
		DefaultModel: ModelHaiku,
		Orchestrator: orch,
	})
	orch.SetSpawner(spawner)

	if err := orch.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer orch.Stop(ctx)

	agents, _ := spawner.SpawnN(ctx, 1, AgentConfig{})
	for _, agent := range agents {
		orch.GetRegistry().Register(ctx, agent)
	}

	task := &Task{
		ID:          "chat-task",
		Type:        "implement",
		Title:       "Chat Test Task",
		Description: "Task to test chat recording",
		Priority:    PriorityNormal,
	}

	if err := orch.AddTask(ctx, task); err != nil {
		t.Fatalf("Failed to add task: %v", err)
	}

	// Wait for completion
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		queue := orch.GetQueue()
		if len(queue.ListByStatus(TaskStatusCompleted)) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Check chat history
	chat := orch.GetChatHistory(100)
	if len(chat) == 0 {
		t.Error("Expected chat messages to be recorded")
	}

	// Should have at least assignment and completion messages
	hasAssignment := false
	hasCompletion := false
	for _, msg := range chat {
		if msg.Type == "assignment" {
			hasAssignment = true
		}
		if msg.Type == "completion" || msg.Type == "approved" {
			hasCompletion = true
		}
	}

	if !hasAssignment {
		t.Error("Missing assignment message in chat")
	}
	if !hasCompletion {
		t.Error("Missing completion/approval message in chat")
	}
}

// testVerifier is a configurable verifier for testing
type testVerifier struct {
	verifyFunc func(ctx context.Context, task *Task) (bool, string, error)
}

func (v *testVerifier) Verify(ctx context.Context, task *Task) (approved bool, feedback string, err error) {
	return v.verifyFunc(ctx, task)
}
