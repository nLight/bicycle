package swarm

import (
	"context"
	"fmt"
	"log"
	"strings"
)

// LLMVerifier implements Verifier using an LLM
type LLMVerifier struct {
	client LLMClient
	model  Model
}

// NewLLMVerifier creates a new LLM-based verifier
func NewLLMVerifier(client LLMClient, model Model) *LLMVerifier {
	return &LLMVerifier{
		client: client,
		model:  model,
	}
}

// Verify checks if a task was completed correctly
func (v *LLMVerifier) Verify(ctx context.Context, task *Task) (approved bool, feedback string, err error) {
	log.Printf("[Verifier] Reviewing task %s", task.ID)

	// Build verification prompt
	prompt := VerifierPrompt(task)

	// Create LLM request
	req := NewPromptBuilder().
		System("You are a senior engineer reviewing completed work. Be thorough but fair.").
		User(prompt).
		Build(v.model)

	req.MaxTokens = 2048

	// Call LLM
	resp, err := v.client.Complete(ctx, req)
	if err != nil {
		return false, "", fmt.Errorf("verification LLM call failed: %w", err)
	}

	// Parse response
	approved, feedback = v.parseVerdict(resp.Content)

	if approved {
		log.Printf("[Verifier] Task %s APPROVED: %s", task.ID, feedback)
	} else {
		log.Printf("[Verifier] Task %s REJECTED: %s", task.ID, feedback)
	}

	return approved, feedback, nil
}

// parseVerdict extracts the verdict from LLM response
func (v *LLMVerifier) parseVerdict(content string) (approved bool, feedback string) {
	lines := strings.Split(content, "\n")

	var reason, detailedFeedback string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(strings.ToUpper(line), "VERDICT:") {
			verdict := strings.TrimSpace(strings.TrimPrefix(line, "VERDICT:"))
			verdict = strings.TrimPrefix(verdict, "verdict:")
			approved = strings.Contains(strings.ToUpper(verdict), "APPROVED")
		} else if strings.HasPrefix(strings.ToUpper(line), "REASON:") {
			reason = strings.TrimSpace(strings.TrimPrefix(line, "REASON:"))
			reason = strings.TrimPrefix(reason, "reason:")
		} else if strings.HasPrefix(strings.ToUpper(line), "FEEDBACK:") {
			detailedFeedback = strings.TrimSpace(strings.TrimPrefix(line, "FEEDBACK:"))
			detailedFeedback = strings.TrimPrefix(detailedFeedback, "feedback:")
		}
	}

	// Combine reason and feedback
	if detailedFeedback != "" {
		feedback = detailedFeedback
	} else if reason != "" {
		feedback = reason
	} else {
		feedback = content // Use full content if parsing failed
	}

	return approved, feedback
}

// AutoVerifier always approves (for testing/simple tasks)
type AutoVerifier struct {
	AlwaysApprove bool
	Feedback      string
}

// Verify implements Verifier
func (v *AutoVerifier) Verify(ctx context.Context, task *Task) (approved bool, feedback string, err error) {
	if v.AlwaysApprove {
		return true, v.Feedback, nil
	}
	return false, v.Feedback, nil
}

// RuleBasedVerifier verifies based on simple rules
type RuleBasedVerifier struct {
	Rules []VerificationRule
}

// VerificationRule defines a verification check
type VerificationRule struct {
	Name        string
	Description string
	Check       func(task *Task) (passed bool, message string)
}

// NewRuleBasedVerifier creates a verifier with common rules
func NewRuleBasedVerifier() *RuleBasedVerifier {
	return &RuleBasedVerifier{
		Rules: []VerificationRule{
			{
				Name:        "has_artifacts",
				Description: "Task must produce at least one artifact",
				Check: func(task *Task) (bool, string) {
					if len(task.Artifacts) == 0 {
						return false, "No artifacts produced"
					}
					return true, "Artifacts present"
				},
			},
			{
				Name:        "non_empty_content",
				Description: "Artifacts must have content",
				Check: func(task *Task) (bool, string) {
					for _, a := range task.Artifacts {
						if a.Content == "" && a.Path == "" {
							return false, fmt.Sprintf("Artifact %s has no content", a.ID)
						}
					}
					return true, "All artifacts have content"
				},
			},
			{
				Name:        "reasonable_length",
				Description: "Response should be substantive",
				Check: func(task *Task) (bool, string) {
					for _, a := range task.Artifacts {
						if len(a.Content) < 50 {
							return false, "Response too short, may be incomplete"
						}
					}
					return true, "Response has reasonable length"
				},
			},
		},
	}
}

// Verify implements Verifier
func (v *RuleBasedVerifier) Verify(ctx context.Context, task *Task) (approved bool, feedback string, err error) {
	var failedRules []string
	var passedRules []string

	for _, rule := range v.Rules {
		passed, message := rule.Check(task)
		if !passed {
			failedRules = append(failedRules, fmt.Sprintf("%s: %s", rule.Name, message))
		} else {
			passedRules = append(passedRules, rule.Name)
		}
	}

	if len(failedRules) > 0 {
		return false, fmt.Sprintf("Failed checks: %s", strings.Join(failedRules, "; ")), nil
	}

	return true, fmt.Sprintf("All checks passed: %s", strings.Join(passedRules, ", ")), nil
}

// CompositeVerifier combines multiple verifiers
type CompositeVerifier struct {
	Verifiers     []Verifier
	RequireAll    bool // If true, all must approve. If false, any can approve.
}

// Verify implements Verifier
func (v *CompositeVerifier) Verify(ctx context.Context, task *Task) (approved bool, feedback string, err error) {
	var results []string

	for i, verifier := range v.Verifiers {
		a, f, e := verifier.Verify(ctx, task)
		if e != nil {
			return false, "", fmt.Errorf("verifier %d failed: %w", i, e)
		}

		status := "REJECTED"
		if a {
			status = "APPROVED"
		}
		results = append(results, fmt.Sprintf("Verifier %d: %s - %s", i, status, f))

		if v.RequireAll && !a {
			return false, strings.Join(results, "\n"), nil
		}
		if !v.RequireAll && a {
			return true, strings.Join(results, "\n"), nil
		}
	}

	// If RequireAll, we passed all. If !RequireAll, we failed all.
	return v.RequireAll, strings.Join(results, "\n"), nil
}
