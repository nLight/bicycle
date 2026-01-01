package swarm

import (
	"context"
	"fmt"
	"log"
	"sync"

	"bicycle/plugin"
)

// AgentSpawner manages the lifecycle of worker agents
type AgentSpawner struct {
	mu      sync.RWMutex
	workers map[string]*WorkerAgent
	client  LLMClient
	broker  plugin.MessageBroker
	model   Model

	// Callback for completed tasks
	orchestrator *Orchestrator
}

// SpawnerConfig configures the agent spawner
type SpawnerConfig struct {
	Client       LLMClient
	Broker       plugin.MessageBroker
	DefaultModel Model
	Orchestrator *Orchestrator
}

// NewAgentSpawner creates a new agent spawner
func NewAgentSpawner(config SpawnerConfig) *AgentSpawner {
	return &AgentSpawner{
		workers:      make(map[string]*WorkerAgent),
		client:       config.Client,
		broker:       config.Broker,
		model:        config.DefaultModel,
		orchestrator: config.Orchestrator,
	}
}

// Spawn creates a new worker agent
func (s *AgentSpawner) Spawn(ctx context.Context, config AgentConfig) (*Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate ID
	id := fmt.Sprintf("worker-%d", len(s.workers)+1)

	worker := NewWorkerAgent(WorkerConfig{
		ID:           id,
		Name:         fmt.Sprintf("Worker %d", len(s.workers)+1),
		Model:        s.model,
		Capabilities: []string{"code", "review"},
		Client:       s.client,
		Broker:       s.broker,
		OnComplete:   s.handleTaskComplete,
		OnProgress:   s.handleTaskProgress,
		OnError:      s.handleTaskError,
	})

	if err := worker.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start worker: %w", err)
	}

	s.workers[id] = worker

	log.Printf("[Spawner] Spawned new worker: %s", id)

	return worker.GetAgent(), nil
}

// SpawnN spawns N workers
func (s *AgentSpawner) SpawnN(ctx context.Context, n int, config AgentConfig) ([]*Agent, error) {
	agents := make([]*Agent, 0, n)
	for i := 0; i < n; i++ {
		agent, err := s.Spawn(ctx, config)
		if err != nil {
			return agents, err
		}
		agents = append(agents, agent)
	}
	return agents, nil
}

// Restart restarts a worker with new task context
func (s *AgentSpawner) Restart(ctx context.Context, agentID string, task *Task) error {
	s.mu.RLock()
	worker, exists := s.workers[agentID]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("worker %s not found", agentID)
	}

	// Build worker context
	var workerCtx WorkerContext
	if s.orchestrator != nil {
		workerCtx = s.orchestrator.buildWorkerContext(task)
	} else {
		workerCtx = WorkerContext{
			Task:             task,
			PreviousAttempts: task.PreviousAttempts,
			RetryCount:       task.RetryCount,
			MaxRetries:       task.MaxRetries,
		}
	}

	// Execute task
	return worker.ExecuteTask(ctx, task, workerCtx)
}

// Stop stops a worker agent
func (s *AgentSpawner) Stop(ctx context.Context, agentID string) error {
	s.mu.Lock()
	worker, exists := s.workers[agentID]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("worker %s not found", agentID)
	}
	delete(s.workers, agentID)
	s.mu.Unlock()

	return worker.Stop(ctx)
}

// StopAll stops all workers
func (s *AgentSpawner) StopAll(ctx context.Context) error {
	s.mu.Lock()
	workers := make([]*WorkerAgent, 0, len(s.workers))
	for _, w := range s.workers {
		workers = append(workers, w)
	}
	s.workers = make(map[string]*WorkerAgent)
	s.mu.Unlock()

	var lastErr error
	for _, w := range workers {
		if err := w.Stop(ctx); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// GetWorker returns a worker by ID
func (s *AgentSpawner) GetWorker(agentID string) (*WorkerAgent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.workers[agentID]
	return w, ok
}

// GetIdleWorker returns an idle worker
func (s *AgentSpawner) GetIdleWorker() *WorkerAgent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, w := range s.workers {
		if w.IsIdle() {
			return w
		}
	}
	return nil
}

// Count returns the number of workers
func (s *AgentSpawner) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.workers)
}

// handleTaskComplete is called when a worker completes a task
func (s *AgentSpawner) handleTaskComplete(agentID string, taskID string, artifacts []Artifact) {
	log.Printf("[Spawner] Worker %s completed task %s with %d artifacts", agentID, taskID, len(artifacts))

	if s.orchestrator != nil {
		ctx := context.Background()
		if err := s.orchestrator.CompleteTask(ctx, agentID, taskID, artifacts); err != nil {
			log.Printf("[Spawner] Failed to report task completion: %v", err)
		}
	}
}

// handleTaskProgress is called when a worker reports progress
func (s *AgentSpawner) handleTaskProgress(agentID string, taskID string, message string) {
	log.Printf("[Spawner] Worker %s progress on task %s: %s", agentID, taskID, message)

	if s.orchestrator != nil {
		ctx := context.Background()
		s.orchestrator.PostWorkerMessage(ctx, agentID, message, "progress", taskID)
	}
}

// handleTaskError is called when a worker encounters an error
func (s *AgentSpawner) handleTaskError(agentID string, taskID string, err error) {
	log.Printf("[Spawner] Worker %s error on task %s: %v", agentID, taskID, err)

	if s.orchestrator != nil {
		ctx := context.Background()
		s.orchestrator.PostWorkerMessage(ctx, agentID, fmt.Sprintf("Error: %v", err), "error", taskID)
	}
}

// Ensure AgentSpawner implements WorkerSpawner
var _ WorkerSpawner = (*AgentSpawner)(nil)
