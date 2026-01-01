package swarm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	anthropicAPIURL = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
)

// ModelMapping maps our model names to Anthropic model IDs
var ModelMapping = map[Model]string{
	ModelHaiku:  "claude-3-5-haiku-20241022",
	ModelSonnet: "claude-sonnet-4-20250514",
	ModelOpus:   "claude-opus-4-20250514",
}

// AnthropicClient implements LLMClient using the Anthropic API
type AnthropicClient struct {
	apiKey     string
	httpClient *http.Client
}

// NewAnthropicClient creates a new Anthropic API client
func NewAnthropicClient(apiKey string) *AnthropicClient {
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}

	return &AnthropicClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

// anthropicRequest is the API request format
type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Temperature float64            `json:"temperature,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicResponse is the API response format
type anthropicResponse struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Role         string `json:"role"`
	Content      []anthropicContent `json:"content"`
	Model        string `json:"model"`
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence,omitempty"`
	Usage        anthropicUsage `json:"usage"`
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// anthropicStreamEvent is a streaming event
type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index,omitempty"`
	Delta *struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
	} `json:"delta,omitempty"`
	Message *anthropicResponse `json:"message,omitempty"`
	Usage   *anthropicUsage    `json:"usage,omitempty"`
}

// Complete sends a request to the Anthropic API
func (c *AnthropicClient) Complete(ctx context.Context, req LLMRequest) (*LLMResponse, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	// Map model
	modelID, ok := ModelMapping[req.Model]
	if !ok {
		modelID = string(req.Model) // Use as-is if not mapped
	}

	// Build API request
	messages := make([]anthropicMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = anthropicMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	apiReq := anthropicRequest{
		Model:       modelID,
		MaxTokens:   maxTokens,
		System:      req.System,
		Messages:    messages,
		Temperature: req.Temperature,
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", anthropicAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract text content
	var content string
	for _, c := range apiResp.Content {
		if c.Type == "text" {
			content += c.Text
		}
	}

	return &LLMResponse{
		Content:      content,
		Model:        apiResp.Model,
		InputTokens:  apiResp.Usage.InputTokens,
		OutputTokens: apiResp.Usage.OutputTokens,
		StopReason:   apiResp.StopReason,
	}, nil
}

// Stream sends a streaming request to the Anthropic API
func (c *AnthropicClient) Stream(ctx context.Context, req LLMRequest, handler func(chunk string) error) (*LLMResponse, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	// Map model
	modelID, ok := ModelMapping[req.Model]
	if !ok {
		modelID = string(req.Model)
	}

	// Build API request
	messages := make([]anthropicMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = anthropicMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	apiReq := anthropicRequest{
		Model:       modelID,
		MaxTokens:   maxTokens,
		System:      req.System,
		Messages:    messages,
		Temperature: req.Temperature,
		Stream:      true,
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", anthropicAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	// Process SSE stream
	var fullContent string
	var finalResponse *LLMResponse

	decoder := json.NewDecoder(resp.Body)
	for {
		// Read "event: " line
		var line string
		if _, err := fmt.Fscanln(resp.Body, &line); err != nil {
			if err == io.EOF {
				break
			}
			// Try reading JSON directly for simpler parsing
		}

		var event anthropicStreamEvent
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				break
			}
			continue
		}

		switch event.Type {
		case "content_block_delta":
			if event.Delta != nil && event.Delta.Text != "" {
				fullContent += event.Delta.Text
				if err := handler(event.Delta.Text); err != nil {
					return nil, err
				}
			}
		case "message_stop":
			// End of stream
		case "message_delta":
			if event.Usage != nil {
				finalResponse = &LLMResponse{
					Content:      fullContent,
					OutputTokens: event.Usage.OutputTokens,
				}
			}
		}
	}

	if finalResponse == nil {
		finalResponse = &LLMResponse{
			Content: fullContent,
		}
	}

	return finalResponse, nil
}
