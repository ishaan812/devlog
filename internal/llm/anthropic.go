package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type AnthropicClient struct {
	apiKey string
	model  string
	client *http.Client
}

func NewAnthropicClient(apiKey, model string) *AnthropicClient {
	if model == "" {
		model = "claude-sonnet-4-5-20250929"
	}
	return &AnthropicClient{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{},
	}
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
	System    string             `json:"system,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *AnthropicClient) Complete(ctx context.Context, prompt string) (string, error) {
	messages := []Message{
		{Role: "user", Content: prompt},
	}
	return c.ChatComplete(ctx, messages)
}

func (c *AnthropicClient) ChatComplete(ctx context.Context, messages []Message) (string, error) {
	var systemPrompt string
	var anthropicMessages []anthropicMessage

	for _, m := range messages {
		if m.Role == "system" {
			systemPrompt = m.Content
			continue
		}
		anthropicMessages = append(anthropicMessages, anthropicMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	reqBody := anthropicRequest{
		Model:     c.model,
		MaxTokens: 4096,
		Messages:  anthropicMessages,
		System:    systemPrompt,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Anthropic API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result anthropicResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("Anthropic API error: %s", result.Error.Message)
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	var text string
	for _, content := range result.Content {
		if content.Type == "text" {
			text += content.Text
		}
	}

	return strings.TrimSpace(text), nil
}
