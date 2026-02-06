package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// VoyageAIEmbedder uses Voyage AI's embedding endpoint
// Voyage AI is recommended by Anthropic for embeddings
type VoyageAIEmbedder struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewVoyageAIEmbedder(apiKey, model string) *VoyageAIEmbedder {
	return &VoyageAIEmbedder{
		baseURL: "https://api.voyageai.com/v1",
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{},
	}
}

type voyageAIEmbedRequest struct {
	Model     string   `json:"model"`
	Input     []string `json:"input"`
	InputType string   `json:"input_type,omitempty"` // "document" or "query"
}

type voyageAIEmbedResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
		Object    string    `json:"object"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (e *VoyageAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

func (e *VoyageAIEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := voyageAIEmbedRequest{
		Model:     e.model,
		Input:     texts,
		InputType: "document", // Default to document mode
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

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result voyageAIEmbedResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("Voyage AI API error: %s (%s)", result.Error.Message, result.Error.Type)
	}

	embeddings := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index >= 0 && d.Index < len(embeddings) {
			embeddings[d.Index] = d.Embedding
		}
	}

	return embeddings, nil
}

func (e *VoyageAIEmbedder) Dimensions() int {
	// Most Voyage models support multiple dimensions
	// Default to 1024 for Voyage 3.x models
	return 1024
}
