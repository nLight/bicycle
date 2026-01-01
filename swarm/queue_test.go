package swarm

import (
	"context"
	"testing"
	"time"

	"bicycle/state"
)

func newTestQueue(t *testing.T) *TaskQueue {
	mb := state.NewMemoryBackend()
	sm := state.NewManager(mb)
	return NewTaskQueue(sm, "test-project")
}

func TestTaskQueue_Add(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	task := &Task{
		ID:    "task-1",
		Title: "Test Task",
		Type:  "implement",
	}

	err := q.Add(ctx, task)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Verify task was added
	got, err := q.Get(ctx, "task-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.Title != "Test Task" {
		t.Errorf("Title mismatch: got %s", got.Title)
	}
	if got.Status != TaskStatusPending {
		t.Errorf("Expected pending status, got %s", got.Status)
	}
	if got.ProjectID != "test-project" {
		t.Errorf("Project ID mismatch: got %s", got.ProjectID)
	}
}

func TestTaskQueue_AddDuplicate(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	task := &Task{ID: "task-1", Title: "Test"}
	q.Add(ctx, task)

	err := q.Add(ctx, task)
	if err == nil {
		t.Error("Adding duplicate should error")
	}
}

func TestTaskQueue_GetNotFound(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	_, err := q.Get(ctx, "nonexistent")
	if err == nil {
		t.Error("Get nonexistent should error")
	}
}

func TestTaskQueue_Update(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	task := &Task{ID: "task-1", Title: "Original"}
	q.Add(ctx, task)

	// Update the task
	task.Title = "Updated"
	err := q.Update(ctx, task)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, _ := q.Get(ctx, "task-1")
	if got.Title != "Updated" {
		t.Errorf("Update didn't persist: got %s", got.Title)
	}
}

func TestTaskQueue_Delete(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	task := &Task{ID: "task-1", Title: "Test"}
	q.Add(ctx, task)

	err := q.Delete(ctx, "task-1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = q.Get(ctx, "task-1")
	if err == nil {
		t.Error("Task should be deleted")
	}
}

func TestTaskQueue_Next(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	// Add tasks with different priorities
	q.Add(ctx, &Task{ID: "low", Title: "Low", Priority: PriorityLow})
	q.Add(ctx, &Task{ID: "high", Title: "High", Priority: PriorityHigh})
	q.Add(ctx, &Task{ID: "normal", Title: "Normal", Priority: PriorityNormal})

	// Should get highest priority first
	next, err := q.Next(ctx)
	if err != nil {
		t.Fatalf("Next failed: %v", err)
	}
	if next.ID != "high" {
		t.Errorf("Expected high priority task, got %s", next.ID)
	}
}

func TestTaskQueue_NextEmpty(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	next, err := q.Next(ctx)
	if err != nil {
		t.Fatalf("Next failed: %v", err)
	}
	if next != nil {
		t.Error("Empty queue should return nil")
	}
}

func TestTaskQueue_Assign(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	q.Add(ctx, &Task{ID: "task-1", Title: "Test"})

	err := q.Assign(ctx, "task-1", "agent-1")
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}

	task, _ := q.Get(ctx, "task-1")
	if task.Status != TaskStatusAssigned {
		t.Errorf("Expected assigned status, got %s", task.Status)
	}
	if task.AssignedTo != "agent-1" {
		t.Errorf("AssignedTo mismatch: got %s", task.AssignedTo)
	}
}

func TestTaskQueue_AssignNonPending(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	q.Add(ctx, &Task{ID: "task-1", Title: "Test"})
	q.Assign(ctx, "task-1", "agent-1")

	// Try to assign again
	err := q.Assign(ctx, "task-1", "agent-2")
	if err == nil {
		t.Error("Assigning non-pending task should error")
	}
}

func TestTaskQueue_Complete(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	q.Add(ctx, &Task{ID: "task-1", Title: "Test"})
	q.Assign(ctx, "task-1", "agent-1")

	artifacts := []Artifact{{ID: "art-1", Type: "code"}}
	err := q.Complete(ctx, "task-1", artifacts)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	task, _ := q.Get(ctx, "task-1")
	if task.Status != TaskStatusReview {
		t.Errorf("Expected review status, got %s", task.Status)
	}
	if len(task.Artifacts) != 1 {
		t.Errorf("Artifacts not stored")
	}
}

func TestTaskQueue_Approve(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	q.Add(ctx, &Task{ID: "task-1", Title: "Test"})
	q.Assign(ctx, "task-1", "agent-1")
	q.Complete(ctx, "task-1", nil)

	err := q.Approve(ctx, "task-1")
	if err != nil {
		t.Fatalf("Approve failed: %v", err)
	}

	task, _ := q.Get(ctx, "task-1")
	if task.Status != TaskStatusCompleted {
		t.Errorf("Expected completed status, got %s", task.Status)
	}
	if task.VerificationStatus != VerificationPassed {
		t.Errorf("Expected verification passed, got %s", task.VerificationStatus)
	}
}

func TestTaskQueue_Reject(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	q.Add(ctx, &Task{ID: "task-1", Title: "Test", MaxRetries: 3})
	q.Assign(ctx, "task-1", "agent-1")
	q.Complete(ctx, "task-1", nil)

	err := q.Reject(ctx, "task-1", "Needs improvement")
	if err != nil {
		t.Fatalf("Reject failed: %v", err)
	}

	task, _ := q.Get(ctx, "task-1")
	if task.Status != TaskStatusRevision {
		t.Errorf("Expected revision status, got %s", task.Status)
	}
	if task.RetryCount != 1 {
		t.Errorf("RetryCount should be 1, got %d", task.RetryCount)
	}
	if len(task.PreviousAttempts) != 1 {
		t.Errorf("Should have 1 previous attempt")
	}
	if task.PreviousAttempts[0].Feedback != "Needs improvement" {
		t.Errorf("Feedback not recorded")
	}
}

func TestTaskQueue_RejectMaxRetries(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	q.Add(ctx, &Task{ID: "task-1", Title: "Test", MaxRetries: 2})

	// First attempt
	q.Assign(ctx, "task-1", "agent-1")
	q.Complete(ctx, "task-1", nil)
	q.Reject(ctx, "task-1", "Try again")

	// Second attempt
	q.Assign(ctx, "task-1", "agent-1")
	q.Complete(ctx, "task-1", nil)
	q.Reject(ctx, "task-1", "Still wrong")

	task, _ := q.Get(ctx, "task-1")
	if task.Status != TaskStatusFailed {
		t.Errorf("Expected failed status after max retries, got %s", task.Status)
	}
}

func TestTaskQueue_ListByStatus(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	q.Add(ctx, &Task{ID: "task-1", Title: "Test 1"})
	q.Add(ctx, &Task{ID: "task-2", Title: "Test 2"})
	q.Assign(ctx, "task-1", "agent-1")

	pending := q.ListByStatus(TaskStatusPending)
	if len(pending) != 1 {
		t.Errorf("Expected 1 pending, got %d", len(pending))
	}

	assigned := q.ListByStatus(TaskStatusAssigned)
	if len(assigned) != 1 {
		t.Errorf("Expected 1 assigned, got %d", len(assigned))
	}
}

func TestTaskQueue_Count(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	if q.Count() != 0 {
		t.Error("Empty queue should have 0 count")
	}

	q.Add(ctx, &Task{ID: "task-1", Title: "Test 1"})
	q.Add(ctx, &Task{ID: "task-2", Title: "Test 2"})

	if q.Count() != 2 {
		t.Errorf("Expected 2, got %d", q.Count())
	}
}

func TestTaskQueue_Persistence(t *testing.T) {
	mb := state.NewMemoryBackend()
	sm := state.NewManager(mb)
	ctx := context.Background()

	// Create queue and add task
	q1 := NewTaskQueue(sm, "test-project")
	q1.Add(ctx, &Task{ID: "task-1", Title: "Persistent"})

	// Create new queue instance and load
	q2 := NewTaskQueue(sm, "test-project")
	err := q2.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	task, err := q2.Get(ctx, "task-1")
	if err != nil {
		t.Fatalf("Task should persist: %v", err)
	}
	if task.Title != "Persistent" {
		t.Error("Task data should persist")
	}
}

func TestTaskQueue_PriorityOrdering(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	// Add tasks in random order
	now := time.Now()
	q.Add(ctx, &Task{ID: "t1", Priority: PriorityNormal})
	q.Add(ctx, &Task{ID: "t2", Priority: PriorityCritical})
	q.Add(ctx, &Task{ID: "t3", Priority: PriorityLow})
	q.Add(ctx, &Task{ID: "t4", Priority: PriorityCritical}) // Same priority, should be FIFO

	// Force t4 to have later time by updating
	time.Sleep(1 * time.Millisecond)
	task4, _ := q.Get(ctx, "t4")
	task4.CreatedAt = now.Add(time.Second)
	q.Update(ctx, task4)

	// Get tasks in priority order
	next, _ := q.Next(ctx)
	if next.ID != "t2" {
		t.Errorf("First should be t2 (critical, earlier), got %s", next.ID)
	}
}
