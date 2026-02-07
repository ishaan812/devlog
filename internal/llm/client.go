package llm

import (
	"context"
	"fmt"

	"github.com/ishaan812/devlog/internal/constants"
)

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Client defines the interface for LLM operations.
type Client interface {
	Complete(ctx context.Context, prompt string) (string, error)
	ChatComplete(ctx context.Context, messages []Message) (string, error)
}

// Provider represents an LLM provider type.
// This is an alias to the constants.Provider type for backwards compatibility
type Provider = constants.Provider

// Re-export provider constants for backwards compatibility
const (
	ProviderOllama     = constants.ProviderOllama
	ProviderOpenAI     = constants.ProviderOpenAI
	ProviderAnthropic  = constants.ProviderAnthropic
	ProviderBedrock    = constants.ProviderBedrock
	ProviderOpenRouter = constants.ProviderOpenRouter
	ProviderGemini     = constants.ProviderGemini
)

// Config holds configuration for creating an LLM client.
type Config struct {
	Provider           Provider
	Model              string
	BaseURL            string
	APIKey             string
	AWSRegion          string
	AWSAccessKeyID     string
	AWSSecretAccessKey string
}

// Option is a functional option for configuring LLM clients.
type Option func(*Config)

// WithModel sets the model.
func WithModel(model string) Option {
	return func(c *Config) { c.Model = model }
}

// WithBaseURL sets the base URL.
func WithBaseURL(url string) Option {
	return func(c *Config) { c.BaseURL = url }
}

// WithAPIKey sets the API key.
func WithAPIKey(key string) Option {
	return func(c *Config) { c.APIKey = key }
}

// WithAWSCredentials sets AWS credentials.
func WithAWSCredentials(accessKeyID, secretAccessKey, region string) Option {
	return func(c *Config) {
		c.AWSAccessKeyID = accessKeyID
		c.AWSSecretAccessKey = secretAccessKey
		c.AWSRegion = region
	}
}

func defaultConfig(provider Provider) *Config {
	cfg := &Config{Provider: provider}

	// Get defaults from constants package
	modelConfig, ok := constants.DefaultModels[provider]
	if ok {
		cfg.Model = modelConfig.LLMModel
		cfg.BaseURL = modelConfig.BaseURL
		cfg.AWSRegion = modelConfig.AWSRegion
	}

	return cfg
}

// NewOllamaClientWithOptions creates an Ollama client with options.
func NewOllamaClientWithOptions(opts ...Option) Client {
	cfg := defaultConfig(ProviderOllama)
	for _, opt := range opts {
		opt(cfg)
	}
	return NewOllamaClient(cfg.BaseURL, cfg.Model)
}

// NewOpenAIClientWithOptions creates an OpenAI client with options.
func NewOpenAIClientWithOptions(apiKey string, opts ...Option) (Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
	}
	cfg := defaultConfig(ProviderOpenAI)
	cfg.APIKey = apiKey
	for _, opt := range opts {
		opt(cfg)
	}
	return NewOpenAIClient(cfg.BaseURL, cfg.APIKey, cfg.Model), nil
}

// NewAnthropicClientWithOptions creates an Anthropic client with options.
func NewAnthropicClientWithOptions(apiKey string, opts ...Option) (Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("Anthropic API key is required")
	}
	cfg := defaultConfig(ProviderAnthropic)
	cfg.APIKey = apiKey
	for _, opt := range opts {
		opt(cfg)
	}
	return NewAnthropicClient(cfg.APIKey, cfg.Model), nil
}

// NewBedrockClientWithOptions creates a Bedrock client with options.
func NewBedrockClientWithOptions(accessKeyID, secretAccessKey string, opts ...Option) (Client, error) {
	if accessKeyID == "" || secretAccessKey == "" {
		return nil, fmt.Errorf("AWS credentials are required for Bedrock")
	}
	cfg := defaultConfig(ProviderBedrock)
	cfg.AWSAccessKeyID = accessKeyID
	cfg.AWSSecretAccessKey = secretAccessKey
	for _, opt := range opts {
		opt(cfg)
	}
	return NewBedrockClient(cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, cfg.AWSRegion, cfg.Model), nil
}

// NewClient creates an LLM client from config.
func NewClient(cfg Config) (Client, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("no model specified for provider %q; run 'devlog onboard' to configure", cfg.Provider)
	}
	switch cfg.Provider {
	case ProviderOllama:
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		return NewOllamaClient(baseURL, cfg.Model), nil
	case ProviderOpenAI:
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("OpenAI API key is required")
		}
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		return NewOpenAIClient(baseURL, cfg.APIKey, cfg.Model), nil
	case ProviderAnthropic:
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("Anthropic API key is required")
		}
		return NewAnthropicClient(cfg.APIKey, cfg.Model), nil
	case ProviderBedrock:
		if cfg.AWSAccessKeyID == "" || cfg.AWSSecretAccessKey == "" {
			return nil, fmt.Errorf("AWS credentials are required for Bedrock")
		}
		if cfg.AWSRegion == "" {
			return nil, fmt.Errorf("AWS region is required for Bedrock; run 'devlog onboard' to configure")
		}
		return NewBedrockClient(cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, cfg.AWSRegion, cfg.Model), nil
	case ProviderOpenRouter:
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("OpenRouter API key is required")
		}
		return NewOpenRouterClient(cfg.APIKey, cfg.Model), nil
	case ProviderGemini:
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("Gemini API key is required")
		}
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "https://generativelanguage.googleapis.com/v1beta"
		}
		return NewGeminiClient(baseURL, cfg.APIKey, cfg.Model), nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.Provider)
	}
}

// AvailableProviders returns supported LLM providers.
func AvailableProviders() []Provider {
	var providers []Provider
	for _, p := range constants.AllProviders {
		if p.SupportsLLM {
			providers = append(providers, Provider(p.Name))
		}
	}
	return providers
}

// ProviderDescription returns a description for a provider.
func ProviderDescription(p Provider) string {
	return constants.ProviderDescription(p)
}

// NewGeminiClientWithOptions creates a Gemini client with options.
func NewGeminiClientWithOptions(apiKey string, opts ...Option) (Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("Gemini API key is required")
	}
	cfg := defaultConfig(ProviderGemini)
	cfg.APIKey = apiKey
	for _, opt := range opts {
		opt(cfg)
	}
	return NewGeminiClient(cfg.BaseURL, cfg.APIKey, cfg.Model), nil
}

// NewOpenRouterClientWithOptions creates an OpenRouter client with options.
func NewOpenRouterClientWithOptions(apiKey string, opts ...Option) (Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("OpenRouter API key is required")
	}
	cfg := defaultConfig(ProviderOpenRouter)
	cfg.APIKey = apiKey
	for _, opt := range opts {
		opt(cfg)
	}
	return NewOpenRouterClient(cfg.APIKey, cfg.Model), nil
}
