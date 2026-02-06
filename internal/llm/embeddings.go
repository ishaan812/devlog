package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// EmbeddingClient generates vector embeddings for text
type EmbeddingClient interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
}

// OllamaEmbedder uses Ollama's embedding endpoint
type OllamaEmbedder struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &OllamaEmbedder{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{},
	}
}

type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
}

func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := ollamaEmbedRequest{
		Model:  e.model,
		Prompt: text,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/api/embeddings", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama embed error (status %d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result ollamaEmbedResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result.Embedding, nil
}

func (e *OllamaEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))
	for i, text := range texts {
		emb, err := e.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("failed to embed text %d: %w", i, err)
		}
		embeddings[i] = emb
	}
	return embeddings, nil
}

func (e *OllamaEmbedder) Dimensions() int {
	return 768 // nomic-embed-text default
}

// OpenAIEmbedder uses OpenAI's embedding endpoint
type OpenAIEmbedder struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewOpenAIEmbedder(baseURL, apiKey, model string) *OpenAIEmbedder {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAIEmbedder{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{},
	}
}

type openAIEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

func (e *OpenAIEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := openAIEmbedRequest{
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

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result openAIEmbedResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("OpenAI API error: %s", result.Error.Message)
	}

	embeddings := make([][]float32, len(texts))
	for _, d := range result.Data {
		embeddings[d.Index] = d.Embedding
	}

	return embeddings, nil
}

func (e *OpenAIEmbedder) Dimensions() int {
	if e.model == "text-embedding-3-large" {
		return 3072
	}
	return 1536 // text-embedding-3-small, ada-002
}

// NewEmbedder creates an embedding client based on config.
// Returns an error if the provider is unsupported or the embedding model is not configured.
func NewEmbedder(cfg Config) (EmbeddingClient, error) {
	embeddingModel := cfg.EmbeddingModel
	if embeddingModel == "" {
		return nil, fmt.Errorf("no embedding model specified for provider %q; run 'devlog onboard' to configure", cfg.Provider)
	}

	switch cfg.Provider {
	case ProviderOllama:
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		return NewOllamaEmbedder(baseURL, embeddingModel), nil

	case ProviderOpenAI:
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("OpenAI API key is required for embeddings")
		}
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		return NewOpenAIEmbedder(baseURL, cfg.APIKey, embeddingModel), nil

	case ProviderOpenRouter:
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("OpenRouter API key is required for embeddings")
		}
		return NewOpenRouterEmbedder(cfg.APIKey, embeddingModel), nil

	case ProviderVoyageAI:
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("Voyage AI API key is required for embeddings")
		}
		return NewVoyageAIEmbedder(cfg.APIKey, embeddingModel), nil

	default:
		return nil, fmt.Errorf("unsupported embedding provider: %q; supported providers: ollama, openai, openrouter, voyageai", cfg.Provider)
	}
}

// EmbeddingProviders returns providers that support embeddings
type EmbeddingProvider struct {
	Name   string
	Models []string
}

// AvailableEmbeddingProviders returns the list of embedding providers with their models
func AvailableEmbeddingProviders() []EmbeddingProvider {
	return []EmbeddingProvider{
		{
			Name: "ollama",
			Models: []string{
				"nomic-embed-text",
				"mxbai-embed-large",
				"all-minilm",
			},
		},
		{
			Name: "openai",
			Models: []string{
				"text-embedding-3-small",
				"text-embedding-3-large",
				"text-embedding-ada-002",
			},
		},
		{
			Name: "openrouter",
			Models: []string{
				"openai/text-embedding-3-small",
				"openai/text-embedding-3-large",
			},
		},
		{
			Name: "voyageai",
			Models: []string{
				"voyage-3.5",
				"voyage-3.5-lite",
				"voyage-3-large",
				"voyage-code-3",
				"voyage-finance-2",
				"voyage-law-2",
			},
		},
	}
}

// DefaultEmbeddingModel returns the default embedding model for a provider.
// Returns empty string for unsupported providers.
func DefaultEmbeddingModel(provider Provider) string {
	switch provider {
	case ProviderOllama:
		return "nomic-embed-text"
	case ProviderOpenAI:
		return "text-embedding-3-small"
	case ProviderOpenRouter:
		return "openai/text-embedding-3-small"
	case ProviderVoyageAI:
		return "voyage-3.5"
	default:
		return ""
	}
}
