package swarm

import (
	"context"
	"testing"
)

func TestLLMVerifier_Verify(t *testing.T) {
	client := NewMockLLMClient()
	client.DefaultResp = "VERDICT: APPROVED\nREASON: All acceptance criteria met\nFEEDBACK: Good implementation"

	verifier := NewLLMVerifier(client, ModelOpus)
	ctx := context.Background()

	task := &Task{
		ID:    "task-1",
		Title: "Test Task",
		Artifacts: []Artifact{
			{ID: "art-1", Type: "code", Content: "func test() {}"},
		},
	}

	approved, feedback, err := verifier.Verify(ctx, task)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if !approved {
		t.Error("Should be approved")
	}
	if feedback == "" {
		t.Error("Should have feedback")
	}
}

func TestLLMVerifier_VerifyRejected(t *testing.T) {
	client := NewMockLLMClient()
	client.DefaultResp = "VERDICT: REJECTED\nREASON: Missing tests\nFEEDBACK: Please add unit tests for the implementation"

	verifier := NewLLMVerifier(client, ModelOpus)
	ctx := context.Background()

	task := &Task{
		ID:    "task-1",
		Title: "Test Task",
		Artifacts: []Artifact{
			{ID: "art-1", Type: "code", Content: "func test() {}"},
		},
	}

	approved, feedback, err := verifier.Verify(ctx, task)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if approved {
		t.Error("Should be rejected")
	}
	if feedback == "" {
		t.Error("Should have feedback")
	}
}

func TestAutoVerifier_AlwaysApprove(t *testing.T) {
	verifier := &AutoVerifier{
		AlwaysApprove: true,
		Feedback:      "Auto-approved",
	}

	ctx := context.Background()
	task := &Task{ID: "task-1"}

	approved, feedback, err := verifier.Verify(ctx, task)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if !approved {
		t.Error("Should be approved")
	}
	if feedback != "Auto-approved" {
		t.Errorf("Expected 'Auto-approved', got '%s'", feedback)
	}
}

func TestAutoVerifier_AlwaysReject(t *testing.T) {
	verifier := &AutoVerifier{
		AlwaysApprove: false,
		Feedback:      "Auto-rejected",
	}

	ctx := context.Background()
	task := &Task{ID: "task-1"}

	approved, _, err := verifier.Verify(ctx, task)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if approved {
		t.Error("Should be rejected")
	}
}

func TestRuleBasedVerifier_Pass(t *testing.T) {
	verifier := NewRuleBasedVerifier()
	ctx := context.Background()

	task := &Task{
		ID: "task-1",
		Artifacts: []Artifact{
			{
				ID:      "art-1",
				Type:    "code",
				Content: "This is a substantive implementation with enough content to pass the length check.",
			},
		},
	}

	approved, _, err := verifier.Verify(ctx, task)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if !approved {
		t.Error("Should pass all rules")
	}
}

func TestRuleBasedVerifier_NoArtifacts(t *testing.T) {
	verifier := NewRuleBasedVerifier()
	ctx := context.Background()

	task := &Task{
		ID:        "task-1",
		Artifacts: []Artifact{},
	}

	approved, feedback, err := verifier.Verify(ctx, task)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if approved {
		t.Error("Should fail - no artifacts")
	}
	if feedback == "" {
		t.Error("Should have feedback about missing artifacts")
	}
}

func TestRuleBasedVerifier_ShortContent(t *testing.T) {
	verifier := NewRuleBasedVerifier()
	ctx := context.Background()

	task := &Task{
		ID: "task-1",
		Artifacts: []Artifact{
			{ID: "art-1", Type: "code", Content: "short"},
		},
	}

	approved, _, err := verifier.Verify(ctx, task)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if approved {
		t.Error("Should fail - content too short")
	}
}

func TestCompositeVerifier_RequireAll(t *testing.T) {
	verifier := &CompositeVerifier{
		Verifiers: []Verifier{
			&AutoVerifier{AlwaysApprove: true, Feedback: "V1"},
			&AutoVerifier{AlwaysApprove: true, Feedback: "V2"},
		},
		RequireAll: true,
	}

	ctx := context.Background()
	task := &Task{ID: "task-1"}

	approved, _, err := verifier.Verify(ctx, task)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if !approved {
		t.Error("Should pass when all approve")
	}
}

func TestCompositeVerifier_RequireAllOneFails(t *testing.T) {
	verifier := &CompositeVerifier{
		Verifiers: []Verifier{
			&AutoVerifier{AlwaysApprove: true, Feedback: "V1"},
			&AutoVerifier{AlwaysApprove: false, Feedback: "V2"},
		},
		RequireAll: true,
	}

	ctx := context.Background()
	task := &Task{ID: "task-1"}

	approved, _, err := verifier.Verify(ctx, task)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if approved {
		t.Error("Should fail when one rejects")
	}
}

func TestCompositeVerifier_RequireAny(t *testing.T) {
	verifier := &CompositeVerifier{
		Verifiers: []Verifier{
			&AutoVerifier{AlwaysApprove: false, Feedback: "V1"},
			&AutoVerifier{AlwaysApprove: true, Feedback: "V2"},
		},
		RequireAll: false,
	}

	ctx := context.Background()
	task := &Task{ID: "task-1"}

	approved, _, err := verifier.Verify(ctx, task)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if !approved {
		t.Error("Should pass when any approves")
	}
}

func TestLLMVerifier_ParseVerdict(t *testing.T) {
	verifier := &LLMVerifier{}

	tests := []struct {
		content  string
		approved bool
		hasFeedback bool
	}{
		{
			content:  "VERDICT: APPROVED\nREASON: Good work",
			approved: true,
			hasFeedback: true,
		},
		{
			content:  "VERDICT: REJECTED\nREASON: Needs work\nFEEDBACK: Add tests",
			approved: false,
			hasFeedback: true,
		},
		{
			content:  "verdict: approved\nreason: ok",
			approved: true,
			hasFeedback: true,
		},
		{
			content:  "This is just random text without verdict",
			approved: false,
			hasFeedback: true, // Uses full content as feedback
		},
	}

	for _, tt := range tests {
		approved, feedback := verifier.parseVerdict(tt.content)
		if approved != tt.approved {
			t.Errorf("parseVerdict(%q) approved = %v, want %v", tt.content, approved, tt.approved)
		}
		if tt.hasFeedback && feedback == "" {
			t.Errorf("parseVerdict(%q) should have feedback", tt.content)
		}
	}
}
