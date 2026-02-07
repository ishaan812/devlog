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
