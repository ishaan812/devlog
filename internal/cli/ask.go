package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ishaan812/devlog/internal/chat"
	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/db"
	"github.com/ishaan812/devlog/internal/llm"
)

var (
	provider string
	model    string
	baseURL  string
)

var askCmd = &cobra.Command{
	Use:   "ask [question]",
	Short: "Ask questions about your development activity",
	Long: `Query your git history using natural language.
The ask command uses an LLM to convert your question into a database query
and summarize the results.

Supported providers: ollama, openai, anthropic, bedrock

Examples:
  devlog ask "What did I work on this week?"
  devlog ask "Which files have I changed the most?"
  devlog ask --provider anthropic "Show me my recent bug fixes"`,
	Args: cobra.MinimumNArgs(1),
	RunE: runAsk,
}

func init() {
	rootCmd.AddCommand(askCmd)

	askCmd.Flags().StringVar(&provider, "provider", "", "LLM provider (ollama, openai, anthropic, bedrock)")
	askCmd.Flags().StringVar(&model, "model", "", "Model to use")
	askCmd.Flags().StringVar(&baseURL, "base-url", "", "Custom API base URL")
}

func runAsk(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	question := strings.Join(args, " ")

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w\n\nRun 'devlog onboard' to set up your configuration", err)
	}

	VerboseLog("Initializing database")
	dbRepo, err := db.GetRepository()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	results, err := dbRepo.ExecuteQuery(ctx, "SELECT COUNT(*) as cnt FROM commits")
	if err != nil {
		return fmt.Errorf("failed to query database: %w", err)
	}
	if len(results) == 0 || results[0]["cnt"] == int64(0) {
		fmt.Println("No commits found in database. Run 'devlog ingest' first to scan a repository.")
		return nil
	}

	selectedProvider := provider
	if selectedProvider == "" {
		selectedProvider = cfg.DefaultProvider
	}
	if selectedProvider == "" {
		return fmt.Errorf("no provider configured; run 'devlog onboard' first")
	}

	VerboseLog("Initializing LLM client with provider: %s", selectedProvider)
	selectedModel := model // from CLI flag
	if selectedModel == "" {
		selectedModel = cfg.DefaultModel
	}
	llmCfg := llm.Config{Provider: llm.Provider(selectedProvider), Model: selectedModel, BaseURL: baseURL}

	switch llmCfg.Provider {
	case llm.ProviderOpenAI:
		llmCfg.APIKey = cfg.GetAPIKey("openai")
		if llmCfg.APIKey == "" {
			return fmt.Errorf("OpenAI API key not configured. Run 'devlog onboard' or set OPENAI_API_KEY")
		}
	case llm.ProviderAnthropic:
		llmCfg.APIKey = cfg.GetAPIKey("anthropic")
		if llmCfg.APIKey == "" {
			return fmt.Errorf("Anthropic API key not configured. Run 'devlog onboard' or set ANTHROPIC_API_KEY")
		}
	case llm.ProviderOpenRouter:
		llmCfg.APIKey = cfg.GetAPIKey("openrouter")
		if llmCfg.APIKey == "" {
			return fmt.Errorf("OpenRouter API key not configured. Run 'devlog onboard' or set OPENROUTER_API_KEY")
		}
	case llm.ProviderGemini:
		llmCfg.APIKey = cfg.GetAPIKey("gemini")
		if llmCfg.APIKey == "" {
			return fmt.Errorf("Gemini API key not configured. Run 'devlog onboard' or set GEMINI_API_KEY")
		}
	case llm.ProviderBedrock:
		llmCfg.AWSAccessKeyID = cfg.AWSAccessKeyID
		llmCfg.AWSSecretAccessKey = cfg.AWSSecretAccessKey
		llmCfg.AWSRegion = cfg.AWSRegion
		if llmCfg.AWSAccessKeyID == "" {
			return fmt.Errorf("AWS credentials not configured. Run 'devlog onboard'")
		}
	case llm.ProviderOllama:
		if cfg.OllamaBaseURL != "" {
			llmCfg.BaseURL = cfg.OllamaBaseURL
		}
		// Ollama uses its own model field as override
		if selectedModel == "" && cfg.OllamaModel != "" {
			llmCfg.Model = cfg.OllamaModel
		}
	}

	client, err := llm.NewClient(llmCfg)
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}

	pipeline := chat.NewPipeline(client, dbRepo.DB(), IsVerbose())

	fmt.Println("Thinking...")
	response, err := pipeline.Ask(ctx, question)
	if err != nil {
		return fmt.Errorf("failed to process question: %w", err)
	}

	fmt.Println()
	fmt.Println(response)

	return nil
}
