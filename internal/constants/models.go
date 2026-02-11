package constants

type ModelConfig struct {
	LLMModel  string
	BaseURL   string
	AWSRegion string
}

var DefaultModels = map[Provider]ModelConfig{
	ProviderOllama: {
		LLMModel: "llama3.1",
		BaseURL:  "http://localhost:11434",
	},
	ProviderOpenAI: {
		LLMModel: "gpt-5.2",
		BaseURL:  "https://api.openai.com/v1",
	},
	ProviderChatGPT: {
		LLMModel: "gpt-5.2",
		BaseURL:  "https://api.openai.com/v1",
	},
	ProviderAnthropic: {
		LLMModel: "claude-opus-4-6-20260205",
	},
	ProviderOpenRouter: {
		LLMModel: "openrouter/free",
		BaseURL:  "https://openrouter.ai/api/v1",
	},
	ProviderGemini: {
		LLMModel: "gemini-3-flash-preview",
		BaseURL:  "",
	},
	ProviderBedrock: {
		LLMModel:  "anthropic.claude-opus-4-6-20260205-v1:0",
		AWSRegion: "us-east-1",
	},
}

type ModelOption struct {
	ID          string
	Model       string
	Description string
}

func GetLLMModels(provider Provider) []ModelOption {
	models, ok := llmModels[provider]
	if !ok {
		return nil
	}
	return models
}

var llmModels = map[Provider][]ModelOption{
	ProviderOllama: {
		{ID: "1", Model: "llama3.2", Description: "Meta Llama 3.2 (default, recommended) "},
		{ID: "2", Model: "deepseek-r1", Description: "DeepSeek R1 (strong reasoning)"},
		{ID: "3", Model: "qwen3", Description: "Qwen3 (dense & MoE)"},
		{ID: "4", Model: "gemma3", Description: "Gemma 3 (best single GPU)"},
		{ID: "5", Model: "phi-4", Description: "Phi-4 14B (Microsoft)"},
		{ID: "6", Model: "qwen3-coder-next", Description: "Qwen3 Coder (coding specialist)"},
		{ID: "7", Model: "llama3.1", Description: "Meta Llama 3.1 (lightweight) "},
		{ID: "8", Model: "kimi-k2.5", Description: "Kimi K2.5 (multimodal, agentic)"},
	},
	ProviderOpenAI: {
		{ID: "1", Model: "gpt-5.2", Description: "GPT-5.2 (latest flagship)"},
		{ID: "2", Model: "gpt-5.3-codex", Description: "GPT-5.3 Codex (best for coding)"},
		{ID: "3", Model: "gpt-4o", Description: "GPT-4o (fast, multimodal)"},
		{ID: "4", Model: "gpt-4o-mini", Description: "GPT-4o Mini (cheap, fast)"},
	},
	ProviderChatGPT: {
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
	ProviderGemini: {
		{ID: "1", Model: "gemini-3-flash-preview", Description: "Gemini 3 Flash (default, fast & balanced)"},
		{ID: "2", Model: "gemini-3-pro-preview", Description: "Gemini 3 Pro (advanced reasoning with thinking)"},
		{ID: "3", Model: "gemini-2.5-flash", Description: "Gemini 2.5 Flash (stable)"},
		{ID: "4", Model: "gemini-2.5-pro", Description: "Gemini 2.5 Pro (stable, coding)"},
		{ID: "5", Model: "gemini-2.0-flash", Description: "Gemini 2.0 Flash (legacy, multimodal)"},
	},
	ProviderBedrock: {
		{ID: "1", Model: "anthropic.claude-sonnet-4-5-20250929-v1:0", Description: "Claude Sonnet 4.5 (balanced)"},
		{ID: "2", Model: "anthropic.claude-opus-4-6-20260205-v1:0", Description: "Claude Opus 4.6 (most powerful)"},
		{ID: "3", Model: "anthropic.claude-haiku-4-5-20250929-v1:0", Description: "Claude Haiku 4.5 (fast, cheap)"},
	},
}

func GetDefaultModel(provider Provider) string {
	if config, ok := DefaultModels[provider]; ok {
		return config.LLMModel
	}
	return ""
}

func GetDefaultBaseURL(provider Provider) string {
	if config, ok := DefaultModels[provider]; ok {
		return config.BaseURL
	}
	return ""
}

func GetDefaultAWSRegion() string {
	return "us-east-1"
}

func ProviderHasModelSelection(provider Provider) bool {
	models := GetLLMModels(provider)
	return len(models) > 1
}
