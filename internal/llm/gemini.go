package llm

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// ── Gemini LLM Client ──────────────────────────────────────────────────────

// GeminiClient implements the Client interface using Google's official Gemini Go SDK.
type GeminiClient struct {
	client *genai.Client
	apiKey string
	model  string
}

func NewGeminiClient(baseURL, apiKey, model string) *GeminiClient {
	// Note: baseURL is ignored as the SDK handles endpoint configuration
	// The SDK will be initialized lazily on first use
	return &GeminiClient{
		client: nil, // Will be initialized in ensureClient
		apiKey: apiKey,
		model:  model,
	}
}

// ensureClient initializes the SDK client if not already initialized
func (c *GeminiClient) ensureClient(ctx context.Context) error {
	if c.client != nil {
		return nil
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend: genai.BackendGeminiAPI,
		APIKey:  c.apiKey,
	})
	if err != nil {
		return fmt.Errorf("failed to create Gemini client: %w", err)
	}

	c.client = client
	return nil
}

func (c *GeminiClient) Complete(ctx context.Context, prompt string) (string, error) {
	messages := []Message{
		{Role: "user", Content: prompt},
	}
	return c.ChatComplete(ctx, messages)
}

func (c *GeminiClient) ChatComplete(ctx context.Context, messages []Message) (string, error) {
	if err := c.ensureClient(ctx); err != nil {
		return "", err
	}

	// Convert messages to Gemini Content format
	var contents []*genai.Content
	var systemInstruction *genai.Content

	for _, m := range messages {
		role := m.Role
		switch role {
		case "system":
			// Gemini uses systemInstruction for system prompts
			systemInstruction = &genai.Content{
				Parts: []*genai.Part{
					genai.NewPartFromText(m.Content),
				},
			}
			continue
		case "assistant":
			role = "model"
		case "user":
			// keep as-is
		default:
			role = "user"
		}

		contents = append(contents, &genai.Content{
			Role: role,
			Parts: []*genai.Part{
				genai.NewPartFromText(m.Content),
			},
		})
	}

	if len(contents) == 0 {
		return "", fmt.Errorf("no user/assistant messages provided")
	}

	// Configure the generation with thinking enabled for pro models
	config := &genai.GenerateContentConfig{}
	if strings.Contains(c.model, "pro") {
		config.ThinkingConfig = &genai.ThinkingConfig{
			ThinkingLevel: "HIGH",
		}
	}

	// Set system instruction if present
	if systemInstruction != nil {
		config.SystemInstruction = systemInstruction
	}

	// Call the GenerateContent method
	result, err := c.client.Models.GenerateContent(ctx, c.model, contents, config)
	if err != nil {
		return "", fmt.Errorf("Gemini API error: %w", err)
	}

	// Extract text from result
	return result.Text(), nil
}

// ── Gemini Embedder ────────────────────────────────────────────────────────

// GeminiEmbedder implements EmbeddingClient using Google's official Gemini Go SDK.
type GeminiEmbedder struct {
	client *genai.Client
	apiKey string
	model  string
}

func NewGeminiEmbedder(baseURL, apiKey, model string) *GeminiEmbedder {
	// Note: baseURL is ignored as the SDK handles endpoint configuration
	return &GeminiEmbedder{
		client: nil, // Will be initialized lazily
		apiKey: apiKey,
		model:  model,
	}
}

// ensureClient initializes the SDK client if not already initialized
func (e *GeminiEmbedder) ensureClient(ctx context.Context) error {
	if e.client != nil {
		return nil
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend: genai.BackendGeminiAPI,
		APIKey:  e.apiKey,
	})
	if err != nil {
		return fmt.Errorf("failed to create Gemini client: %w", err)
	}

	e.client = client
	return nil
}

func (e *GeminiEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if err := e.ensureClient(ctx); err != nil {
		return nil, err
	}

	contents := []*genai.Content{
		genai.NewContentFromText(text, genai.RoleUser),
	}

	result, err := e.client.Models.EmbedContent(ctx, e.model, contents, nil)
	if err != nil {
		return nil, fmt.Errorf("Gemini embed error: %w", err)
	}

	if len(result.Embeddings) == 0 || result.Embeddings[0] == nil {
		return nil, fmt.Errorf("no embedding returned from Gemini")
	}

	return result.Embeddings[0].Values, nil
}

func (e *GeminiEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if err := e.ensureClient(ctx); err != nil {
		return nil, err
	}

	// Convert texts to Content objects
	contents := make([]*genai.Content, len(texts))
	for i, text := range texts {
		contents[i] = genai.NewContentFromText(text, genai.RoleUser)
	}

	// Process embeddings sequentially since SDK doesn't have a batch endpoint
	// Note: The SDK's EmbedContent can handle multiple contents, so we'll call it once
	result, err := e.client.Models.EmbedContent(ctx, e.model, contents, nil)
	if err != nil {
		return nil, fmt.Errorf("Gemini batch embed error: %w", err)
	}

	if len(result.Embeddings) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(result.Embeddings))
	}

	embeddings := make([][]float32, len(texts))
	for i, emb := range result.Embeddings {
		if emb == nil || len(emb.Values) == 0 {
			return nil, fmt.Errorf("no embedding for text %d", i)
		}
		embeddings[i] = emb.Values
	}

	return embeddings, nil
}

func (e *GeminiEmbedder) Dimensions() int {
	switch e.model {
	case "text-embedding-004":
		return 768
	default:
		// gemini-embedding-001 returns 3072-dimensional embeddings
		return 3072
	}
}
