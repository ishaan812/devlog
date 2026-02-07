package constants

import "strings"

// Provider represents an LLM provider type
type Provider string

// LLM Providers
const (
	ProviderOllama     Provider = "ollama"
	ProviderOpenAI     Provider = "openai"
	ProviderAnthropic  Provider = "anthropic"
	ProviderOpenRouter Provider = "openrouter"
	ProviderBedrock    Provider = "bedrock"
	ProviderGemini     Provider = "gemini"
	ProviderVoyageAI   Provider = "voyageai" // Embeddings only
)

// ProviderInfo contains display information about a provider
type ProviderInfo struct {
	Key                string
	Name               string
	Description        string
	SupportsLLM        bool
	SupportsEmbeddings bool
}

// AllProviders returns all available LLM providers in order
var AllProviders = []ProviderInfo{
	{
		Key:                "1",
		Name:               "Ollama",
		Description:        "Free, local, private — Llama 3.2, DeepSeek, Qwen3",
		SupportsLLM:        true,
		SupportsEmbeddings: true,
	},
	{
		Key:                "2",
		Name:               "Anthropic",
		Description:        "Claude Opus 4.6, Sonnet 4.5, Haiku 4.5",
		SupportsLLM:        true,
		SupportsEmbeddings: false, // Anthropic recommends Voyage AI
	},
	{
		Key:                "3",
		Name:               "OpenAI",
		Description:        "GPT-5.2, GPT-5.3 Codex, GPT-4o",
		SupportsLLM:        true,
		SupportsEmbeddings: true,
	},
	{
		Key:                "4",
		Name:               "OpenRouter",
		Description:        "Unified API — Gemini, Claude, GPT, Llama, DeepSeek & free models",
		SupportsLLM:        true,
		SupportsEmbeddings: true,
	},
	{
		Key:                "5",
		Name:               "Gemini",
		Description:        "Google Gemini — Flash, Pro, 1M context",
		SupportsLLM:        true,
		SupportsEmbeddings: true,
	},
	{
		Key:                "6",
		Name:               "Bedrock",
		Description:        "Claude via AWS (enterprise)",
		SupportsLLM:        true,
		SupportsEmbeddings: false,
	},
}

// EmbeddingProviderInfo contains display information about embedding providers
type EmbeddingProviderInfo struct {
	Key         string
	Name        string
	Description string
	Provider    Provider
}

// AllEmbeddingProviders returns all available embedding providers
var AllEmbeddingProviders = []EmbeddingProviderInfo{
	{
		Key:         "1",
		Name:        "Same as LLM provider",
		Description: "Use the same provider for embeddings (recommended)",
		Provider:    "", // Special case - will use LLM provider
	},
	{
		Key:         "2",
		Name:        "Ollama",
		Description: "Local embeddings (nomic-embed-text)",
		Provider:    ProviderOllama,
	},
	{
		Key:         "3",
		Name:        "OpenAI",
		Description: "OpenAI embeddings (text-embedding-3-small)",
		Provider:    ProviderOpenAI,
	},
	{
		Key:         "4",
		Name:        "OpenRouter",
		Description: "OpenRouter embeddings (openai/text-embedding-3-small)",
		Provider:    ProviderOpenRouter,
	},
	{
		Key:         "5",
		Name:        "Gemini",
		Description: "Google Gemini embeddings (gemini-embedding-001)",
		Provider:    ProviderGemini,
	},
	{
		Key:         "6",
		Name:        "Voyage AI",
		Description: "Voyage AI embeddings (voyage-3.5) - Recommended by Anthropic",
		Provider:    ProviderVoyageAI,
	},
}

// GetProviderByKey returns the provider for a given key
func GetProviderByKey(key string) Provider {
	for _, p := range AllProviders {
		if p.Key == key {
			return Provider(p.Name)
		}
	}
	return ""
}

// GetProviderInfo returns information about a provider
func GetProviderInfo(provider Provider) *ProviderInfo {
	providerName := string(provider)
	for _, p := range AllProviders {
		if strings.EqualFold(p.Name, providerName) {
			return &p
		}
	}
	return nil
}

// ProviderSupportsEmbeddings checks if a provider supports embeddings
func ProviderSupportsEmbeddings(provider Provider) bool {
	info := GetProviderInfo(provider)
	if info == nil {
		return false
	}
	return info.SupportsEmbeddings
}

// ProviderDescription returns a human-readable description of a provider
func ProviderDescription(provider Provider) string {
	switch provider {
	case ProviderOllama:
		return "Ollama (local, free)"
	case ProviderOpenAI:
		return "OpenAI (GPT-5.2, GPT-4o)"
	case ProviderAnthropic:
		return "Anthropic (Claude Opus/Sonnet)"
	case ProviderBedrock:
		return "AWS Bedrock (Claude via AWS)"
	case ProviderOpenRouter:
		return "OpenRouter (unified API, multiple models)"
	case ProviderGemini:
		return "Google Gemini (Flash, Pro, 1M context)"
	case ProviderVoyageAI:
		return "Voyage AI (embeddings specialist)"
	default:
		return string(provider)
	}
}

// ProviderSetupInfo holds setup/configuration metadata for a provider
type ProviderSetupInfo struct {
	APIKeyURL    string // Where to get the API key
	APIKeyPrefix string // Expected prefix for validation (e.g. "sk-ant-")
	Placeholder  string // Input placeholder text
	SetupHint    string // Help text shown during setup
	NeedsAPIKey  bool   // Whether this provider requires an API key
}

// GetProviderSetupInfo returns setup metadata for a provider
func GetProviderSetupInfo(provider Provider) ProviderSetupInfo {
	switch provider {
	case ProviderOllama:
		return ProviderSetupInfo{
			Placeholder: "http://localhost:11434",
			SetupHint:   "Ollama runs locally on your machine. Make sure it's running: ollama serve",
			NeedsAPIKey: false,
		}
	case ProviderAnthropic:
		return ProviderSetupInfo{
			APIKeyURL:    "https://console.anthropic.com/",
			APIKeyPrefix: "sk-ant-",
			Placeholder:  "sk-ant-...",
			SetupHint:    "Get your API key from: console.anthropic.com",
			NeedsAPIKey:  true,
		}
	case ProviderOpenAI:
		return ProviderSetupInfo{
			APIKeyURL:    "https://platform.openai.com/api-keys",
			APIKeyPrefix: "sk-",
			Placeholder:  "sk-...",
			SetupHint:    "Get your API key from: platform.openai.com/api-keys",
			NeedsAPIKey:  true,
		}
	case ProviderOpenRouter:
		return ProviderSetupInfo{
			APIKeyURL:    "https://openrouter.ai/keys",
			APIKeyPrefix: "sk-or-",
			Placeholder:  "sk-or-...",
			SetupHint:    "Get your API key from: openrouter.ai/keys",
			NeedsAPIKey:  true,
		}
	case ProviderBedrock:
		return ProviderSetupInfo{
			Placeholder: "AWS Access Key ID",
			SetupHint:   "AWS Bedrock requires IAM credentials with Bedrock access.",
			NeedsAPIKey: true,
		}
	case ProviderGemini:
		return ProviderSetupInfo{
			APIKeyURL:    "https://aistudio.google.com/apikey",
			APIKeyPrefix: "AI",
			Placeholder:  "AIza...",
			SetupHint:    "Get your API key from: aistudio.google.com/apikey",
			NeedsAPIKey:  true,
		}
	case ProviderVoyageAI:
		return ProviderSetupInfo{
			APIKeyURL:    "https://dash.voyageai.com/",
			APIKeyPrefix: "pa-",
			Placeholder:  "pa-...",
			SetupHint:    "Get your API key from: dash.voyageai.com",
			NeedsAPIKey:  true,
		}
	default:
		return ProviderSetupInfo{}
	}
}
