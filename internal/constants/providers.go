package constants

import "strings"

type Provider string

const (
	ProviderOllama     Provider = "ollama"
	ProviderOpenAI     Provider = "openai"
	ProviderChatGPT    Provider = "chatgpt"
	ProviderAnthropic  Provider = "anthropic"
	ProviderOpenRouter Provider = "openrouter"
	ProviderBedrock    Provider = "bedrock"
	ProviderGemini     Provider = "gemini"
)

type ProviderInfo struct {
	Key         string
	Name        string
	Description string
	SupportsLLM bool
}

var AllProviders = []ProviderInfo{
	{
		Key:         "1",
		Name:        "Ollama",
		Description: "Free, local, private — Llama 3.2, DeepSeek, Qwen3",
		SupportsLLM: true,
	},
	{
		Key:         "2",
		Name:        "Anthropic",
		Description: "Claude Opus 4.6, Sonnet 4.5, Haiku 4.5",
		SupportsLLM: true,
	},
	{
		Key:         "3",
		Name:        "OpenAI",
		Description: "GPT-5.2, GPT-5.3 Codex, GPT-4o",
		SupportsLLM: true,
	},
	{
		Key:         "4",
		Name:        "ChatGPT",
		Description: "OpenAI via browser login — requires Plus/Pro/Team/Enterprise plan",
		SupportsLLM: true,
	},
	{
		Key:         "5",
		Name:        "OpenRouter",
		Description: "Unified API — Gemini, Claude, GPT, Llama, DeepSeek & free models",
		SupportsLLM: true,
	},
	{
		Key:         "6",
		Name:        "Gemini",
		Description: "Google Gemini — Flash, Pro, 1M context",
		SupportsLLM: true,
	},
	{
		Key:         "7",
		Name:        "Bedrock",
		Description: "Claude via AWS (enterprise)",
		SupportsLLM: true,
	},
}

func GetProviderByKey(key string) Provider {
	for _, p := range AllProviders {
		if p.Key == key {
			return Provider(p.Name)
		}
	}
	return ""
}

func GetProviderInfo(provider Provider) *ProviderInfo {
	providerName := string(provider)
	for _, p := range AllProviders {
		if strings.EqualFold(p.Name, providerName) {
			return &p
		}
	}
	return nil
}

func ProviderDescription(provider Provider) string {
	switch provider {
	case ProviderOllama:
		return "Ollama (local, free)"
	case ProviderOpenAI:
		return "OpenAI (GPT-5.2, GPT-4o)"
	case ProviderChatGPT:
		return "ChatGPT (web login, GPT-5.2, GPT-4o)"
	case ProviderAnthropic:
		return "Anthropic (Claude Opus/Sonnet)"
	case ProviderBedrock:
		return "AWS Bedrock (Claude via AWS)"
	case ProviderOpenRouter:
		return "OpenRouter (unified API, multiple models)"
	case ProviderGemini:
		return "Google Gemini (Flash, Pro, 1M context)"
	default:
		return string(provider)
	}
}

type ProviderSetupInfo struct {
	APIKeyURL    string
	APIKeyPrefix string
	Placeholder  string
	SetupHint    string
	NeedsAPIKey  bool
	WebLogin     bool
}

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
	case ProviderChatGPT:
		return ProviderSetupInfo{
			APIKeyURL:    "https://platform.openai.com/api-keys",
			APIKeyPrefix: "sk-",
			Placeholder:  "sk-...",
			SetupHint:    "Your browser will open to platform.openai.com — create or copy an API key",
			NeedsAPIKey:  true,
			WebLogin:     true,
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
	default:
		return ProviderSetupInfo{}
	}
}
