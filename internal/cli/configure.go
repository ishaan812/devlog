package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/ishaan812/devlog/internal/auth"
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
	Long: `Configure your DevLog settings including LLM provider and user information.

This command opens an interactive configuration wizard that allows you to update:
  - LLM provider and API keys
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
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.EnsureDefaultProfile(); err != nil {
		return fmt.Errorf("failed to ensure default profile: %w", err)
	}

	if !configureLegacy && term.IsTerminal(int(os.Stdin.Fd())) {
		updatedCfg, err := tui.RunConfigure(cfg)
		if err != nil {
			if err.Error() == "configuration canceled" {
				fmt.Println("Configuration canceled.")
				return nil
			}
			fmt.Println("Falling back to text-based configuration...")
			return runConfigureLegacy(cfg)
		}
		_ = updatedCfg
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

	cfg.HydrateGlobalFromActiveProfile()

	fmt.Println()
	titleColor.Println("DevLog Configuration")
	fmt.Println()

	fmt.Println()
	titleColor.Println("Current Settings:")
	dimColor.Println(strings.Repeat("─", 40))
	infoColor.Printf("  LLM Provider: %s\n", cfg.DefaultProvider)
	if cfg.DefaultModel != "" {
		dimColor.Printf("  LLM Model: %s\n", cfg.DefaultModel)
	}
	if cfg.GitHubUsername != "" {
		infoColor.Printf("  GitHub Username: %s\n", cfg.GitHubUsername)
	}
	if cfg.UserEmail != "" {
		dimColor.Printf("  Email: %s\n", cfg.UserEmail)
	}
	fmt.Println()

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
			{"2", "User Information", "Update name, email, GitHub username"},
			{"3", "API Keys", "Update API keys for providers"},
			{"4", "View Settings", "Display current configuration"},
			{"5", "Save & Exit", "Save changes and exit"},
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
			if err := configureUserInfo(cfg, reader); err != nil {
				return err
			}
		case "3":
			if err := configureAPIKeys(cfg, reader); err != nil {
				return err
			}
		case "4":
			displayCurrentSettings(cfg)
		case "5":
			cfg.CopyLLMConfigToProfile(cfg.GetActiveProfileName())
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
	case constants.ProviderChatGPT:
		if err := configureChatGPT(cfg, reader); err != nil {
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

func configureUserInfo(cfg *config.Config, reader *bufio.Reader) error {
	titleColor := color.New(color.FgHiCyan, color.Bold)
	promptColor := color.New(color.FgHiYellow)
	successColor := color.New(color.FgHiGreen)
	dimColor := color.New(color.FgHiBlack)

	fmt.Println()
	titleColor.Println("Configure User Information")
	dimColor.Println(strings.Repeat("─", 40))
	fmt.Println()

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
		{"3", "ChatGPT", "Sign in with ChatGPT (OAuth)"},
		{"4", "OpenRouter", "Update OpenRouter API key"},
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
		infoColor.Println("Sign in with ChatGPT...")
		dimColor.Println("A browser window will open for you to sign in.")
		dimColor.Println("Requires a Plus, Pro, Team, or Enterprise plan.")
		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = " Waiting for browser login..."
		s.Start()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		tokens, authErr := auth.LoginWithChatGPT(ctx)
		cancel()
		s.Stop()
		if authErr != nil {
			errorColor.Printf("Login failed: %v\n", authErr)
		} else {
			cfg.ChatGPTAccessToken = tokens.BearerToken()
			cfg.ChatGPTRefreshToken = tokens.RefreshToken
			successColor.Println("Signed in with ChatGPT!")
		}
	case "4":
		promptColor.Print("OpenRouter API Key: ")
		key, _ := reader.ReadString('\n')
		cfg.OpenRouterAPIKey = strings.TrimSpace(key)
		successColor.Println("OpenRouter API key updated!")
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
	if cfg.AWSAccessKeyID != "" {
		infoColor.Printf("  AWS: %s\n", maskAPIKey(cfg.AWSAccessKeyID))
	}
	fmt.Println()

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
