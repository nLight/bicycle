package swarm

import (
	"context"
	"fmt"
	"log"
	"sync"

	"bicycle/internal/config"
	"bicycle/plugin"
	"bicycle/state"
	"bicycle/swarm"
)

func init() {
	plugin.Register(&SwarmPlugin{})
}

// SwarmPlugin provides swarm orchestration capabilities
type SwarmPlugin struct {
	mu           sync.RWMutex
	broker       plugin.MessageBroker
	stateManager *state.Manager
	config       *config.Config

	// Active projects
	projects     map[string]*swarm.Orchestrator
	spawners     map[string]*swarm.AgentSpawner

	// LLM client
	llmClient    swarm.LLMClient

	ctx    context.Context
	cancel context.CancelFunc
}

// Name returns the plugin name
func (p *SwarmPlugin) Name() string {
	return "swarm"
}

// CheckRequirements verifies the plugin can run
func (p *SwarmPlugin) CheckRequirements(ctx context.Context) error {
	return nil
}

// Extensions returns the plugin's extensions
func (p *SwarmPlugin) Extensions() []plugin.Extension {
	return []plugin.Extension{
		&SwarmCommands{plugin: p},
	}
}

// Start initializes the swarm plugin
func (p *SwarmPlugin) Start(ctx context.Context, broker plugin.MessageBroker) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.broker = broker
	p.projects = make(map[string]*swarm.Orchestrator)
	p.spawners = make(map[string]*swarm.AgentSpawner)
	p.ctx, p.cancel = context.WithCancel(ctx)

	// Get config from context
	if cfg, ok := ctx.Value("config").(*config.Config); ok {
		p.config = cfg
	}

	// Get state manager from daemon
	if daemon, ok := ctx.Value("daemon").(interface{ GetStateManager() *state.Manager }); ok {
		p.stateManager = daemon.GetStateManager()
	}

	// Create mock LLM client for now
	// TODO: Replace with real Anthropic client
	p.llmClient = swarm.NewMockLLMClient()

	log.Println("[SwarmPlugin] Started")
	return nil
}

// Stop shuts down the swarm plugin
func (p *SwarmPlugin) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Stop all orchestrators
	for id, orch := range p.projects {
		log.Printf("[SwarmPlugin] Stopping project %s", id)
		orch.Stop(ctx)
	}

	// Stop all spawners
	for id, spawner := range p.spawners {
		log.Printf("[SwarmPlugin] Stopping spawner %s", id)
		spawner.StopAll(ctx)
	}

	if p.cancel != nil {
		p.cancel()
	}

	log.Println("[SwarmPlugin] Stopped")
	return nil
}

// SetLLMClient sets the LLM client
func (p *SwarmPlugin) SetLLMClient(client swarm.LLMClient) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.llmClient = client
}

// CreateProject creates a new project
func (p *SwarmPlugin) CreateProject(ctx context.Context, project *swarm.Project) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.projects[project.ID]; exists {
		return fmt.Errorf("project %s already exists", project.ID)
	}

	// Create verifier
	verifier := swarm.NewLLMVerifier(p.llmClient, swarm.Model(project.OrchestratorModel))

	// Create spawner
	spawner := swarm.NewAgentSpawner(swarm.SpawnerConfig{
		Client:       p.llmClient,
		Broker:       p.broker,
		DefaultModel: swarm.Model(project.WorkerModel),
	})

	// Create orchestrator
	orch := swarm.NewOrchestrator(
		p.stateManager,
		p.broker,
		project,
		verifier,
		spawner,
		swarm.DefaultOrchestratorConfig(),
	)

	// Wire spawner to orchestrator
	spawner = swarm.NewAgentSpawner(swarm.SpawnerConfig{
		Client:       p.llmClient,
		Broker:       p.broker,
		DefaultModel: swarm.Model(project.WorkerModel),
		Orchestrator: orch,
	})

	p.projects[project.ID] = orch
	p.spawners[project.ID] = spawner

	log.Printf("[SwarmPlugin] Created project: %s", project.ID)
	return nil
}

// StartProject starts a project's orchestration
func (p *SwarmPlugin) StartProject(ctx context.Context, projectID string, workerCount int) error {
	p.mu.Lock()
	orch, exists := p.projects[projectID]
	spawner := p.spawners[projectID]
	p.mu.Unlock()

	if !exists {
		return fmt.Errorf("project %s not found", projectID)
	}

	// Start orchestrator
	if err := orch.Start(ctx); err != nil {
		return err
	}

	// Spawn workers
	agents, err := spawner.SpawnN(ctx, workerCount, swarm.AgentConfig{})
	if err != nil {
		return err
	}

	// Register agents with orchestrator
	registry := orch.GetRegistry()
	for _, agent := range agents {
		registry.Register(ctx, agent)
	}

	log.Printf("[SwarmPlugin] Started project %s with %d workers", projectID, workerCount)
	return nil
}

// StopProject stops a project
func (p *SwarmPlugin) StopProject(ctx context.Context, projectID string) error {
	p.mu.Lock()
	orch, exists := p.projects[projectID]
	spawner := p.spawners[projectID]
	p.mu.Unlock()

	if !exists {
		return fmt.Errorf("project %s not found", projectID)
	}

	spawner.StopAll(ctx)
	orch.Stop(ctx)

	log.Printf("[SwarmPlugin] Stopped project %s", projectID)
	return nil
}

// AddTask adds a task to a project
func (p *SwarmPlugin) AddTask(ctx context.Context, projectID string, task *swarm.Task) error {
	p.mu.RLock()
	orch, exists := p.projects[projectID]
	p.mu.RUnlock()

	if !exists {
		return fmt.Errorf("project %s not found", projectID)
	}

	return orch.AddTask(ctx, task)
}

// GetProjectStatus returns a project's status
func (p *SwarmPlugin) GetProjectStatus(projectID string) string {
	p.mu.RLock()
	orch, exists := p.projects[projectID]
	spawner := p.spawners[projectID]
	p.mu.RUnlock()

	if !exists {
		return fmt.Sprintf("Project %s not found", projectID)
	}

	queue := orch.GetQueue()
	registry := orch.GetRegistry()
	counts := queue.CountByStatus()

	status := fmt.Sprintf("Project: %s\n", projectID)
	status += fmt.Sprintf("Tasks: %d total\n", queue.Count())
	status += fmt.Sprintf("  Pending: %d\n", counts[swarm.TaskStatusPending])
	status += fmt.Sprintf("  Assigned: %d\n", counts[swarm.TaskStatusAssigned])
	status += fmt.Sprintf("  In Progress: %d\n", counts[swarm.TaskStatusInProgress])
	status += fmt.Sprintf("  Review: %d\n", counts[swarm.TaskStatusReview])
	status += fmt.Sprintf("  Completed: %d\n", counts[swarm.TaskStatusCompleted])
	status += fmt.Sprintf("  Failed: %d\n", counts[swarm.TaskStatusFailed])
	status += fmt.Sprintf("Agents: %d registered, %d spawned\n", registry.Count(), spawner.Count())

	// Recent chat
	chat := orch.GetChatHistory(5)
	if len(chat) > 0 {
		status += "\nRecent Activity:\n"
		for _, msg := range chat {
			status += fmt.Sprintf("  [%s] %s: %s\n", msg.Type, msg.AgentName, truncate(msg.Content, 60))
		}
	}

	return status
}

// ListProjects returns all project IDs
func (p *SwarmPlugin) ListProjects() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ids := make([]string, 0, len(p.projects))
	for id := range p.projects {
		ids = append(ids, id)
	}
	return ids
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// SwarmCommands provides CLI commands for the swarm
type SwarmCommands struct {
	plugin *SwarmPlugin
}

// Type returns the extension type
func (c *SwarmCommands) Type() plugin.ExtensionType {
	return plugin.ExtensionTypeCommand
}

// Name returns the extension name
func (c *SwarmCommands) Name() string {
	return "swarm-commands"
}

// SupportsMode returns true if the extension supports the given mode
func (c *SwarmCommands) SupportsMode(mode plugin.Mode) bool {
	return true
}

// Commands returns the available commands
func (c *SwarmCommands) Commands() []*plugin.Command {
	return []*plugin.Command{
		{
			Name:        "swarm",
			Description: "Swarm management commands",
			Usage:       "<subcommand> [args]",
			Handler:     c.handleSwarm,
		},
		{
			Name:        "swarm-create",
			Description: "Create a new swarm project",
			Usage:       "<project-id> [worker-model] [verifier-model]",
			Handler:     c.handleCreate,
		},
		{
			Name:        "swarm-start",
			Description: "Start a swarm project",
			Usage:       "<project-id> [worker-count]",
			Handler:     c.handleStart,
		},
		{
			Name:        "swarm-stop",
			Description: "Stop a swarm project",
			Usage:       "<project-id>",
			Handler:     c.handleStop,
		},
		{
			Name:        "swarm-task",
			Description: "Add a task to a project",
			Usage:       "<project-id> <task-type> <title> <description>",
			Handler:     c.handleTask,
		},
		{
			Name:        "swarm-status",
			Description: "Show swarm project status",
			Usage:       "[project-id]",
			Handler:     c.handleStatus,
		},
	}
}

func (c *SwarmCommands) handleSwarm(ctx context.Context, args []string) (*plugin.CommandResult, error) {
	help := `Swarm Commands:
  /swarm-create <id> [worker-model] [verifier-model] - Create project
  /swarm-start <id> [workers]                        - Start project
  /swarm-stop <id>                                   - Stop project
  /swarm-task <id> <type> <title> <desc>            - Add task
  /swarm-status [id]                                 - Show status

Models: haiku, sonnet, opus`

	return &plugin.CommandResult{Output: help}, nil
}

func (c *SwarmCommands) handleCreate(ctx context.Context, args []string) (*plugin.CommandResult, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: /swarm-create <project-id> [worker-model] [verifier-model]")
	}

	projectID := args[0]
	workerModel := "haiku"
	verifierModel := "opus"

	if len(args) > 1 {
		workerModel = args[1]
	}
	if len(args) > 2 {
		verifierModel = args[2]
	}

	project := &swarm.Project{
		ID:                projectID,
		Name:              projectID,
		Status:            swarm.ProjectStatusDraft,
		WorkerModel:       workerModel,
		OrchestratorModel: verifierModel,
		VerificationPolicy: swarm.DefaultVerificationPolicy(),
	}

	if err := c.plugin.CreateProject(ctx, project); err != nil {
		return nil, err
	}

	return &plugin.CommandResult{
		Output: fmt.Sprintf("Created project: %s (workers: %s, verifier: %s)", projectID, workerModel, verifierModel),
	}, nil
}

func (c *SwarmCommands) handleStart(ctx context.Context, args []string) (*plugin.CommandResult, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: /swarm-start <project-id> [worker-count]")
	}

	projectID := args[0]
	workerCount := 3

	if len(args) > 1 {
		fmt.Sscanf(args[1], "%d", &workerCount)
	}

	if err := c.plugin.StartProject(ctx, projectID, workerCount); err != nil {
		return nil, err
	}

	return &plugin.CommandResult{
		Output: fmt.Sprintf("Started project %s with %d workers", projectID, workerCount),
	}, nil
}

func (c *SwarmCommands) handleStop(ctx context.Context, args []string) (*plugin.CommandResult, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: /swarm-stop <project-id>")
	}

	projectID := args[0]

	if err := c.plugin.StopProject(ctx, projectID); err != nil {
		return nil, err
	}

	return &plugin.CommandResult{
		Output: fmt.Sprintf("Stopped project %s", projectID),
	}, nil
}

func (c *SwarmCommands) handleTask(ctx context.Context, args []string) (*plugin.CommandResult, error) {
	if len(args) < 4 {
		return nil, fmt.Errorf("usage: /swarm-task <project-id> <type> <title> <description>")
	}

	projectID := args[0]
	taskType := args[1]
	title := args[2]
	description := args[3]

	task := &swarm.Task{
		ID:          fmt.Sprintf("task-%d", len(args)),
		Type:        taskType,
		Title:       title,
		Description: description,
		Priority:    swarm.PriorityNormal,
	}

	if err := c.plugin.AddTask(ctx, projectID, task); err != nil {
		return nil, err
	}

	return &plugin.CommandResult{
		Output: fmt.Sprintf("Added task %s to project %s", task.ID, projectID),
	}, nil
}

func (c *SwarmCommands) handleStatus(ctx context.Context, args []string) (*plugin.CommandResult, error) {
	if len(args) < 1 {
		// List all projects
		projects := c.plugin.ListProjects()
		if len(projects) == 0 {
			return &plugin.CommandResult{Output: "No active projects"}, nil
		}

		output := "Active Projects:\n"
		for _, id := range projects {
			output += fmt.Sprintf("  - %s\n", id)
		}
		return &plugin.CommandResult{Output: output}, nil
	}

	projectID := args[0]
	status := c.plugin.GetProjectStatus(projectID)

	return &plugin.CommandResult{Output: status}, nil
}
