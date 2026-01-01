package swarm

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"bicycle/state"
)

const (
	taskKeyPrefix = "tasks:"
)

// TaskQueue manages the task queue with persistence
type TaskQueue struct {
	mu       sync.RWMutex
	state    *state.Manager
	projectID string

	// In-memory index for fast lookups
	tasks    map[string]*Task
	byStatus map[TaskStatus][]*Task
}

// NewTaskQueue creates a new task queue
func NewTaskQueue(stateManager *state.Manager, projectID string) *TaskQueue {
	return &TaskQueue{
		state:     stateManager,
		projectID: projectID,
		tasks:     make(map[string]*Task),
		byStatus:  make(map[TaskStatus][]*Task),
	}
}

// Load loads tasks from persistent storage
func (q *TaskQueue) Load(ctx context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	prefix := q.taskKey("")
	keys, err := q.state.List(ctx, prefix)
	if err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	q.tasks = make(map[string]*Task)
	q.byStatus = make(map[TaskStatus][]*Task)

	for _, key := range keys {
		var task Task
		if err := q.state.Get(ctx, key, &task); err != nil {
			continue // Skip corrupted entries
		}
		q.tasks[task.ID] = &task
		q.byStatus[task.Status] = append(q.byStatus[task.Status], &task)
	}

	return nil
}

// Add adds a new task to the queue
func (q *TaskQueue) Add(ctx context.Context, task *Task) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if task.ID == "" {
		return fmt.Errorf("task ID is required")
	}

	if _, exists := q.tasks[task.ID]; exists {
		return fmt.Errorf("task %s already exists", task.ID)
	}

	now := time.Now()
	task.CreatedAt = now
	task.UpdatedAt = now
	task.ProjectID = q.projectID

	if task.Status == "" {
		task.Status = TaskStatusPending
	}
	if task.MaxRetries == 0 {
		task.MaxRetries = 3
	}

	// Persist
	if err := q.state.Set(ctx, q.taskKey(task.ID), task); err != nil {
		return fmt.Errorf("failed to persist task: %w", err)
	}

	// Update in-memory index
	q.tasks[task.ID] = task
	q.byStatus[task.Status] = append(q.byStatus[task.Status], task)

	return nil
}

// Get retrieves a task by ID
func (q *TaskQueue) Get(ctx context.Context, taskID string) (*Task, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	task, exists := q.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task %s not found", taskID)
	}

	// Return a copy
	taskCopy := *task
	return &taskCopy, nil
}

// Update updates an existing task
func (q *TaskQueue) Update(ctx context.Context, task *Task) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	existing, exists := q.tasks[task.ID]
	if !exists {
		return fmt.Errorf("task %s not found", task.ID)
	}

	oldStatus := existing.Status
	task.UpdatedAt = time.Now()

	// Persist
	if err := q.state.Set(ctx, q.taskKey(task.ID), task); err != nil {
		return fmt.Errorf("failed to persist task: %w", err)
	}

	// Update in-memory index
	q.tasks[task.ID] = task

	// Update status index if status changed
	if oldStatus != task.Status {
		q.removeFromStatusIndex(task.ID, oldStatus)
		q.byStatus[task.Status] = append(q.byStatus[task.Status], task)
	}

	return nil
}

// Delete removes a task
func (q *TaskQueue) Delete(ctx context.Context, taskID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	task, exists := q.tasks[taskID]
	if !exists {
		return fmt.Errorf("task %s not found", taskID)
	}

	// Remove from persistence
	if err := q.state.Delete(ctx, q.taskKey(taskID)); err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}

	// Remove from in-memory index
	delete(q.tasks, taskID)
	q.removeFromStatusIndex(taskID, task.Status)

	return nil
}

// Next returns the next available task for assignment
// This includes both pending tasks and tasks needing revision
func (q *TaskQueue) Next(ctx context.Context) (*Task, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// Gather all assignable tasks (pending + revision)
	var available []*Task
	available = append(available, q.byStatus[TaskStatusPending]...)
	available = append(available, q.byStatus[TaskStatusRevision]...)

	if len(available) == 0 {
		return nil, nil // No tasks available
	}

	// Sort by priority (highest first), then revision tasks first (to retry faster), then by creation time
	sort.Slice(available, func(i, j int) bool {
		// Priority first
		if available[i].Priority != available[j].Priority {
			return available[i].Priority > available[j].Priority
		}
		// Revision tasks get priority (they've been waiting longer)
		if (available[i].Status == TaskStatusRevision) != (available[j].Status == TaskStatusRevision) {
			return available[i].Status == TaskStatusRevision
		}
		// Then by creation time
		return available[i].CreatedAt.Before(available[j].CreatedAt)
	})

	// Return a copy
	taskCopy := *available[0]
	return &taskCopy, nil
}

// Assign assigns a task to an agent
func (q *TaskQueue) Assign(ctx context.Context, taskID, agentID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	task, exists := q.tasks[taskID]
	if !exists {
		return fmt.Errorf("task %s not found", taskID)
	}

	if task.Status != TaskStatusPending && task.Status != TaskStatusRevision {
		return fmt.Errorf("task %s is not available for assignment (status: %s)", taskID, task.Status)
	}

	oldStatus := task.Status
	task.Status = TaskStatusAssigned
	task.AssignedTo = agentID
	task.UpdatedAt = time.Now()

	// Persist
	if err := q.state.Set(ctx, q.taskKey(taskID), task); err != nil {
		return fmt.Errorf("failed to persist task: %w", err)
	}

	// Update status index
	q.removeFromStatusIndex(taskID, oldStatus)
	q.byStatus[task.Status] = append(q.byStatus[task.Status], task)

	return nil
}

// Complete marks a task as ready for review
func (q *TaskQueue) Complete(ctx context.Context, taskID string, artifacts []Artifact) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	task, exists := q.tasks[taskID]
	if !exists {
		return fmt.Errorf("task %s not found", taskID)
	}

	oldStatus := task.Status
	task.Status = TaskStatusReview
	task.VerificationStatus = VerificationPending
	task.Artifacts = artifacts
	task.UpdatedAt = time.Now()

	// Persist
	if err := q.state.Set(ctx, q.taskKey(taskID), task); err != nil {
		return fmt.Errorf("failed to persist task: %w", err)
	}

	// Update status index
	q.removeFromStatusIndex(taskID, oldStatus)
	q.byStatus[task.Status] = append(q.byStatus[task.Status], task)

	return nil
}

// Reject rejects a task and puts it back for revision
func (q *TaskQueue) Reject(ctx context.Context, taskID string, feedback string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	task, exists := q.tasks[taskID]
	if !exists {
		return fmt.Errorf("task %s not found", taskID)
	}

	// Record this attempt
	attempt := TaskAttempt{
		AttemptNumber: task.RetryCount + 1,
		AgentID:       task.AssignedTo,
		StartedAt:     task.UpdatedAt,
		CompletedAt:   time.Now(),
		Artifacts:     task.Artifacts,
		Feedback:      feedback,
		Result:        "rejected",
	}
	task.PreviousAttempts = append(task.PreviousAttempts, attempt)
	task.RetryCount++

	oldStatus := task.Status

	if task.RetryCount >= task.MaxRetries {
		task.Status = TaskStatusFailed
		task.VerificationStatus = VerificationFailed
	} else {
		task.Status = TaskStatusRevision
		task.VerificationStatus = VerificationFailed
		task.AssignedTo = "" // Unassign for reassignment
	}

	task.UpdatedAt = time.Now()

	// Persist
	if err := q.state.Set(ctx, q.taskKey(taskID), task); err != nil {
		return fmt.Errorf("failed to persist task: %w", err)
	}

	// Update status index
	q.removeFromStatusIndex(taskID, oldStatus)
	q.byStatus[task.Status] = append(q.byStatus[task.Status], task)

	return nil
}

// Approve approves a task after verification
func (q *TaskQueue) Approve(ctx context.Context, taskID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	task, exists := q.tasks[taskID]
	if !exists {
		return fmt.Errorf("task %s not found", taskID)
	}

	oldStatus := task.Status
	task.Status = TaskStatusCompleted
	task.VerificationStatus = VerificationPassed
	task.UpdatedAt = time.Now()

	// Persist
	if err := q.state.Set(ctx, q.taskKey(taskID), task); err != nil {
		return fmt.Errorf("failed to persist task: %w", err)
	}

	// Update status index
	q.removeFromStatusIndex(taskID, oldStatus)
	q.byStatus[task.Status] = append(q.byStatus[task.Status], task)

	return nil
}

// ListByStatus returns all tasks with the given status
func (q *TaskQueue) ListByStatus(status TaskStatus) []*Task {
	q.mu.RLock()
	defer q.mu.RUnlock()

	tasks := q.byStatus[status]
	result := make([]*Task, len(tasks))
	for i, t := range tasks {
		taskCopy := *t
		result[i] = &taskCopy
	}
	return result
}

// PendingReview returns tasks awaiting review
func (q *TaskQueue) PendingReview() []*Task {
	return q.ListByStatus(TaskStatusReview)
}

// Count returns the total number of tasks
func (q *TaskQueue) Count() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.tasks)
}

// CountByStatus returns task count by status
func (q *TaskQueue) CountByStatus() map[TaskStatus]int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	counts := make(map[TaskStatus]int)
	for status, tasks := range q.byStatus {
		counts[status] = len(tasks)
	}
	return counts
}

// taskKey generates the storage key for a task
func (q *TaskQueue) taskKey(taskID string) string {
	return fmt.Sprintf("%s%s:%s", taskKeyPrefix, q.projectID, taskID)
}

// removeFromStatusIndex removes a task from the status index
func (q *TaskQueue) removeFromStatusIndex(taskID string, status TaskStatus) {
	tasks := q.byStatus[status]
	for i, t := range tasks {
		if t.ID == taskID {
			q.byStatus[status] = append(tasks[:i], tasks[i+1:]...)
			break
		}
	}
}
