package llm

import (
	"context"
	"fmt"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type LLMClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
	ChatComplete(ctx context.Context, messages []Message) (string, error)
}

type Provider string

const (
	ProviderOllama    Provider = "ollama"
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
	ProviderBedrock   Provider = "bedrock"
)

type Config struct {
	Provider Provider
	Model    string
	BaseURL  string
	APIKey   string

	// AWS Bedrock specific
	AWSRegion          string
	AWSAccessKeyID     string
	AWSSecretAccessKey string
}

func NewClient(cfg Config) (LLMClient, error) {
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

// AvailableProviders returns a list of all supported providers
func AvailableProviders() []Provider {
	return []Provider{
		ProviderOllama,
		ProviderOpenAI,
		ProviderAnthropic,
		ProviderBedrock,
	}
}

// ProviderDescription returns a human-readable description of a provider
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
