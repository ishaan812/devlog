package llm

import (
	"context"
	"fmt"
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
type Provider string

const (
	ProviderOllama    Provider = "ollama"
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
	ProviderBedrock   Provider = "bedrock"
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
	switch provider {
	case ProviderOllama:
		cfg.BaseURL = "http://localhost:11434"
		cfg.Model = "llama3.2"
	case ProviderOpenAI:
		cfg.BaseURL = "https://api.openai.com/v1"
		cfg.Model = "gpt-4o-mini"
	case ProviderAnthropic:
		cfg.Model = "claude-sonnet-4-5-20250929"
	case ProviderBedrock:
		cfg.AWSRegion = "us-east-1"
		cfg.Model = "anthropic.claude-sonnet-4-5-20250929-v1:0"
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
	switch cfg.Provider {
	case ProviderOllama:
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		model := cfg.Model
		if model == "" {
			model = "llama3.2"
		}
		return NewOllamaClient(baseURL, model), nil
	case ProviderOpenAI:
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("OpenAI API key is required")
		}
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		model := cfg.Model
		if model == "" {
			model = "gpt-4o-mini"
		}
		return NewOpenAIClient(baseURL, cfg.APIKey, model), nil
	case ProviderAnthropic:
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("Anthropic API key is required")
		}
		model := cfg.Model
		if model == "" {
			model = "claude-sonnet-4-5-20250929"
		}
		return NewAnthropicClient(cfg.APIKey, model), nil
	case ProviderBedrock:
		if cfg.AWSAccessKeyID == "" || cfg.AWSSecretAccessKey == "" {
			return nil, fmt.Errorf("AWS credentials are required for Bedrock")
		}
		region := cfg.AWSRegion
		if region == "" {
			region = "us-east-1"
		}
		model := cfg.Model
		if model == "" {
			model = "anthropic.claude-sonnet-4-5-20250929-v1:0"
		}
		return NewBedrockClient(cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, region, model), nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.Provider)
	}
}

// AvailableProviders returns supported providers.
func AvailableProviders() []Provider {
	return []Provider{ProviderOllama, ProviderOpenAI, ProviderAnthropic, ProviderBedrock}
}

// ProviderDescription returns a description for a provider.
func ProviderDescription(p Provider) string {
	switch p {
	case ProviderOllama:
		return "Ollama (local, free)"
	case ProviderOpenAI:
		return "OpenAI (GPT-4, GPT-3.5)"
	case ProviderAnthropic:
		return "Anthropic (Claude)"
	case ProviderBedrock:
		return "AWS Bedrock (Claude via AWS)"
	default:
		return string(p)
	}
}
