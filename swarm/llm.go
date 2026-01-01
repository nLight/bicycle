package swarm

import (
	"context"
	"fmt"
)

// Model represents an LLM model tier
type Model string

const (
	ModelHaiku  Model = "haiku"
	ModelSonnet Model = "sonnet"
	ModelOpus   Model = "opus"
)

// Message represents a chat message for the LLM
type Message struct {
	Role    string `json:"role"` // "user", "assistant", "system"
	Content string `json:"content"`
}

// LLMRequest represents a request to the LLM
type LLMRequest struct {
	Model       Model     `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	System      string    `json:"system,omitempty"`
}

// LLMResponse represents a response from the LLM
type LLMResponse struct {
	Content      string `json:"content"`
	Model        string `json:"model"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	StopReason   string `json:"stop_reason"`
}

// LLMClient is the interface for interacting with LLMs
type LLMClient interface {
	// Complete sends a request to the LLM and returns the response
	Complete(ctx context.Context, req LLMRequest) (*LLMResponse, error)

	// Stream sends a request and streams the response
	Stream(ctx context.Context, req LLMRequest, handler func(chunk string) error) (*LLMResponse, error)
}

// MockLLMClient implements LLMClient for testing
type MockLLMClient struct {
	Responses   map[string]string // Keyed by first user message content
	DefaultResp string
	Calls       []LLMRequest
}

// NewMockLLMClient creates a new mock LLM client
func NewMockLLMClient() *MockLLMClient {
	return &MockLLMClient{
		Responses:   make(map[string]string),
		DefaultResp: "Mock response",
	}
}

// Complete implements LLMClient
func (m *MockLLMClient) Complete(ctx context.Context, req LLMRequest) (*LLMResponse, error) {
	m.Calls = append(m.Calls, req)

	// Find matching response
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			if resp, ok := m.Responses[msg.Content]; ok {
				return &LLMResponse{
					Content:      resp,
					Model:        string(req.Model),
					InputTokens:  100,
					OutputTokens: 50,
					StopReason:   "end_turn",
				}, nil
			}
		}
	}

	return &LLMResponse{
		Content:      m.DefaultResp,
		Model:        string(req.Model),
		InputTokens:  100,
		OutputTokens: 50,
		StopReason:   "end_turn",
	}, nil
}

// Stream implements LLMClient
func (m *MockLLMClient) Stream(ctx context.Context, req LLMRequest, handler func(chunk string) error) (*LLMResponse, error) {
	resp, err := m.Complete(ctx, req)
	if err != nil {
		return nil, err
	}

	// Simulate streaming by sending content in chunks
	chunkSize := 10
	for i := 0; i < len(resp.Content); i += chunkSize {
		end := i + chunkSize
		if end > len(resp.Content) {
			end = len(resp.Content)
		}
		if err := handler(resp.Content[i:end]); err != nil {
			return nil, err
		}
	}

	return resp, nil
}

// PromptBuilder helps construct prompts for agents
type PromptBuilder struct {
	messages []Message
	system   string
}

// NewPromptBuilder creates a new prompt builder
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{}
}

// System sets the system message
func (b *PromptBuilder) System(content string) *PromptBuilder {
	b.system = content
	return b
}

// User adds a user message
func (b *PromptBuilder) User(content string) *PromptBuilder {
	b.messages = append(b.messages, Message{Role: "user", Content: content})
	return b
}

// Assistant adds an assistant message
func (b *PromptBuilder) Assistant(content string) *PromptBuilder {
	b.messages = append(b.messages, Message{Role: "assistant", Content: content})
	return b
}

// Build creates the LLM request
func (b *PromptBuilder) Build(model Model) LLMRequest {
	return LLMRequest{
		Model:    model,
		Messages: b.messages,
		System:   b.system,
	}
}

// WorkerPrompt generates a prompt for a worker agent
func WorkerPrompt(task *Task, context WorkerContext) string {
	prompt := fmt.Sprintf(`You are a software development agent working on a task.

## Task
ID: %s
Type: %s
Title: %s
Description: %s

## Acceptance Criteria
`, task.ID, task.Type, task.Title, task.Description)

	for i, criterion := range task.AcceptanceCriteria {
		prompt += fmt.Sprintf("%d. %s\n", i+1, criterion)
	}

	if len(context.PreviousAttempts) > 0 {
		prompt += "\n## Previous Attempts\n"
		for _, attempt := range context.PreviousAttempts {
			prompt += fmt.Sprintf("\n### Attempt %d (by %s)\n", attempt.AttemptNumber, attempt.AgentID)
			prompt += fmt.Sprintf("Result: %s\n", attempt.Result)
			if attempt.Feedback != "" {
				prompt += fmt.Sprintf("Feedback: %s\n", attempt.Feedback)
			}
		}
	}

	if context.LastFeedback != "" {
		prompt += fmt.Sprintf("\n## Feedback from Last Review\n%s\n", context.LastFeedback)
	}

	if len(context.RelevantChat) > 0 {
		prompt += "\n## Relevant Discussion\n"
		for _, msg := range context.RelevantChat {
			prompt += fmt.Sprintf("[%s] %s: %s\n", msg.Type, msg.AgentName, msg.Content)
		}
	}

	prompt += `
## Instructions
1. Analyze the task and any previous feedback carefully
2. Implement the required changes
3. Ensure all acceptance criteria are met
4. Report your progress and any blockers

Provide your implementation and explanation.`

	return prompt
}

// VerifierPrompt generates a prompt for a verifier agent
func VerifierPrompt(task *Task) string {
	prompt := fmt.Sprintf(`You are a senior engineer reviewing completed work.

## Task Being Reviewed
ID: %s
Type: %s
Title: %s
Description: %s

## Acceptance Criteria
`, task.ID, task.Type, task.Title, task.Description)

	for i, criterion := range task.AcceptanceCriteria {
		prompt += fmt.Sprintf("%d. %s\n", i+1, criterion)
	}

	if len(task.Artifacts) > 0 {
		prompt += "\n## Artifacts Produced\n"
		for _, artifact := range task.Artifacts {
			prompt += fmt.Sprintf("\n### %s (%s)\n", artifact.ID, artifact.Type)
			if artifact.Path != "" {
				prompt += fmt.Sprintf("Path: %s\n", artifact.Path)
			}
			if artifact.Content != "" {
				prompt += fmt.Sprintf("```\n%s\n```\n", artifact.Content)
			}
		}
	}

	prompt += fmt.Sprintf("\n## Retry Information\nAttempt: %d of %d\n", task.RetryCount+1, task.MaxRetries)

	prompt += `
## Instructions
Review the completed work against the acceptance criteria.

Respond in the following format:
VERDICT: APPROVED or REJECTED
REASON: <brief explanation>
FEEDBACK: <detailed feedback for the worker if rejected>

Be thorough but fair. Approve if the core requirements are met, even if minor improvements could be made.`

	return prompt
}
