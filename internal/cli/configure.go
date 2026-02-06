package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/constants"
	"github.com/ishaan812/devlog/internal/tui"
)

var (
	configureLegacy bool
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Configure DevLog settings",
	Long: `Configure your DevLog settings including LLM provider, embeddings, and user information.

This command opens an interactive configuration wizard that allows you to update:
  - LLM provider and API keys
  - Embedding provider and models
  - User information (name, email, GitHub username)

Examples:
  devlog configure              # Interactive TUI configuration
  devlog configure --legacy     # Text-based configuration`,
	RunE: runConfigure,
}

func init() {
	rootCmd.AddCommand(configureCmd)
	configureCmd.Flags().BoolVar(&configureLegacy, "legacy", false, "Use legacy text-based configuration")
}

func runConfigure(cmd *cobra.Command, args []string) error {
	// Load existing config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Ensure default profile exists
	if err := cfg.EnsureDefaultProfile(); err != nil {
		return fmt.Errorf("failed to ensure default profile: %w", err)
	}

	// Check if we should use the TUI (requires a terminal)
	if !configureLegacy && term.IsTerminal(int(os.Stdin.Fd())) {
		updatedCfg, err := tui.RunConfigure(cfg)
		if err != nil {
			// If TUI fails or is canceled, fall back to legacy
			if err.Error() == "configuration canceled" {
				fmt.Println("Configuration canceled.")
				return nil
			}
			// For other errors, try legacy mode
			fmt.Println("Falling back to text-based configuration...")
			return runConfigureLegacy(cfg)
		}
		_ = updatedCfg // Config is already saved by TUI
		return nil
	}

	return runConfigureLegacy(cfg)
}

func runConfigureLegacy(cfg *config.Config) error {
	reader := bufio.NewReader(os.Stdin)

	titleColor := color.New(color.FgHiCyan, color.Bold)
	promptColor := color.New(color.FgHiYellow)
	successColor := color.New(color.FgHiGreen)
	dimColor := color.New(color.FgHiBlack)
	infoColor := color.New(color.FgHiWhite)
	accentColor := color.New(color.FgHiMagenta)

	fmt.Println()
	titleColor.Println("DevLog Configuration")
	fmt.Println()

	// Show current configuration
	fmt.Println()
	titleColor.Println("Current Settings:")
	dimColor.Println(strings.Repeat("─", 40))
	infoColor.Printf("  LLM Provider: %s\n", cfg.DefaultProvider)
	if cfg.DefaultModel != "" {
		dimColor.Printf("  LLM Model: %s\n", cfg.DefaultModel)
	}
	if cfg.EmbeddingProvider != "" {
		infoColor.Printf("  Embedding Provider: %s\n", cfg.EmbeddingProvider)
	}
	if cfg.DefaultEmbedModel != "" {
		dimColor.Printf("  Embedding Model: %s\n", cfg.DefaultEmbedModel)
	}
	if cfg.GitHubUsername != "" {
		infoColor.Printf("  GitHub Username: %s\n", cfg.GitHubUsername)
	}
	if cfg.UserEmail != "" {
		dimColor.Printf("  Email: %s\n", cfg.UserEmail)
	}
	fmt.Println()

	// Configuration menu
	for {
		fmt.Println()
		titleColor.Println("What would you like to configure?")
		dimColor.Println(strings.Repeat("─", 40))
		fmt.Println()

		options := []struct {
			key  string
			name string
			desc string
		}{
			{"1", "LLM Provider", "Change language model provider and API key"},
			{"2", "Embedding Provider", "Change embedding provider and model"},
			{"3", "User Information", "Update name, email, GitHub username"},
			{"4", "API Keys", "Update API keys for providers"},
			{"5", "View Settings", "Display current configuration"},
			{"6", "Save & Exit", "Save changes and exit"},
			{"0", "Exit without saving", "Discard changes and exit"},
		}

		for _, opt := range options {
			accentColor.Printf("  [%s] ", opt.key)
			infoColor.Printf("%-22s", opt.name)
			dimColor.Printf(" - %s\n", opt.desc)
		}

		fmt.Println()
		promptColor.Print("Select option: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			if err := configureLLMProvider(cfg, reader); err != nil {
				return err
			}
		case "2":
			if err := configureEmbeddingProvider(cfg, reader); err != nil {
				return err
			}
		case "3":
			if err := configureUserInfo(cfg, reader); err != nil {
				return err
			}
		case "4":
			if err := configureAPIKeys(cfg, reader); err != nil {
				return err
			}
		case "5":
			displayCurrentSettings(cfg)
		case "6":
			// Save and exit
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}
			fmt.Println()
			successColor.Println("Configuration saved successfully!")
			return nil
		case "0":
			fmt.Println()
			infoColor.Println("Configuration canceled. Changes not saved.")
			return nil
		default:
			dimColor.Println("Invalid option. Please try again.")
		}
	}
}

func configureLLMProvider(cfg *config.Config, reader *bufio.Reader) error {
	titleColor := color.New(color.FgHiCyan, color.Bold)
	promptColor := color.New(color.FgHiYellow)
	successColor := color.New(color.FgHiGreen)
	dimColor := color.New(color.FgHiBlack)
	infoColor := color.New(color.FgHiWhite)
	accentColor := color.New(color.FgHiMagenta)

	fmt.Println()
	titleColor.Println("Configure LLM Provider")
	dimColor.Println(strings.Repeat("─", 40))
	fmt.Println()

	infoColor.Printf("Current provider: %s\n", cfg.DefaultProvider)
	fmt.Println()

	// Display providers
	for _, p := range constants.AllProviders {
		if !p.SupportsLLM {
			continue
		}
		accentColor.Printf("  [%s] ", p.Key)
		infoColor.Printf("%-12s", p.Name)
		dimColor.Printf(" - %s\n", p.Description)
	}

	fmt.Println()
	promptColor.Print("Select provider (or press Enter to keep current): ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	if choice == "" {
		dimColor.Println("Keeping current provider.")
		return nil
	}

	// Find provider by key
	var selectedProvider constants.Provider
	for _, p := range constants.AllProviders {
		if p.Key == choice && p.SupportsLLM {
			selectedProvider = constants.Provider(strings.ToLower(p.Name))
			break
		}
	}

	if selectedProvider == "" {
		dimColor.Println("Invalid choice. Keeping current provider.")
		return nil
	}

	cfg.DefaultProvider = string(selectedProvider)

	// Configure provider-specific settings
	switch selectedProvider {
	case constants.ProviderOllama:
		configureOllama(cfg, reader)
	case constants.ProviderAnthropic:
		if err := configureAnthropic(cfg, reader); err != nil {
			return err
		}
	case constants.ProviderOpenAI:
		if err := configureOpenAI(cfg, reader); err != nil {
			return err
		}
	case constants.ProviderOpenRouter:
		if err := configureOpenRouter(cfg, reader); err != nil {
			return err
		}
	case constants.ProviderBedrock:
		if err := configureBedrock(cfg, reader); err != nil {
			return err
		}
	}

	fmt.Println()
	successColor.Printf("LLM provider updated to: %s\n", selectedProvider)
	return nil
}

func configureEmbeddingProvider(cfg *config.Config, reader *bufio.Reader) error {
	titleColor := color.New(color.FgHiCyan, color.Bold)
	promptColor := color.New(color.FgHiYellow)
	successColor := color.New(color.FgHiGreen)
	dimColor := color.New(color.FgHiBlack)
	infoColor := color.New(color.FgHiWhite)
	accentColor := color.New(color.FgHiMagenta)

	fmt.Println()
	titleColor.Println("Configure Embedding Provider")
	dimColor.Println(strings.Repeat("─", 40))
	fmt.Println()

	currentEmbed := cfg.EmbeddingProvider
	if currentEmbed == "" {
		currentEmbed = cfg.DefaultProvider
	}
	infoColor.Printf("Current embedding provider: %s\n", currentEmbed)
	if cfg.DefaultEmbedModel != "" {
		dimColor.Printf("Current embedding model: %s\n", cfg.DefaultEmbedModel)
	}
	fmt.Println()

	// Display embedding providers
	for _, p := range constants.AllEmbeddingProviders {
		accentColor.Printf("  [%s] ", p.Key)
		infoColor.Printf("%-30s", p.Name)
		dimColor.Printf(" - %s\n", p.Description)
	}

	fmt.Println()
	promptColor.Print("Select embedding provider (or press Enter to keep current): ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	if choice == "" {
		dimColor.Println("Keeping current embedding provider.")
		return nil
	}

	// Find embedding provider by key
	var selectedEmbedProvider constants.Provider
	for _, p := range constants.AllEmbeddingProviders {
		if p.Key == choice {
			selectedEmbedProvider = p.Provider
			break
		}
	}

	// If "same as LLM provider" or not found, use LLM provider
	if selectedEmbedProvider == "" {
		selectedEmbedProvider = constants.Provider(cfg.DefaultProvider)
		// Check if LLM provider supports embeddings
		if !constants.ProviderSupportsEmbeddings(selectedEmbedProvider) {
			fmt.Println()
			color.New(color.FgHiRed).Printf("Error: %s doesn't support embeddings. Please select a dedicated embedding provider.\n", cfg.DefaultProvider)
			return nil
		}
		cfg.EmbeddingProvider = string(selectedEmbedProvider)
		cfg.DefaultEmbedModel = constants.GetDefaultEmbeddingModel(selectedEmbedProvider)
	} else {
		cfg.EmbeddingProvider = string(selectedEmbedProvider)
		cfg.DefaultEmbedModel = constants.GetDefaultEmbeddingModel(selectedEmbedProvider)

		// Prompt for API key if needed and not already set
		switch selectedEmbedProvider {
		case constants.ProviderOpenAI:
			if cfg.OpenAIAPIKey == "" {
				promptColor.Print("OpenAI API Key: ")
				key, _ := reader.ReadString('\n')
				cfg.OpenAIAPIKey = strings.TrimSpace(key)
			}
		case constants.ProviderOpenRouter:
			if cfg.OpenRouterAPIKey == "" {
				promptColor.Print("OpenRouter API Key: ")
				key, _ := reader.ReadString('\n')
				cfg.OpenRouterAPIKey = strings.TrimSpace(key)
			}
		case constants.ProviderVoyageAI:
			if cfg.VoyageAIAPIKey == "" {
				promptColor.Print("Voyage AI API Key: ")
				key, _ := reader.ReadString('\n')
				cfg.VoyageAIAPIKey = strings.TrimSpace(key)
			}
		}
	}

	fmt.Println()
	successColor.Printf("Embedding provider updated to: %s\n", cfg.EmbeddingProvider)
	dimColor.Printf("Embedding model: %s\n", cfg.DefaultEmbedModel)
	return nil
}

func configureUserInfo(cfg *config.Config, reader *bufio.Reader) error {
	titleColor := color.New(color.FgHiCyan, color.Bold)
	promptColor := color.New(color.FgHiYellow)
	successColor := color.New(color.FgHiGreen)
	dimColor := color.New(color.FgHiBlack)

	fmt.Println()
	titleColor.Println("Configure User Information")
	dimColor.Println(strings.Repeat("─", 40))
	fmt.Println()

	// GitHub Username
	promptColor.Print("GitHub username")
	if cfg.GitHubUsername != "" {
		dimColor.Printf(" [current: %s]", cfg.GitHubUsername)
	}
	promptColor.Print(": ")
	username, _ := reader.ReadString('\n')
	username = strings.TrimSpace(username)
	if username != "" {
		cfg.GitHubUsername = username
	}

	// User Email
	promptColor.Print("Email")
	if cfg.UserEmail != "" {
		dimColor.Printf(" [current: %s]", cfg.UserEmail)
	}
	promptColor.Print(": ")
	email, _ := reader.ReadString('\n')
	email = strings.TrimSpace(email)
	if email != "" {
		cfg.UserEmail = email
	}

	// User Name
	promptColor.Print("Name")
	if cfg.UserName != "" {
		dimColor.Printf(" [current: %s]", cfg.UserName)
	}
	promptColor.Print(": ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name != "" {
		cfg.UserName = name
	}

	fmt.Println()
	successColor.Println("User information updated!")
	return nil
}

func configureAPIKeys(cfg *config.Config, reader *bufio.Reader) error {
	titleColor := color.New(color.FgHiCyan, color.Bold)
	promptColor := color.New(color.FgHiYellow)
	successColor := color.New(color.FgHiGreen)
	dimColor := color.New(color.FgHiBlack)
	infoColor := color.New(color.FgHiWhite)
	accentColor := color.New(color.FgHiMagenta)

	fmt.Println()
	titleColor.Println("Configure API Keys")
	dimColor.Println(strings.Repeat("─", 40))
	fmt.Println()

	options := []struct {
		key      string
		provider string
		desc     string
	}{
		{"1", "Anthropic", "Update Anthropic API key"},
		{"2", "OpenAI", "Update OpenAI API key"},
		{"3", "OpenRouter", "Update OpenRouter API key"},
		{"4", "Voyage AI", "Update Voyage AI API key"},
		{"5", "AWS Bedrock", "Update AWS credentials"},
		{"0", "Back", "Return to main menu"},
	}

	for _, opt := range options {
		accentColor.Printf("  [%s] ", opt.key)
		infoColor.Printf("%-12s", opt.provider)
		dimColor.Printf(" - %s\n", opt.desc)
	}

	fmt.Println()
	promptColor.Print("Select provider: ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	switch choice {
	case "1":
		promptColor.Print("Anthropic API Key: ")
		key, _ := reader.ReadString('\n')
		cfg.AnthropicAPIKey = strings.TrimSpace(key)
		successColor.Println("Anthropic API key updated!")
	case "2":
		promptColor.Print("OpenAI API Key: ")
		key, _ := reader.ReadString('\n')
		cfg.OpenAIAPIKey = strings.TrimSpace(key)
		successColor.Println("OpenAI API key updated!")
	case "3":
		promptColor.Print("OpenRouter API Key: ")
		key, _ := reader.ReadString('\n')
		cfg.OpenRouterAPIKey = strings.TrimSpace(key)
		successColor.Println("OpenRouter API key updated!")
	case "4":
		promptColor.Print("Voyage AI API Key: ")
		key, _ := reader.ReadString('\n')
		cfg.VoyageAIAPIKey = strings.TrimSpace(key)
		successColor.Println("Voyage AI API key updated!")
	case "5":
		promptColor.Print("AWS Access Key ID: ")
		accessKey, _ := reader.ReadString('\n')
		cfg.AWSAccessKeyID = strings.TrimSpace(accessKey)

		promptColor.Print("AWS Secret Access Key: ")
		secretKey, _ := reader.ReadString('\n')
		cfg.AWSSecretAccessKey = strings.TrimSpace(secretKey)

		promptColor.Print("AWS Region [us-east-1]: ")
		region, _ := reader.ReadString('\n')
		region = strings.TrimSpace(region)
		if region == "" {
			region = "us-east-1"
		}
		cfg.AWSRegion = region
		successColor.Println("AWS credentials updated!")
	case "0":
		return nil
	default:
		dimColor.Println("Invalid option.")
	}

	return nil
}

func displayCurrentSettings(cfg *config.Config) {
	titleColor := color.New(color.FgHiCyan, color.Bold)
	dimColor := color.New(color.FgHiBlack)
	infoColor := color.New(color.FgHiWhite)
	successColor := color.New(color.FgHiGreen)

	fmt.Println()
	titleColor.Println("Current Configuration")
	dimColor.Println(strings.Repeat("─", 50))
	fmt.Println()

	// LLM Configuration
	successColor.Println("LLM Provider:")
	infoColor.Printf("  Provider: %s\n", cfg.DefaultProvider)
	if cfg.DefaultModel != "" {
		infoColor.Printf("  Model: %s\n", cfg.DefaultModel)
	}
	if cfg.OllamaBaseURL != "" {
		dimColor.Printf("  Base URL: %s\n", cfg.OllamaBaseURL)
	}
	fmt.Println()

	// Embedding Configuration
	successColor.Println("Embeddings:")
	embedProvider := cfg.EmbeddingProvider
	if embedProvider == "" {
		embedProvider = cfg.DefaultProvider
	}
	infoColor.Printf("  Provider: %s\n", embedProvider)
	if cfg.DefaultEmbedModel != "" {
		infoColor.Printf("  Model: %s\n", cfg.DefaultEmbedModel)
	}
	fmt.Println()

	// User Information
	successColor.Println("User Information:")
	if cfg.GitHubUsername != "" {
		infoColor.Printf("  GitHub: %s\n", cfg.GitHubUsername)
	}
	if cfg.UserEmail != "" {
		infoColor.Printf("  Email: %s\n", cfg.UserEmail)
	}
	if cfg.UserName != "" {
		infoColor.Printf("  Name: %s\n", cfg.UserName)
	}
	fmt.Println()

	// API Keys (masked)
	successColor.Println("API Keys:")
	if cfg.AnthropicAPIKey != "" {
		infoColor.Printf("  Anthropic: %s\n", maskAPIKey(cfg.AnthropicAPIKey))
	}
	if cfg.OpenAIAPIKey != "" {
		infoColor.Printf("  OpenAI: %s\n", maskAPIKey(cfg.OpenAIAPIKey))
	}
	if cfg.OpenRouterAPIKey != "" {
		infoColor.Printf("  OpenRouter: %s\n", maskAPIKey(cfg.OpenRouterAPIKey))
	}
	if cfg.VoyageAIAPIKey != "" {
		infoColor.Printf("  Voyage AI: %s\n", maskAPIKey(cfg.VoyageAIAPIKey))
	}
	if cfg.AWSAccessKeyID != "" {
		infoColor.Printf("  AWS: %s\n", maskAPIKey(cfg.AWSAccessKeyID))
	}
	fmt.Println()

	// Profile Information
	if cfg.ActiveProfile != "" {
		successColor.Println("Active Profile:")
		infoColor.Printf("  %s\n", cfg.ActiveProfile)
		fmt.Println()
	}
}

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "..." + key[len(key)-4:]
}
