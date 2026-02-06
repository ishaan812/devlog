package constants

// ModelConfig holds model configuration for a provider
type ModelConfig struct {
	LLMModel       string
	EmbeddingModel string
	BaseURL        string
	AWSRegion      string
}

// DefaultModels contains default model configurations for each provider
var DefaultModels = map[Provider]ModelConfig{
	ProviderOllama: {
		LLMModel:       "llama3.1",
		EmbeddingModel: "nomic-embed-text",
		BaseURL:        "http://localhost:11434",
	},
	ProviderOpenAI: {
		LLMModel:       "gpt-5.2",
		EmbeddingModel: "text-embedding-3-small",
		BaseURL:        "https://api.openai.com/v1",
	},
	ProviderAnthropic: {
		LLMModel:       "claude-opus-4-6-20260205",
		EmbeddingModel: "", // Anthropic doesn't provide embeddings
	},
	ProviderOpenRouter: {
		LLMModel:       "openrouter/free",
		EmbeddingModel: "openai/text-embedding-3-small",
		BaseURL:        "https://openrouter.ai/api/v1",
	},
	ProviderBedrock: {
		LLMModel:       "anthropic.claude-opus-4-6-20260205-v1:0",
		EmbeddingModel: "",
		AWSRegion:      "us-east-1",
	},
	ProviderVoyageAI: {
		LLMModel:       "", // Voyage AI is embeddings only
		EmbeddingModel: "voyage-3.5",
		BaseURL:        "https://api.voyageai.com/v1",
	},
}

// ModelOption represents a selectable model with metadata for TUI/CLI display
type ModelOption struct {
	ID          string // Short key like "1", "2", etc.
	Model       string // The actual model identifier
	Description string // Human-readable description
}

// GetLLMModels returns available LLM model options for a provider
func GetLLMModels(provider Provider) []ModelOption {
	models, ok := llmModels[provider]
	if !ok {
		return nil
	}
	return models
}

// GetEmbeddingModels returns available embedding model options for a provider
func GetEmbeddingModels(provider Provider) []ModelOption {
	models, ok := embeddingModels[provider]
	if !ok {
		return nil
	}
	return models
}

// llmModels contains selectable LLM models per provider
var llmModels = map[Provider][]ModelOption{
	ProviderOllama: {
		{ID: "1", Model: "llama3.1", Description: "Meta Llama 3.1 (default, recommended)"},
		{ID: "2", Model: "deepseek-r1", Description: "DeepSeek R1 (strong reasoning)"},
		{ID: "3", Model: "qwen3", Description: "Qwen3 (dense & MoE)"},
		{ID: "4", Model: "gemma3", Description: "Gemma 3 (best single GPU)"},
		{ID: "5", Model: "phi-4", Description: "Phi-4 14B (Microsoft)"},
		{ID: "6", Model: "qwen3-coder-next", Description: "Qwen3 Coder (coding specialist)"},
		{ID: "7", Model: "llama3.2", Description: "Meta Llama 3.2 (lightweight)"},
		{ID: "8", Model: "kimi-k2.5", Description: "Kimi K2.5 (multimodal, agentic)"},
	},
	ProviderOpenAI: {
		{ID: "1", Model: "gpt-5.2", Description: "GPT-5.2 (latest flagship)"},
		{ID: "2", Model: "gpt-5.3-codex", Description: "GPT-5.3 Codex (best for coding)"},
		{ID: "3", Model: "gpt-4o", Description: "GPT-4o (fast, multimodal)"},
		{ID: "4", Model: "gpt-4o-mini", Description: "GPT-4o Mini (cheap, fast)"},
	},
	ProviderAnthropic: {
		{ID: "1", Model: "claude-opus-4-6-20260205", Description: "Claude Opus 4.6 (newest, most powerful)"},
		{ID: "2", Model: "claude-sonnet-4-5-20250929", Description: "Claude Sonnet 4.5 (balanced)"},
		{ID: "3", Model: "claude-haiku-4-5-20250929", Description: "Claude Haiku 4.5 (fast, cheap)"},
		{ID: "4", Model: "claude-3-5-sonnet-20241022", Description: "Claude 3.5 Sonnet (legacy)"},
	},
	ProviderOpenRouter: {
		{ID: "1", Model: "openrouter/free", Description: "Auto-routing to best free model (recommended)"},
		{ID: "2", Model: "google/gemini-3-flash-preview:free", Description: "Google Gemini 3 Flash (free, 1M context)"},
		{ID: "3", Model: "google/gemini-2.5-flash:free", Description: "Google Gemini 2.5 Flash (free, fast)"},
		{ID: "4", Model: "meta-llama/llama-3.3-70b-instruct:free", Description: "Meta Llama 3.3 70B (free, GPT-4 level)"},
		{ID: "5", Model: "anthropic/claude-opus-4-6-20260205", Description: "Claude Opus 4.6 (paid, newest & best)"},
		{ID: "6", Model: "anthropic/claude-sonnet-4-5", Description: "Claude Sonnet 4.5 (paid, balanced)"},
		{ID: "7", Model: "openai/gpt-5.3-codex", Description: "GPT-5.3 Codex (paid, best for coding)"},
		{ID: "8", Model: "openai/gpt-5.2", Description: "GPT-5.2 (paid, general purpose)"},
		{ID: "9", Model: "deepseek/deepseek-v3.2", Description: "DeepSeek V3.2 (very cheap, ~90% GPT-5)"},
		{ID: "10", Model: "moonshot/kimi-k2.5-0127", Description: "Kimi K2.5 (multimodal, agentic)"},
	},
	ProviderBedrock: {
		{ID: "1", Model: "anthropic.claude-sonnet-4-5-20250929-v1:0", Description: "Claude Sonnet 4.5 (balanced)"},
		{ID: "2", Model: "anthropic.claude-opus-4-6-20260205-v1:0", Description: "Claude Opus 4.6 (most powerful)"},
		{ID: "3", Model: "anthropic.claude-haiku-4-5-20250929-v1:0", Description: "Claude Haiku 4.5 (fast, cheap)"},
	},
}

// embeddingModels contains selectable embedding models per provider
var embeddingModels = map[Provider][]ModelOption{
	ProviderOllama: {
		{ID: "1", Model: "nomic-embed-text", Description: "Nomic Embed (default, recommended)"},
		{ID: "2", Model: "qwen3-embedding", Description: "Qwen3 Embedding (latest)"},
		{ID: "3", Model: "mxbai-embed-large", Description: "MxBai Embed Large"},
	},
	ProviderOpenAI: {
		{ID: "1", Model: "text-embedding-3-small", Description: "Ada v3 Small (default, cheap)"},
		{ID: "2", Model: "text-embedding-3-large", Description: "Ada v3 Large (higher quality)"},
		{ID: "3", Model: "text-embedding-ada-002", Description: "Ada v2 (legacy)"},
	},
	ProviderOpenRouter: {
		{ID: "1", Model: "openai/text-embedding-3-small", Description: "OpenAI Ada v3 Small (default)"},
		{ID: "2", Model: "openai/text-embedding-3-large", Description: "OpenAI Ada v3 Large"},
	},
	ProviderVoyageAI: {
		{ID: "1", Model: "voyage-3.5", Description: "Voyage 3.5 (default, recommended)"},
		{ID: "2", Model: "voyage-3.5-lite", Description: "Voyage 3.5 Lite (fast, cheap)"},
		{ID: "3", Model: "voyage-3-large", Description: "Voyage 3 Large (highest quality)"},
		{ID: "4", Model: "voyage-code-3", Description: "Voyage Code 3 (code specialist)"},
		{ID: "5", Model: "voyage-finance-2", Description: "Voyage Finance 2 (finance)"},
		{ID: "6", Model: "voyage-law-2", Description: "Voyage Law 2 (legal)"},
	},
}

// GetDefaultModel returns the default LLM model for a provider
func GetDefaultModel(provider Provider) string {
	if config, ok := DefaultModels[provider]; ok {
		return config.LLMModel
	}
	return ""
}

// GetDefaultEmbeddingModel returns the default embedding model for a provider.
func GetDefaultEmbeddingModel(provider Provider) string {
	if config, ok := DefaultModels[provider]; ok {
		return config.EmbeddingModel
	}
	return ""
}

// GetDefaultBaseURL returns the default base URL for a provider
func GetDefaultBaseURL(provider Provider) string {
	if config, ok := DefaultModels[provider]; ok {
		return config.BaseURL
	}
	return ""
}

// GetDefaultAWSRegion returns the default AWS region for Bedrock
func GetDefaultAWSRegion() string {
	return "us-east-1"
}

// ProviderHasModelSelection returns whether a provider should show a model picker
func ProviderHasModelSelection(provider Provider) bool {
	models := GetLLMModels(provider)
	return len(models) > 1
}
