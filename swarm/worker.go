package swarm

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"bicycle/plugin"
)

// WorkerAgent executes tasks using an LLM
type WorkerAgent struct {
	mu     sync.RWMutex
	agent  *Agent
	client LLMClient
	broker plugin.MessageBroker

	// Current work
	currentTask    *Task
	currentContext WorkerContext
	cancel         context.CancelFunc

	// Callbacks
	onComplete func(agentID string, taskID string, artifacts []Artifact)
	onProgress func(agentID string, taskID string, message string)
	onError    func(agentID string, taskID string, err error)
}

// WorkerConfig configures a worker agent
type WorkerConfig struct {
	ID           string
	Name         string
	Model        Model
	Capabilities []string
	Client       LLMClient
	Broker       plugin.MessageBroker
	OnComplete   func(agentID string, taskID string, artifacts []Artifact)
	OnProgress   func(agentID string, taskID string, message string)
	OnError      func(agentID string, taskID string, err error)
}

// NewWorkerAgent creates a new worker agent
func NewWorkerAgent(config WorkerConfig) *WorkerAgent {
	return &WorkerAgent{
		agent: &Agent{
			ID:           config.ID,
			Name:         config.Name,
			Role:         RoleWorker,
			Status:       AgentStatusIdle,
			Model:        string(config.Model),
			Capabilities: config.Capabilities,
			StartedAt:    time.Now(),
			Config: AgentConfig{
				MaxConcurrentTasks: 1,
				TaskTimeout:        30 * time.Minute,
				HeartbeatInterval:  30 * time.Second,
			},
		},
		client:     config.Client,
		broker:     config.Broker,
		onComplete: config.OnComplete,
		onProgress: config.OnProgress,
		onError:    config.OnError,
	}
}

// GetAgent returns the agent metadata
func (w *WorkerAgent) GetAgent() *Agent {
	w.mu.RLock()
	defer w.mu.RUnlock()
	agentCopy := *w.agent
	return &agentCopy
}

// Start begins the worker agent
func (w *WorkerAgent) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.agent.Status = AgentStatusIdle
	w.agent.LastHeartbeat = time.Now()

	log.Printf("[Worker %s] Started", w.agent.ID)
	return nil
}

// Stop stops the worker agent
func (w *WorkerAgent) Stop(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancel != nil {
		w.cancel()
	}

	w.agent.Status = AgentStatusStopped
	log.Printf("[Worker %s] Stopped", w.agent.ID)
	return nil
}

// ExecuteTask executes a task
func (w *WorkerAgent) ExecuteTask(ctx context.Context, task *Task, workerCtx WorkerContext) error {
	w.mu.Lock()
	if w.agent.Status != AgentStatusIdle {
		w.mu.Unlock()
		return fmt.Errorf("worker is not idle (status: %s)", w.agent.Status)
	}

	taskCtx, cancel := context.WithTimeout(ctx, w.agent.Config.TaskTimeout)
	w.cancel = cancel
	w.currentTask = task
	w.currentContext = workerCtx
	w.agent.Status = AgentStatusWorking
	w.agent.CurrentTaskID = task.ID
	w.agent.LastHeartbeat = time.Now()
	w.mu.Unlock()

	// Execute in background
	go w.executeTaskInternal(taskCtx, task, workerCtx)

	return nil
}

// executeTaskInternal does the actual work
func (w *WorkerAgent) executeTaskInternal(ctx context.Context, task *Task, workerCtx WorkerContext) {
	defer func() {
		w.mu.Lock()
		w.agent.Status = AgentStatusIdle
		w.agent.CurrentTaskID = ""
		w.currentTask = nil
		w.cancel = nil
		w.mu.Unlock()
	}()

	w.reportProgress("Starting task execution")

	// Build prompt
	prompt := WorkerPrompt(task, workerCtx)

	// Create LLM request
	req := NewPromptBuilder().
		System("You are an expert software developer. Complete the assigned task thoroughly and correctly.").
		User(prompt).
		Build(Model(w.agent.Model))

	req.MaxTokens = 4096

	// Call LLM
	w.reportProgress("Processing with LLM...")

	resp, err := w.client.Complete(ctx, req)
	if err != nil {
		w.reportError(fmt.Errorf("LLM call failed: %w", err))
		return
	}

	w.reportProgress("LLM processing complete")

	// Parse response and create artifacts
	artifacts := w.parseArtifacts(resp.Content, task)

	// Report completion
	w.mu.RLock()
	onComplete := w.onComplete
	agentID := w.agent.ID
	w.mu.RUnlock()

	if onComplete != nil {
		onComplete(agentID, task.ID, artifacts)
	}

	w.reportProgress("Task completed, ready for review")

	// Update stats
	w.mu.Lock()
	w.agent.TasksCompleted++
	total := w.agent.TasksCompleted + w.agent.TasksFailed
	w.agent.SuccessRate = float64(w.agent.TasksCompleted) / float64(total)
	w.mu.Unlock()
}

// parseArtifacts extracts artifacts from LLM response
func (w *WorkerAgent) parseArtifacts(content string, task *Task) []Artifact {
	// For now, create a single artifact with the full response
	// In a real implementation, this would parse code blocks, files, etc.
	return []Artifact{
		{
			ID:        fmt.Sprintf("%s-artifact-1", task.ID),
			TaskID:    task.ID,
			Type:      "response",
			Content:   content,
			CreatedAt: time.Now(),
		},
	}
}

// reportProgress reports progress
func (w *WorkerAgent) reportProgress(message string) {
	w.mu.RLock()
	onProgress := w.onProgress
	agentID := w.agent.ID
	taskID := ""
	if w.currentTask != nil {
		taskID = w.currentTask.ID
	}
	w.mu.RUnlock()

	log.Printf("[Worker %s] %s", agentID, message)

	if onProgress != nil && taskID != "" {
		onProgress(agentID, taskID, message)
	}
}

// reportError reports an error
func (w *WorkerAgent) reportError(err error) {
	w.mu.RLock()
	onError := w.onError
	agentID := w.agent.ID
	taskID := ""
	if w.currentTask != nil {
		taskID = w.currentTask.ID
	}
	w.mu.RUnlock()

	log.Printf("[Worker %s] Error: %v", agentID, err)

	if onError != nil && taskID != "" {
		onError(agentID, taskID, err)
	}

	// Update failure stats
	w.mu.Lock()
	w.agent.TasksFailed++
	total := w.agent.TasksCompleted + w.agent.TasksFailed
	if total > 0 {
		w.agent.SuccessRate = float64(w.agent.TasksCompleted) / float64(total)
	}
	w.mu.Unlock()
}

// Cancel cancels the current task
func (w *WorkerAgent) Cancel() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancel != nil {
		w.cancel()
		log.Printf("[Worker %s] Task cancelled", w.agent.ID)
	}
}

// Heartbeat updates the agent's heartbeat
func (w *WorkerAgent) Heartbeat() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.agent.LastHeartbeat = time.Now()
}

// IsIdle returns true if the worker is idle
func (w *WorkerAgent) IsIdle() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.agent.Status == AgentStatusIdle
}

// GetStatus returns the current status
func (w *WorkerAgent) GetStatus() AgentStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.agent.Status
}
