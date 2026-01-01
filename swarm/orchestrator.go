package swarm

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"bicycle/plugin"
	"bicycle/state"
)

// Verifier is called to verify task completion
type Verifier interface {
	// Verify checks if a task was completed correctly
	// Returns approval status and feedback
	Verify(ctx context.Context, task *Task) (approved bool, feedback string, err error)
}

// WorkerSpawner creates new worker agents
type WorkerSpawner interface {
	// Spawn creates a new worker agent
	Spawn(ctx context.Context, config AgentConfig) (*Agent, error)

	// Restart restarts a worker with new context
	Restart(ctx context.Context, agentID string, task *Task) error

	// Stop stops a worker agent
	Stop(ctx context.Context, agentID string) error
}

// Orchestrator manages the swarm workflow
type Orchestrator struct {
	mu     sync.RWMutex
	state  *state.Manager
	broker plugin.MessageBroker

	projectID string
	project   *Project

	queue    *TaskQueue
	registry *AgentRegistry

	verifier Verifier
	spawner  WorkerSpawner

	// Chat history for context
	chatHistory []ChatMessage

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Configuration
	verificationInterval time.Duration
	assignmentInterval   time.Duration
	healthCheckInterval  time.Duration
}

// OrchestratorConfig holds orchestrator configuration
type OrchestratorConfig struct {
	VerificationInterval time.Duration
	AssignmentInterval   time.Duration
	HealthCheckInterval  time.Duration
}

// DefaultOrchestratorConfig returns default configuration
func DefaultOrchestratorConfig() OrchestratorConfig {
	return OrchestratorConfig{
		VerificationInterval: 10 * time.Second,
		AssignmentInterval:   5 * time.Second,
		HealthCheckInterval:  30 * time.Second,
	}
}

// NewOrchestrator creates a new orchestrator
func NewOrchestrator(
	stateManager *state.Manager,
	broker plugin.MessageBroker,
	project *Project,
	verifier Verifier,
	spawner WorkerSpawner,
	config OrchestratorConfig,
) *Orchestrator {
	ctx, cancel := context.WithCancel(context.Background())

	return &Orchestrator{
		state:                stateManager,
		broker:               broker,
		projectID:            project.ID,
		project:              project,
		queue:                NewTaskQueue(stateManager, project.ID),
		registry:             NewAgentRegistry(stateManager, project.ID),
		verifier:             verifier,
		spawner:              spawner,
		ctx:                  ctx,
		cancel:               cancel,
		verificationInterval: config.VerificationInterval,
		assignmentInterval:   config.AssignmentInterval,
		healthCheckInterval:  config.HealthCheckInterval,
	}
}

// SetSpawner sets the worker spawner (useful for wiring up circular dependencies)
func (o *Orchestrator) SetSpawner(spawner WorkerSpawner) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.spawner = spawner
}

// Start begins the orchestration loops
func (o *Orchestrator) Start(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	log.Printf("[Orchestrator] Starting for project %s", o.projectID)

	// Load existing state
	if err := o.queue.Load(ctx); err != nil {
		log.Printf("[Orchestrator] Warning: failed to load task queue: %v", err)
	}
	if err := o.registry.Load(ctx); err != nil {
		log.Printf("[Orchestrator] Warning: failed to load agent registry: %v", err)
	}

	// Start loops
	o.wg.Add(3)
	go o.assignmentLoop()
	go o.verificationLoop()
	go o.healthCheckLoop()

	log.Printf("[Orchestrator] Started with %d tasks, %d agents",
		o.queue.Count(), o.registry.Count())

	return nil
}

// Stop stops the orchestrator
func (o *Orchestrator) Stop(ctx context.Context) error {
	o.mu.Lock()
	o.cancel()
	o.mu.Unlock()

	// Wait for loops to finish
	done := make(chan struct{})
	go func() {
		o.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("[Orchestrator] Stopped gracefully")
	case <-ctx.Done():
		log.Printf("[Orchestrator] Forced shutdown")
	}

	return nil
}

// AddTask adds a task to the queue
func (o *Orchestrator) AddTask(ctx context.Context, task *Task) error {
	if err := o.queue.Add(ctx, task); err != nil {
		return err
	}

	o.postChat(ctx, ChatMessage{
		AgentID:   "orchestrator",
		AgentName: "Orchestrator",
		Role:      RoleOrchestrator,
		TaskID:    task.ID,
		Type:      "task_added",
		Content:   fmt.Sprintf("New task added: %s - %s", task.ID, task.Title),
	})

	return nil
}

// GetQueue returns the task queue
func (o *Orchestrator) GetQueue() *TaskQueue {
	return o.queue
}

// GetRegistry returns the agent registry
func (o *Orchestrator) GetRegistry() *AgentRegistry {
	return o.registry
}

// GetChatHistory returns recent chat messages
func (o *Orchestrator) GetChatHistory(limit int) []ChatMessage {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if limit <= 0 || limit > len(o.chatHistory) {
		limit = len(o.chatHistory)
	}

	start := len(o.chatHistory) - limit
	result := make([]ChatMessage, limit)
	copy(result, o.chatHistory[start:])
	return result
}

// assignmentLoop assigns pending tasks to idle workers
func (o *Orchestrator) assignmentLoop() {
	defer o.wg.Done()

	ticker := time.NewTicker(o.assignmentInterval)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			o.tryAssignTasks()
		}
	}
}

// tryAssignTasks attempts to assign pending tasks to idle workers
func (o *Orchestrator) tryAssignTasks() {
	ctx := o.ctx

	// Get next pending task
	task, err := o.queue.Next(ctx)
	if err != nil || task == nil {
		return // No tasks or error
	}

	// Get idle worker
	worker, err := o.registry.GetIdleWorker(ctx)
	if err != nil || worker == nil {
		return // No workers available
	}

	// Assign task to worker
	if err := o.queue.Assign(ctx, task.ID, worker.ID); err != nil {
		log.Printf("[Orchestrator] Failed to assign task %s: %v", task.ID, err)
		return
	}

	if err := o.registry.AssignTask(ctx, worker.ID, task.ID); err != nil {
		log.Printf("[Orchestrator] Failed to update agent %s: %v", worker.ID, err)
		return
	}

	// Build context for the worker
	workerCtx := o.buildWorkerContext(task)

	// Spawn/restart worker with task context
	if o.spawner != nil {
		if err := o.spawner.Restart(ctx, worker.ID, task); err != nil {
			log.Printf("[Orchestrator] Failed to start worker %s: %v", worker.ID, err)
		}
	}

	o.postChat(ctx, ChatMessage{
		AgentID:   "orchestrator",
		AgentName: "Orchestrator",
		Role:      RoleOrchestrator,
		TaskID:    task.ID,
		Type:      "assignment",
		Content:   fmt.Sprintf("Assigned task %s to agent %s", task.ID, worker.ID),
	})

	log.Printf("[Orchestrator] Assigned task %s to worker %s (context: %d previous attempts)",
		task.ID, worker.ID, len(workerCtx.PreviousAttempts))
}

// buildWorkerContext builds context for a worker starting a task
func (o *Orchestrator) buildWorkerContext(task *Task) WorkerContext {
	o.mu.RLock()
	defer o.mu.RUnlock()

	// Get relevant chat messages for this task
	var relevantChat []ChatMessage
	for _, msg := range o.chatHistory {
		if msg.TaskID == task.ID {
			relevantChat = append(relevantChat, msg)
		}
	}

	// Get last feedback if any
	var lastFeedback string
	if len(task.PreviousAttempts) > 0 {
		lastFeedback = task.PreviousAttempts[len(task.PreviousAttempts)-1].Feedback
	}

	return WorkerContext{
		Task:             task,
		PreviousAttempts: task.PreviousAttempts,
		LastFeedback:     lastFeedback,
		RelevantChat:     relevantChat,
		RetryCount:       task.RetryCount,
		MaxRetries:       task.MaxRetries,
	}
}

// WorkerContext contains context for a worker executing a task
type WorkerContext struct {
	Task             *Task
	PreviousAttempts []TaskAttempt
	LastFeedback     string
	RelevantChat     []ChatMessage
	RetryCount       int
	MaxRetries       int
}

// verificationLoop checks completed tasks
func (o *Orchestrator) verificationLoop() {
	defer o.wg.Done()

	ticker := time.NewTicker(o.verificationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			o.tryVerifyTasks()
		}
	}
}

// tryVerifyTasks verifies tasks awaiting review
func (o *Orchestrator) tryVerifyTasks() {
	ctx := o.ctx

	if o.verifier == nil {
		return // No verifier configured
	}

	tasks := o.queue.PendingReview()
	policy := o.project.VerificationPolicy

	for _, task := range tasks {
		// Check if we should skip verification
		if o.shouldSkipVerification(task, policy) {
			if err := o.queue.Approve(ctx, task.ID); err != nil {
				log.Printf("[Orchestrator] Failed to auto-approve task %s: %v", task.ID, err)
			}
			continue
		}

		// Verify the task
		approved, feedback, err := o.verifier.Verify(ctx, task)
		if err != nil {
			log.Printf("[Orchestrator] Verification error for task %s: %v", task.ID, err)
			continue
		}

		if approved {
			if err := o.queue.Approve(ctx, task.ID); err != nil {
				log.Printf("[Orchestrator] Failed to approve task %s: %v", task.ID, err)
				continue
			}

			// Update agent stats
			if task.AssignedTo != "" {
				o.registry.CompleteTask(ctx, task.AssignedTo, true)
			}

			o.postChat(ctx, ChatMessage{
				AgentID:   "orchestrator",
				AgentName: "Orchestrator",
				Role:      RoleOrchestrator,
				TaskID:    task.ID,
				Type:      "approval",
				Content:   fmt.Sprintf("Task %s approved: %s", task.ID, feedback),
			})

			log.Printf("[Orchestrator] Approved task %s", task.ID)
		} else {
			if err := o.queue.Reject(ctx, task.ID, feedback); err != nil {
				log.Printf("[Orchestrator] Failed to reject task %s: %v", task.ID, err)
				continue
			}

			// Get updated task to check if it failed
			updatedTask, _ := o.queue.Get(ctx, task.ID)

			if updatedTask != nil && updatedTask.Status == TaskStatusFailed {
				// Update agent stats for final failure
				if task.AssignedTo != "" {
					o.registry.CompleteTask(ctx, task.AssignedTo, false)
				}

				o.postChat(ctx, ChatMessage{
					AgentID:   "orchestrator",
					AgentName: "Orchestrator",
					Role:      RoleOrchestrator,
					TaskID:    task.ID,
					Type:      "failure",
					Content:   fmt.Sprintf("Task %s failed after %d attempts: %s", task.ID, task.RetryCount, feedback),
				})

				log.Printf("[Orchestrator] Task %s failed permanently", task.ID)
			} else {
				// Free up the agent so it can pick up the revision
				if task.AssignedTo != "" {
					o.registry.SetStatus(ctx, task.AssignedTo, AgentStatusIdle)
				}

				o.postChat(ctx, ChatMessage{
					AgentID:   "orchestrator",
					AgentName: "Orchestrator",
					Role:      RoleOrchestrator,
					TaskID:    task.ID,
					Type:      "correction",
					Content:   fmt.Sprintf("Task %s needs revision (attempt %d/%d): %s", task.ID, task.RetryCount, task.MaxRetries, feedback),
				})

				log.Printf("[Orchestrator] Rejected task %s for revision (attempt %d/%d)",
					task.ID, task.RetryCount, task.MaxRetries)
			}
		}
	}
}

// shouldSkipVerification checks if verification should be skipped
func (o *Orchestrator) shouldSkipVerification(task *Task, policy VerificationPolicy) bool {
	// Check skip labels
	for _, label := range policy.SkipLabels {
		if _, exists := task.Labels[label]; exists {
			return true
		}
	}

	// Check always-verify labels
	for _, label := range policy.AlwaysVerifyLabels {
		if _, exists := task.Labels[label]; exists {
			return false
		}
	}

	return false
}

// healthCheckLoop monitors agent health
func (o *Orchestrator) healthCheckLoop() {
	defer o.wg.Done()

	ticker := time.NewTicker(o.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			o.checkAgentHealth()
		}
	}
}

// checkAgentHealth identifies and handles stale agents
func (o *Orchestrator) checkAgentHealth() {
	ctx := o.ctx

	stale := o.registry.FindStaleAgents()
	for _, agent := range stale {
		log.Printf("[Orchestrator] Agent %s is stale (last heartbeat: %v)",
			agent.ID, agent.LastHeartbeat)

		// Mark as failed
		if err := o.registry.SetStatus(ctx, agent.ID, AgentStatusFailed); err != nil {
			log.Printf("[Orchestrator] Failed to update stale agent %s: %v", agent.ID, err)
			continue
		}

		// If agent had a task, put it back in revision
		if agent.CurrentTaskID != "" {
			task, err := o.queue.Get(ctx, agent.CurrentTaskID)
			if err == nil && task != nil {
				o.queue.Reject(ctx, task.ID, "Agent became unresponsive")
			}
		}

		o.postChat(ctx, ChatMessage{
			AgentID:   "orchestrator",
			AgentName: "Orchestrator",
			Role:      RoleOrchestrator,
			Type:      "health",
			Content:   fmt.Sprintf("Agent %s marked as failed (unresponsive)", agent.ID),
		})
	}
}

// postChat adds a message to the chat history and publishes it
func (o *Orchestrator) postChat(ctx context.Context, msg ChatMessage) {
	o.mu.Lock()
	msg.ID = fmt.Sprintf("msg-%d", len(o.chatHistory)+1)
	msg.ProjectID = o.projectID
	msg.Timestamp = time.Now()
	o.chatHistory = append(o.chatHistory, msg)
	o.mu.Unlock()

	// Publish to broker
	if o.broker != nil {
		o.broker.Publish(ctx, plugin.Message{
			Topic:   "swarm.chat",
			Payload: msg,
			Source:  "orchestrator",
		})
	}
}

// PostWorkerMessage allows workers to post to chat
func (o *Orchestrator) PostWorkerMessage(ctx context.Context, agentID, content, msgType string, taskID string) {
	agent, err := o.registry.Get(ctx, agentID)
	if err != nil {
		return
	}

	o.postChat(ctx, ChatMessage{
		AgentID:   agentID,
		AgentName: agent.Name,
		Role:      agent.Role,
		TaskID:    taskID,
		Type:      msgType,
		Content:   content,
	})
}

// CompleteTask is called by workers when they finish a task
func (o *Orchestrator) CompleteTask(ctx context.Context, agentID string, taskID string, artifacts []Artifact) error {
	// Update task queue
	if err := o.queue.Complete(ctx, taskID, artifacts); err != nil {
		return err
	}

	o.postChat(ctx, ChatMessage{
		AgentID:   agentID,
		AgentName: agentID, // Will be updated when we have the agent
		Role:      RoleWorker,
		TaskID:    taskID,
		Type:      "completion",
		Content:   fmt.Sprintf("Task %s completed, ready for review", taskID),
	})

	return nil
}
