package swarm

import (
	"time"
)

// TaskStatus represents the current status of a task
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusAssigned   TaskStatus = "assigned"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusReview     TaskStatus = "review"
	TaskStatusRevision   TaskStatus = "revision"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
)

// TaskPriority represents task priority levels
type TaskPriority int

const (
	PriorityLow    TaskPriority = 0
	PriorityNormal TaskPriority = 1
	PriorityHigh   TaskPriority = 2
	PriorityCritical TaskPriority = 3
)

// Task represents a unit of work in the swarm
type Task struct {
	ID          string            `json:"id"`
	ProjectID   string            `json:"project_id"`
	ParentID    string            `json:"parent_id,omitempty"` // For subtasks
	Type        string            `json:"type"`                // e.g., "implement", "fix", "refactor"
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      TaskStatus        `json:"status"`
	Priority    TaskPriority      `json:"priority"`
	AssignedTo  string            `json:"assigned_to,omitempty"` // Agent ID

	// Acceptance criteria for verification
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`

	// Artifacts produced by this task
	Artifacts []Artifact `json:"artifacts,omitempty"`

	// Verification tracking
	VerificationStatus VerificationStatus `json:"verification_status"`
	RetryCount         int                `json:"retry_count"`
	MaxRetries         int                `json:"max_retries"`

	// Context for restarts
	PreviousAttempts []TaskAttempt `json:"previous_attempts,omitempty"`

	// Metadata
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	DueAt     *time.Time        `json:"due_at,omitempty"`
}

// TaskAttempt records a previous execution attempt
type TaskAttempt struct {
	AttemptNumber int       `json:"attempt_number"`
	AgentID       string    `json:"agent_id"`
	StartedAt     time.Time `json:"started_at"`
	CompletedAt   time.Time `json:"completed_at"`
	Artifacts     []Artifact `json:"artifacts,omitempty"`
	Feedback      string    `json:"feedback,omitempty"` // Orchestrator feedback
	Result        string    `json:"result"`             // "success", "failed", "rejected"
}

// VerificationStatus tracks verification state
type VerificationStatus string

const (
	VerificationPending  VerificationStatus = "pending"
	VerificationPassed   VerificationStatus = "passed"
	VerificationFailed   VerificationStatus = "failed"
	VerificationSkipped  VerificationStatus = "skipped"
)

// Artifact represents a work product
type Artifact struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Type      string    `json:"type"` // "code", "document", "test", "config"
	Path      string    `json:"path,omitempty"`
	Content   string    `json:"content,omitempty"`
	Checksum  string    `json:"checksum,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// AgentStatus represents the current status of an agent
type AgentStatus string

const (
	AgentStatusIdle     AgentStatus = "idle"
	AgentStatusWorking  AgentStatus = "working"
	AgentStatusBlocked  AgentStatus = "blocked"
	AgentStatusStopped  AgentStatus = "stopped"
	AgentStatusFailed   AgentStatus = "failed"
)

// AgentRole defines the agent's role in the swarm
type AgentRole string

const (
	RoleWorker      AgentRole = "worker"
	RoleOrchestrator AgentRole = "orchestrator"
	RoleArchitect   AgentRole = "architect"
	RoleReviewer    AgentRole = "reviewer"
)

// Agent represents a worker in the swarm
type Agent struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Role        AgentRole         `json:"role"`
	Status      AgentStatus       `json:"status"`
	Model       string            `json:"model"` // e.g., "haiku", "sonnet", "opus"

	// Current work
	CurrentTaskID string `json:"current_task_id,omitempty"`

	// Capabilities
	Capabilities []string `json:"capabilities,omitempty"` // e.g., "go", "python", "testing"

	// Performance tracking
	TasksCompleted int     `json:"tasks_completed"`
	TasksFailed    int     `json:"tasks_failed"`
	SuccessRate    float64 `json:"success_rate"`

	// Health
	LastHeartbeat time.Time `json:"last_heartbeat"`
	StartedAt     time.Time `json:"started_at"`

	// Configuration
	Config AgentConfig `json:"config"`
}

// AgentConfig holds agent-specific configuration
type AgentConfig struct {
	MaxConcurrentTasks int           `json:"max_concurrent_tasks"`
	TaskTimeout        time.Duration `json:"task_timeout"`
	HeartbeatInterval  time.Duration `json:"heartbeat_interval"`
}

// ProjectStatus represents project lifecycle status
type ProjectStatus string

const (
	ProjectStatusDraft     ProjectStatus = "draft"
	ProjectStatusActive    ProjectStatus = "active"
	ProjectStatusPaused    ProjectStatus = "paused"
	ProjectStatusCompleted ProjectStatus = "completed"
	ProjectStatusArchived  ProjectStatus = "archived"
)

// Project represents a software project being worked on
type Project struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Status      ProjectStatus `json:"status"`

	// Project definition
	Repository    string   `json:"repository,omitempty"`
	WorkingDir    string   `json:"working_dir"`
	Requirements  []string `json:"requirements,omitempty"`

	// Orchestration settings
	OrchestratorModel   string `json:"orchestrator_model"`   // e.g., "opus"
	WorkerModel         string `json:"worker_model"`         // e.g., "haiku"
	VerificationPolicy  VerificationPolicy `json:"verification_policy"`

	// Statistics
	TotalTasks     int `json:"total_tasks"`
	CompletedTasks int `json:"completed_tasks"`
	FailedTasks    int `json:"failed_tasks"`

	// Metadata
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// VerificationPolicy controls how and when verification occurs
type VerificationPolicy struct {
	// Verify every N task completions
	BatchSize int `json:"batch_size"`

	// Always verify if confidence below this threshold
	ConfidenceThreshold float64 `json:"confidence_threshold"`

	// Maximum retries before escalating
	MaxRetries int `json:"max_retries"`

	// Skip verification for tasks matching these labels
	SkipLabels []string `json:"skip_labels,omitempty"`

	// Always verify tasks matching these labels
	AlwaysVerifyLabels []string `json:"always_verify_labels,omitempty"`
}

// DefaultVerificationPolicy returns sensible defaults
func DefaultVerificationPolicy() VerificationPolicy {
	return VerificationPolicy{
		BatchSize:           1,    // Verify every task
		ConfidenceThreshold: 0.8,
		MaxRetries:          3,
	}
}

// ChatMessage represents a message in the swarm chat
type ChatMessage struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	AgentID   string    `json:"agent_id"`
	AgentName string    `json:"agent_name"`
	Role      AgentRole `json:"role"`
	Content   string    `json:"content"`

	// Optional references
	TaskID    string `json:"task_id,omitempty"`
	ReplyToID string `json:"reply_to_id,omitempty"`

	// Message type for filtering
	Type      string `json:"type"` // "progress", "question", "correction", "completion"

	Timestamp time.Time `json:"timestamp"`
}
