package swarm

import (
	"context"
	"fmt"
	"sync"
	"time"

	"bicycle/state"
)

const (
	agentKeyPrefix = "agents:"
)

// AgentRegistry manages agent registration and lifecycle
type AgentRegistry struct {
	mu        sync.RWMutex
	state     *state.Manager
	projectID string

	// In-memory index
	agents   map[string]*Agent
	byStatus map[AgentStatus][]*Agent
	byRole   map[AgentRole][]*Agent

	// Health monitoring
	healthCheckInterval time.Duration
	staleThreshold      time.Duration
}

// NewAgentRegistry creates a new agent registry
func NewAgentRegistry(stateManager *state.Manager, projectID string) *AgentRegistry {
	return &AgentRegistry{
		state:               stateManager,
		projectID:           projectID,
		agents:              make(map[string]*Agent),
		byStatus:            make(map[AgentStatus][]*Agent),
		byRole:              make(map[AgentRole][]*Agent),
		healthCheckInterval: 30 * time.Second,
		staleThreshold:      2 * time.Minute,
	}
}

// Load loads agents from persistent storage
func (r *AgentRegistry) Load(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	prefix := r.agentKey("")
	keys, err := r.state.List(ctx, prefix)
	if err != nil {
		return fmt.Errorf("failed to list agents: %w", err)
	}

	r.agents = make(map[string]*Agent)
	r.byStatus = make(map[AgentStatus][]*Agent)
	r.byRole = make(map[AgentRole][]*Agent)

	for _, key := range keys {
		var agent Agent
		if err := r.state.Get(ctx, key, &agent); err != nil {
			continue
		}
		r.agents[agent.ID] = &agent
		r.byStatus[agent.Status] = append(r.byStatus[agent.Status], &agent)
		r.byRole[agent.Role] = append(r.byRole[agent.Role], &agent)
	}

	return nil
}

// Register adds a new agent to the registry
func (r *AgentRegistry) Register(ctx context.Context, agent *Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if agent.ID == "" {
		return fmt.Errorf("agent ID is required")
	}

	if _, exists := r.agents[agent.ID]; exists {
		return fmt.Errorf("agent %s already registered", agent.ID)
	}

	now := time.Now()
	agent.StartedAt = now
	agent.LastHeartbeat = now

	if agent.Status == "" {
		agent.Status = AgentStatusIdle
	}
	if agent.Role == "" {
		agent.Role = RoleWorker
	}

	// Set default config
	if agent.Config.MaxConcurrentTasks == 0 {
		agent.Config.MaxConcurrentTasks = 1
	}
	if agent.Config.TaskTimeout == 0 {
		agent.Config.TaskTimeout = 30 * time.Minute
	}
	if agent.Config.HeartbeatInterval == 0 {
		agent.Config.HeartbeatInterval = 30 * time.Second
	}

	// Persist
	if err := r.state.Set(ctx, r.agentKey(agent.ID), agent); err != nil {
		return fmt.Errorf("failed to persist agent: %w", err)
	}

	// Update in-memory index
	r.agents[agent.ID] = agent
	r.byStatus[agent.Status] = append(r.byStatus[agent.Status], agent)
	r.byRole[agent.Role] = append(r.byRole[agent.Role], agent)

	return nil
}

// Get retrieves an agent by ID
func (r *AgentRegistry) Get(ctx context.Context, agentID string) (*Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, exists := r.agents[agentID]
	if !exists {
		return nil, fmt.Errorf("agent %s not found", agentID)
	}

	// Return a copy
	agentCopy := *agent
	return &agentCopy, nil
}

// Update updates an agent's information
func (r *AgentRegistry) Update(ctx context.Context, agent *Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.agents[agent.ID]
	if !exists {
		return fmt.Errorf("agent %s not found", agent.ID)
	}

	oldStatus := existing.Status

	// Persist
	if err := r.state.Set(ctx, r.agentKey(agent.ID), agent); err != nil {
		return fmt.Errorf("failed to persist agent: %w", err)
	}

	// Update in-memory index
	r.agents[agent.ID] = agent

	// Update status index if changed
	if oldStatus != agent.Status {
		r.removeFromStatusIndex(agent.ID, oldStatus)
		r.byStatus[agent.Status] = append(r.byStatus[agent.Status], agent)
	}

	return nil
}

// Heartbeat updates an agent's last heartbeat time
func (r *AgentRegistry) Heartbeat(ctx context.Context, agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	agent.LastHeartbeat = time.Now()

	// Persist
	if err := r.state.Set(ctx, r.agentKey(agentID), agent); err != nil {
		return fmt.Errorf("failed to persist agent: %w", err)
	}

	return nil
}

// SetStatus updates an agent's status
func (r *AgentRegistry) SetStatus(ctx context.Context, agentID string, status AgentStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	oldStatus := agent.Status
	agent.Status = status
	agent.LastHeartbeat = time.Now()

	// Persist
	if err := r.state.Set(ctx, r.agentKey(agentID), agent); err != nil {
		return fmt.Errorf("failed to persist agent: %w", err)
	}

	// Update status index
	if oldStatus != status {
		r.removeFromStatusIndex(agentID, oldStatus)
		r.byStatus[status] = append(r.byStatus[status], agent)
	}

	return nil
}

// AssignTask assigns a task to an agent
func (r *AgentRegistry) AssignTask(ctx context.Context, agentID, taskID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	if agent.Status != AgentStatusIdle {
		return fmt.Errorf("agent %s is not idle (status: %s)", agentID, agent.Status)
	}

	oldStatus := agent.Status
	agent.Status = AgentStatusWorking
	agent.CurrentTaskID = taskID
	agent.LastHeartbeat = time.Now()

	// Persist
	if err := r.state.Set(ctx, r.agentKey(agentID), agent); err != nil {
		return fmt.Errorf("failed to persist agent: %w", err)
	}

	// Update status index
	r.removeFromStatusIndex(agentID, oldStatus)
	r.byStatus[agent.Status] = append(r.byStatus[agent.Status], agent)

	return nil
}

// CompleteTask marks an agent as having completed its task
func (r *AgentRegistry) CompleteTask(ctx context.Context, agentID string, success bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	oldStatus := agent.Status
	agent.Status = AgentStatusIdle
	agent.CurrentTaskID = ""
	agent.LastHeartbeat = time.Now()

	if success {
		agent.TasksCompleted++
	} else {
		agent.TasksFailed++
	}

	total := agent.TasksCompleted + agent.TasksFailed
	if total > 0 {
		agent.SuccessRate = float64(agent.TasksCompleted) / float64(total)
	}

	// Persist
	if err := r.state.Set(ctx, r.agentKey(agentID), agent); err != nil {
		return fmt.Errorf("failed to persist agent: %w", err)
	}

	// Update status index
	r.removeFromStatusIndex(agentID, oldStatus)
	r.byStatus[agent.Status] = append(r.byStatus[agent.Status], agent)

	return nil
}

// Unregister removes an agent from the registry
func (r *AgentRegistry) Unregister(ctx context.Context, agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	// Remove from persistence
	if err := r.state.Delete(ctx, r.agentKey(agentID)); err != nil {
		return fmt.Errorf("failed to delete agent: %w", err)
	}

	// Remove from in-memory index
	delete(r.agents, agentID)
	r.removeFromStatusIndex(agentID, agent.Status)
	r.removeFromRoleIndex(agentID, agent.Role)

	return nil
}

// GetIdleWorker returns an idle worker agent
func (r *AgentRegistry) GetIdleWorker(ctx context.Context) (*Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get workers
	workers := r.byRole[RoleWorker]
	for _, agent := range workers {
		if agent.Status == AgentStatusIdle {
			agentCopy := *agent
			return &agentCopy, nil
		}
	}

	return nil, nil // No idle workers
}

// GetOrchestrator returns the orchestrator agent
func (r *AgentRegistry) GetOrchestrator(ctx context.Context) (*Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	orchestrators := r.byRole[RoleOrchestrator]
	if len(orchestrators) == 0 {
		return nil, nil
	}

	// Return first orchestrator
	agentCopy := *orchestrators[0]
	return &agentCopy, nil
}

// ListByRole returns agents with the given role
func (r *AgentRegistry) ListByRole(role AgentRole) []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := r.byRole[role]
	result := make([]*Agent, len(agents))
	for i, a := range agents {
		agentCopy := *a
		result[i] = &agentCopy
	}
	return result
}

// ListByStatus returns agents with the given status
func (r *AgentRegistry) ListByStatus(status AgentStatus) []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := r.byStatus[status]
	result := make([]*Agent, len(agents))
	for i, a := range agents {
		agentCopy := *a
		result[i] = &agentCopy
	}
	return result
}

// FindStaleAgents returns agents that haven't sent a heartbeat recently
func (r *AgentRegistry) FindStaleAgents() []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	threshold := time.Now().Add(-r.staleThreshold)
	var stale []*Agent

	for _, agent := range r.agents {
		if agent.LastHeartbeat.Before(threshold) && agent.Status != AgentStatusStopped {
			agentCopy := *agent
			stale = append(stale, &agentCopy)
		}
	}

	return stale
}

// Count returns the total number of registered agents
func (r *AgentRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.agents)
}

// agentKey generates the storage key for an agent
func (r *AgentRegistry) agentKey(agentID string) string {
	return fmt.Sprintf("%s%s:%s", agentKeyPrefix, r.projectID, agentID)
}

// removeFromStatusIndex removes an agent from the status index
func (r *AgentRegistry) removeFromStatusIndex(agentID string, status AgentStatus) {
	agents := r.byStatus[status]
	for i, a := range agents {
		if a.ID == agentID {
			r.byStatus[status] = append(agents[:i], agents[i+1:]...)
			break
		}
	}
}

// removeFromRoleIndex removes an agent from the role index
func (r *AgentRegistry) removeFromRoleIndex(agentID string, role AgentRole) {
	agents := r.byRole[role]
	for i, a := range agents {
		if a.ID == agentID {
			r.byRole[role] = append(agents[:i], agents[i+1:]...)
			break
		}
	}
}
