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

type OpenRouterClient struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewOpenRouterClient(apiKey, model string) *OpenRouterClient {
	return &OpenRouterClient{
		baseURL: "https://openrouter.ai/api/v1",
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{},
	}
}

type openRouterChatRequest struct {
	Model    string              `json:"model"`
	Messages []openRouterMessage `json:"messages"`
}

type openRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openRouterChatResponse struct {
	Choices []struct {
		Message openRouterMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

func (c *OpenRouterClient) Complete(ctx context.Context, prompt string) (string, error) {
	messages := []Message{
		{Role: "user", Content: prompt},
	}
	return c.ChatComplete(ctx, messages)
}

func (c *OpenRouterClient) ChatComplete(ctx context.Context, messages []Message) (string, error) {
	openRouterMessages := make([]openRouterMessage, len(messages))
	for i, m := range messages {
		openRouterMessages[i] = openRouterMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	reqBody := openRouterChatRequest{
		Model:    c.model,
		Messages: openRouterMessages,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("HTTP-Referer", "https://github.com/ishaan812/devlog") // Optional but recommended
	req.Header.Set("X-Title", "DevLog")                                   // Optional but recommended

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var result openRouterChatResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("OpenRouter API error: %s (code %d)", result.Error.Message, result.Error.Code)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

// OpenRouterEmbedder uses OpenRouter's embedding endpoint
type OpenRouterEmbedder struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewOpenRouterEmbedder(apiKey, model string) *OpenRouterEmbedder {
	return &OpenRouterEmbedder{
		baseURL: "https://openrouter.ai/api/v1",
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{},
	}
}

type openRouterEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openRouterEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

func (e *OpenRouterEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

func (e *OpenRouterEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := openRouterEmbedRequest{
		Model: e.model,
		Input: texts,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/embeddings", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("HTTP-Referer", "https://github.com/ishaan812/devlog")
	req.Header.Set("X-Title", "DevLog")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result openRouterEmbedResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("OpenRouter API error: %s (code %d)", result.Error.Message, result.Error.Code)
	}

	embeddings := make([][]float32, len(texts))
	for _, d := range result.Data {
		embeddings[d.Index] = d.Embedding
	}

	return embeddings, nil
}

func (e *OpenRouterEmbedder) Dimensions() int {
	// Most common embedding models
	if strings.Contains(e.model, "text-embedding-3-large") {
		return 3072
	}
	if strings.Contains(e.model, "text-embedding-3-small") {
		return 1536
	}
	// Default for most models
	return 1536
}
