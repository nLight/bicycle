package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"bicycle/state"
	"bicycle/swarm"
)

func main() {
	// Parse flags
	apiKey := flag.String("api-key", "", "Anthropic API key (or set ANTHROPIC_API_KEY)")
	workerCount := flag.Int("workers", 2, "Number of worker agents")
	useMock := flag.Bool("mock", false, "Use mock LLM client (no API calls)")
	flag.Parse()

	// Get API key
	key := *apiKey
	if key == "" {
		key = os.Getenv("ANTHROPIC_API_KEY")
	}

	// Create LLM client
	var llmClient swarm.LLMClient
	if *useMock || key == "" {
		log.Println("Using mock LLM client (no real API calls)")
		mock := swarm.NewMockLLMClient()
		mock.DefaultResp = `I've analyzed the task and implemented the required functionality.

## Implementation

The code has been written following best practices:
- Clean architecture
- Error handling
- Proper documentation

## Testing

All tests pass and edge cases are covered.

## Summary

Task completed successfully.`
		llmClient = mock
	} else {
		log.Println("Using Anthropic API")
		llmClient = swarm.NewAnthropicClient(key)
	}

	// Create state manager
	backend := state.NewMemoryBackend()
	stateManager := state.NewManager(backend)

	// Create project
	project := &swarm.Project{
		ID:                "demo-project",
		Name:              "Demo Project",
		Status:            swarm.ProjectStatusActive,
		WorkerModel:       "haiku",
		OrchestratorModel: "opus",
		VerificationPolicy: swarm.VerificationPolicy{
			BatchSize:  1,
			MaxRetries: 3,
		},
	}

	// Create verifier
	var verifier swarm.Verifier
	if *useMock || key == "" {
		// Use rule-based verifier for mock mode
		verifier = swarm.NewRuleBasedVerifier()
	} else {
		verifier = swarm.NewLLMVerifier(llmClient, swarm.ModelOpus)
	}

	// Create spawner (will be wired up after orchestrator)
	spawner := swarm.NewAgentSpawner(swarm.SpawnerConfig{
		Client:       llmClient,
		DefaultModel: swarm.ModelHaiku,
	})

	// Create orchestrator
	orch := swarm.NewOrchestrator(
		stateManager,
		nil, // No broker for demo
		project,
		verifier,
		spawner,
		swarm.OrchestratorConfig{
			VerificationInterval: 2 * time.Second,
			AssignmentInterval:   1 * time.Second,
			HealthCheckInterval:  5 * time.Second,
		},
	)

	// Rewire spawner with orchestrator
	spawner = swarm.NewAgentSpawner(swarm.SpawnerConfig{
		Client:       llmClient,
		DefaultModel: swarm.ModelHaiku,
		Orchestrator: orch,
	})
	orch.SetSpawner(spawner)

	ctx := context.Background()

	// Start orchestrator
	log.Println("Starting orchestrator...")
	if err := orch.Start(ctx); err != nil {
		log.Fatalf("Failed to start orchestrator: %v", err)
	}

	// Spawn workers
	log.Printf("Spawning %d workers...", *workerCount)
	agents, err := spawner.SpawnN(ctx, *workerCount, swarm.AgentConfig{})
	if err != nil {
		log.Fatalf("Failed to spawn workers: %v", err)
	}

	// Register agents
	registry := orch.GetRegistry()
	for _, agent := range agents {
		registry.Register(ctx, agent)
		log.Printf("  Registered agent: %s (%s)", agent.ID, agent.Model)
	}

	// Add demo tasks
	tasks := []*swarm.Task{
		{
			ID:          "task-1",
			Type:        "implement",
			Title:       "Create user authentication",
			Description: "Implement JWT-based user authentication with login/logout endpoints",
			Priority:    swarm.PriorityHigh,
			AcceptanceCriteria: []string{
				"Login endpoint returns JWT token",
				"Logout invalidates token",
				"Protected routes require valid token",
			},
		},
		{
			ID:          "task-2",
			Type:        "implement",
			Title:       "Add database migrations",
			Description: "Set up database migration system with initial schema",
			Priority:    swarm.PriorityNormal,
			AcceptanceCriteria: []string{
				"Migration system is initialized",
				"Initial user table schema created",
				"Migrations can be rolled back",
			},
		},
		{
			ID:          "task-3",
			Type:        "test",
			Title:       "Write integration tests",
			Description: "Create integration tests for the authentication flow",
			Priority:    swarm.PriorityNormal,
			AcceptanceCriteria: []string{
				"Test successful login",
				"Test failed login",
				"Test token expiration",
			},
		},
	}

	log.Println("\nAdding tasks...")
	for _, task := range tasks {
		if err := orch.AddTask(ctx, task); err != nil {
			log.Printf("Failed to add task %s: %v", task.ID, err)
		} else {
			log.Printf("  Added: %s - %s", task.ID, task.Title)
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("🚴 Bicycle Swarm Demo Running")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Project: %s\n", project.Name)
	fmt.Printf("Workers: %d (%s)\n", *workerCount, project.WorkerModel)
	fmt.Printf("Verifier: %s\n", project.OrchestratorModel)
	fmt.Printf("Tasks: %d\n", len(tasks))
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("\nPress Ctrl+C to stop")

	// Status ticker
	statusTicker := time.NewTicker(3 * time.Second)
	defer statusTicker.Stop()

	// Shutdown handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-sigChan:
			fmt.Println("\nShutting down...")
			spawner.StopAll(ctx)
			orch.Stop(ctx)
			stateManager.Close()
			fmt.Println("Goodbye!")
			return

		case <-statusTicker.C:
			printStatus(orch, spawner)
		}
	}
}

func printStatus(orch *swarm.Orchestrator, spawner *swarm.AgentSpawner) {
	queue := orch.GetQueue()
	counts := queue.CountByStatus()

	fmt.Println("\n--- Status Update ---")
	fmt.Printf("Tasks: %d pending, %d assigned, %d review, %d completed, %d failed\n",
		counts[swarm.TaskStatusPending],
		counts[swarm.TaskStatusAssigned],
		counts[swarm.TaskStatusReview],
		counts[swarm.TaskStatusCompleted],
		counts[swarm.TaskStatusFailed],
	)
	fmt.Printf("Workers: %d active\n", spawner.Count())

	// Recent chat
	chat := orch.GetChatHistory(3)
	if len(chat) > 0 {
		fmt.Println("\nRecent Activity:")
		for _, msg := range chat {
			content := msg.Content
			if len(content) > 80 {
				content = content[:77] + "..."
			}
			fmt.Printf("  [%s] %s: %s\n", msg.Type, msg.AgentName, content)
		}
	}

	// Check if all done
	if counts[swarm.TaskStatusPending] == 0 &&
		counts[swarm.TaskStatusAssigned] == 0 &&
		counts[swarm.TaskStatusReview] == 0 &&
		counts[swarm.TaskStatusRevision] == 0 {
		fmt.Println("\n✅ All tasks completed!")
	}
}
