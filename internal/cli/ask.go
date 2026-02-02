package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/ishaan812/devlog/internal/chat"
	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/db"
	"github.com/ishaan812/devlog/internal/llm"
	"github.com/spf13/cobra"
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
	question := strings.Join(args, " ")

	// Load config
	cfg, err := config.Load()
	if err != nil {
		VerboseLog("Warning: failed to load config: %v", err)
		cfg = &config.Config{DefaultProvider: "ollama"}
	}

	// Initialize database
	VerboseLog("Initializing database")
	database, err := db.GetDB()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Check if we have any data
	var count int
	err = database.QueryRow("SELECT COUNT(*) FROM commits").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to query database: %w", err)
	}

	if count == 0 {
		fmt.Println("No commits found in database. Run 'devlog ingest' first to scan a repository.")
		return nil
	}

	// Determine provider
	selectedProvider := provider
	if selectedProvider == "" {
		selectedProvider = cfg.DefaultProvider
	}
	if selectedProvider == "" {
		selectedProvider = "ollama"
	}

	// Initialize LLM client
	VerboseLog("Initializing LLM client with provider: %s", selectedProvider)
	llmCfg := llm.Config{
		Provider: llm.Provider(selectedProvider),
		Model:    model,
		BaseURL:  baseURL,
	}

	// Set API keys from config
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
		if model == "" && cfg.OllamaModel != "" {
			llmCfg.Model = cfg.OllamaModel
		}
	}

	client, err := llm.NewClient(llmCfg)
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}

	// Create pipeline and ask question
	pipeline := chat.NewPipeline(client, database, IsVerbose())

	fmt.Println("Thinking...")
	ctx := context.Background()
	response, err := pipeline.Ask(ctx, question)
	if err != nil {
		return fmt.Errorf("failed to process question: %w", err)
	}

	fmt.Println()
	fmt.Println(response)

	return nil
}
