package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/constants"
	"github.com/ishaan812/devlog/internal/tui"
)

var (
	onboardLegacy bool
)

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Interactive setup wizard for DevLog",
	Long: `Welcome to DevLog! This command will guide you through the initial setup.

You'll create a profile, configure your preferred LLM provider, and optionally
set up your user information.`,
	RunE: runOnboard,
}

func init() {
	rootCmd.AddCommand(onboardCmd)
	onboardCmd.Flags().BoolVar(&onboardLegacy, "legacy", false, "Use legacy text-based setup")
}

// Color helpers for legacy mode
var (
	titleColor   = color.New(color.FgHiCyan, color.Bold)
	successColor = color.New(color.FgHiGreen)
	errorColor   = color.New(color.FgHiRed)
	promptColor  = color.New(color.FgHiYellow)
	infoColor    = color.New(color.FgHiWhite)
	dimColor     = color.New(color.FgHiBlack)
	accentColor  = color.New(color.FgHiMagenta)
)

func runOnboard(cmd *cobra.Command, args []string) error {
	// Check if we should use the TUI (requires a terminal)
	if !onboardLegacy && term.IsTerminal(int(os.Stdin.Fd())) {
		cfg, err := tui.RunOnboard()
		if err != nil {
			// If TUI fails or is canceled, fall back to legacy
			if err.Error() == "onboarding canceled" {
				fmt.Println("Onboarding canceled.")
				return nil
			}
			// For other errors, try legacy mode
			fmt.Println("Falling back to text-based setup...")
			return runOnboardLegacy()
		}
		_ = cfg // Config is already saved by TUI
		return nil
	}

	return runOnboardLegacy()
}

func runOnboardLegacy() error {
	reader := bufio.NewReader(os.Stdin)

	// Welcome banner
	printBanner()

	// Load existing config
	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}

	fmt.Println()
	titleColor.Println("Welcome to DevLog Setup!")
	fmt.Println()
	infoColor.Println("DevLog helps you track and query your development activity")
	infoColor.Println("using natural language powered by LLMs.")
	fmt.Println()

	// Step 1: Create profile
	printStep(1, "Create a Profile")
	fmt.Println()

	promptColor.Print("Profile name (press Enter for default): ")
	dimColor.Print("[default] ")
	profileName, _ := reader.ReadString('\n')
	profileName = strings.TrimSpace(profileName)
	if profileName == "" {
		profileName = "default"
	}

	promptColor.Print("Profile description (optional): ")
	profileDesc, _ := reader.ReadString('\n')
	profileDesc = strings.TrimSpace(profileDesc)

	// Create the profile
	if err := cfg.CreateProfile(profileName, profileDesc); err != nil {
		// Profile might already exist, that's okay
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("failed to create profile: %w", err)
		}
	}
	cfg.ActiveProfile = profileName

	// Step 2: Choose provider
	fmt.Println()
	printStep(2, "Choose your LLM provider")
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
	promptColor.Print("Select provider (1-5): ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	// Find provider by key
	var selectedProvider constants.Provider
	for _, p := range constants.AllProviders {
		if p.Key == choice && p.SupportsLLM {
			selectedProvider = constants.Provider(strings.ToLower(p.Name))
			break
		}
	}

	if selectedProvider == "" {
		errorColor.Println("Invalid provider selection. Please try again.")
		return fmt.Errorf("invalid provider selection")
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
	case constants.ProviderOpenRouter:
		if err := configureOpenRouter(cfg, reader); err != nil {
			return err
		}
	case constants.ProviderBedrock:
		if err := configureBedrock(cfg, reader); err != nil {
			return err
		}
	}

	// Step 3: Embedding model configuration
	fmt.Println()
	printStep(3, "Embedding Model Configuration")
	fmt.Println()
	configureEmbeddings(cfg, reader)

	// Step 4: User info
	fmt.Println()
	printStep(4, "Your information (optional)")
	fmt.Println()

	promptColor.Print("GitHub username (for identifying your commits): ")
	githubUser, _ := reader.ReadString('\n')
	githubUser = strings.TrimSpace(githubUser)
	if githubUser != "" {
		cfg.GitHubUsername = githubUser
	}

	promptColor.Print("Your email (for filtering commits): ")
	email, _ := reader.ReadString('\n')
	email = strings.TrimSpace(email)
	if email != "" {
		cfg.UserEmail = email
	}

	promptColor.Print("Your name (for work logs): ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name != "" {
		cfg.UserName = name
	}

	// Save config
	fmt.Println()
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Saving configuration..."
	s.Color("cyan")
	s.Start()
	time.Sleep(500 * time.Millisecond)

	cfg.OnboardingComplete = true
	if err := cfg.Save(); err != nil {
		s.Stop()
		errorColor.Printf("Failed to save config: %v\n", err)
		return err
	}
	s.Stop()
	successColor.Println("Configuration saved!")

	// Step 5: Quick tutorial
	fmt.Println()
	printStep(5, "Quick Tutorial")
	fmt.Println()

	printTutorial()

	// Done
	fmt.Println()
	successColor.Println("Setup complete! You're ready to use DevLog.")
	fmt.Println()

	accentColor.Println("Get started:")
	fmt.Println()
	dimColor.Print("  $ ")
	infoColor.Println("devlog ingest           # Scan current repo")
	dimColor.Print("  $ ")
	infoColor.Println("devlog ask \"What did I do today?\"")
	fmt.Println()

	return nil
}

func printBanner() {
	banner := `
    ____            __
   / __ \___ _   __/ /   ____  ____ _
  / / / / _ \ | / / /   / __ \/ __ '/
 / /_/ /  __/ |/ / /___/ /_/ / /_/ /
/_____/\___/|___/_____/\____/\__, /
                            /____/
`
	accentColor.Println(banner)
}

func printStep(num int, title string) {
	titleColor.Printf("Step %d: %s\n", num, title)
	dimColor.Println(strings.Repeat("─", 40))
}

func configureOllama(cfg *config.Config, reader *bufio.Reader) {
	defaultURL := constants.GetDefaultBaseURL(constants.ProviderOllama)
	defaultModel := constants.GetDefaultModel(constants.ProviderOllama)

	fmt.Println()
	infoColor.Println("Ollama runs locally on your machine.")
	fmt.Println()

	promptColor.Print("Ollama URL (press Enter for default): ")
	dimColor.Printf("[%s] ", defaultURL)
	url, _ := reader.ReadString('\n')
	url = strings.TrimSpace(url)
	if url != "" {
		cfg.OllamaBaseURL = url
	} else {
		cfg.OllamaBaseURL = defaultURL
	}

	// Model selection
	fmt.Println()
	infoColor.Println("Select a model (or press Enter for default):")
	fmt.Println()

	models := constants.GetLLMModels(constants.ProviderOllama)
	for _, m := range models {
		accentColor.Printf("  [%s] ", m.ID)
		infoColor.Printf("%-20s", m.Model)
		dimColor.Printf(" - %s\n", m.Description)
	}

	fmt.Println()
	promptColor.Print("Select model: ")
	dimColor.Printf("[1] ")
	modelChoice, _ := reader.ReadString('\n')
	modelChoice = strings.TrimSpace(modelChoice)

	selectedModel := defaultModel
	for _, m := range models {
		if m.ID == modelChoice {
			selectedModel = m.Model
			break
		}
	}

	cfg.OllamaModel = selectedModel
	cfg.DefaultModel = selectedModel

	fmt.Println()
	infoColor.Println("Make sure Ollama is running:")
	dimColor.Print("  $ ")
	infoColor.Println("ollama serve")
	dimColor.Print("  $ ")
	infoColor.Printf("ollama pull %s\n", selectedModel)
}

func configureAnthropic(cfg *config.Config, reader *bufio.Reader) error {
	setupInfo := constants.GetProviderSetupInfo(constants.ProviderAnthropic)
	defaultModel := constants.GetDefaultModel(constants.ProviderAnthropic)

	fmt.Println()
	infoColor.Println(setupInfo.SetupHint)
	fmt.Println()

	promptColor.Print("Anthropic API Key: ")
	key, _ := reader.ReadString('\n')
	key = strings.TrimSpace(key)

	if key == "" {
		errorColor.Println("API key is required for Anthropic")
		return fmt.Errorf("API key required")
	}

	cfg.AnthropicAPIKey = key

	// Model selection
	fmt.Println()
	infoColor.Println("Select a model (or press Enter for default):")
	fmt.Println()

	models := constants.GetLLMModels(constants.ProviderAnthropic)
	for _, m := range models {
		accentColor.Printf("  [%s] ", m.ID)
		infoColor.Printf("%-35s", m.Model)
		dimColor.Printf(" - %s\n", m.Description)
	}

	fmt.Println()
	promptColor.Print("Select model: ")
	dimColor.Printf("[1] ")
	modelChoice, _ := reader.ReadString('\n')
	modelChoice = strings.TrimSpace(modelChoice)

	selectedModel := defaultModel
	for _, m := range models {
		if m.ID == modelChoice {
			selectedModel = m.Model
			break
		}
	}

	cfg.DefaultModel = selectedModel

	fmt.Println()
	successColor.Println("Anthropic configured!")
	dimColor.Printf("Model: %s\n", selectedModel)

	return nil
}

func configureOpenAI(cfg *config.Config, reader *bufio.Reader) error {
	setupInfo := constants.GetProviderSetupInfo(constants.ProviderOpenAI)
	defaultModel := constants.GetDefaultModel(constants.ProviderOpenAI)

	fmt.Println()
	infoColor.Println(setupInfo.SetupHint)
	fmt.Println()

	promptColor.Print("OpenAI API Key: ")
	key, _ := reader.ReadString('\n')
	key = strings.TrimSpace(key)

	if key == "" {
		errorColor.Println("API key is required for OpenAI")
		return fmt.Errorf("API key required")
	}

	cfg.OpenAIAPIKey = key

	// Model selection
	fmt.Println()
	infoColor.Println("Select a model (or press Enter for default):")
	fmt.Println()

	models := constants.GetLLMModels(constants.ProviderOpenAI)
	for _, m := range models {
		accentColor.Printf("  [%s] ", m.ID)
		infoColor.Printf("%-25s", m.Model)
		dimColor.Printf(" - %s\n", m.Description)
	}

	fmt.Println()
	promptColor.Print("Select model: ")
	dimColor.Printf("[1] ")
	modelChoice, _ := reader.ReadString('\n')
	modelChoice = strings.TrimSpace(modelChoice)

	selectedModel := defaultModel
	for _, m := range models {
		if m.ID == modelChoice {
			selectedModel = m.Model
			break
		}
	}

	cfg.DefaultModel = selectedModel

	fmt.Println()
	successColor.Println("OpenAI configured!")
	dimColor.Printf("Model: %s\n", selectedModel)

	return nil
}

func configureOpenRouter(cfg *config.Config, reader *bufio.Reader) error {
	setupInfo := constants.GetProviderSetupInfo(constants.ProviderOpenRouter)
	defaultModel := constants.GetDefaultModel(constants.ProviderOpenRouter)

	fmt.Println()
	infoColor.Println(setupInfo.SetupHint)
	fmt.Println()

	promptColor.Print("OpenRouter API Key: ")
	key, _ := reader.ReadString('\n')
	key = strings.TrimSpace(key)

	if key == "" {
		errorColor.Println("API key is required for OpenRouter")
		return fmt.Errorf("API key required")
	}

	cfg.OpenRouterAPIKey = key

	// Model selection
	fmt.Println()
	infoColor.Println("Select a model (or press Enter for auto-routing):")
	fmt.Println()

	models := constants.GetLLMModels(constants.ProviderOpenRouter)
	for _, m := range models {
		accentColor.Printf("  [%s] ", m.ID)
		infoColor.Printf("%-45s", m.Model)
		dimColor.Printf(" - %s\n", m.Description)
	}

	fmt.Println()
	promptColor.Printf("Select model (1-%d): ", len(models))
	dimColor.Print("[1] ")
	modelChoice, _ := reader.ReadString('\n')
	modelChoice = strings.TrimSpace(modelChoice)

	selectedModel := defaultModel
	for _, m := range models {
		if m.ID == modelChoice {
			selectedModel = m.Model
			break
		}
	}

	cfg.DefaultModel = selectedModel

	fmt.Println()
	successColor.Println("OpenRouter configured!")
	dimColor.Printf("Model: %s\n", selectedModel)

	return nil
}

func configureBedrock(cfg *config.Config, reader *bufio.Reader) error {
	setupInfo := constants.GetProviderSetupInfo(constants.ProviderBedrock)
	defaultModel := constants.GetDefaultModel(constants.ProviderBedrock)
	defaultRegion := constants.GetDefaultAWSRegion()

	fmt.Println()
	infoColor.Println(setupInfo.SetupHint)
	fmt.Println()

	promptColor.Print("AWS Access Key ID: ")
	accessKey, _ := reader.ReadString('\n')
	accessKey = strings.TrimSpace(accessKey)

	if accessKey == "" {
		errorColor.Println("Access Key ID is required")
		return fmt.Errorf("credentials required")
	}

	promptColor.Print("AWS Secret Access Key: ")
	secretKey, _ := reader.ReadString('\n')
	secretKey = strings.TrimSpace(secretKey)

	if secretKey == "" {
		errorColor.Println("Secret Access Key is required")
		return fmt.Errorf("credentials required")
	}

	promptColor.Print("AWS Region (press Enter for default): ")
	dimColor.Printf("[%s] ", defaultRegion)
	region, _ := reader.ReadString('\n')
	region = strings.TrimSpace(region)
	if region == "" {
		region = defaultRegion
	}

	cfg.AWSAccessKeyID = accessKey
	cfg.AWSSecretAccessKey = secretKey
	cfg.AWSRegion = region

	// Model selection
	fmt.Println()
	infoColor.Println("Select a model (or press Enter for default):")
	fmt.Println()

	models := constants.GetLLMModels(constants.ProviderBedrock)
	for _, m := range models {
		accentColor.Printf("  [%s] ", m.ID)
		infoColor.Printf("%-45s", m.Model)
		dimColor.Printf(" - %s\n", m.Description)
	}

	fmt.Println()
	promptColor.Print("Select model: ")
	dimColor.Printf("[1] ")
	modelChoice, _ := reader.ReadString('\n')
	modelChoice = strings.TrimSpace(modelChoice)

	selectedModel := defaultModel
	for _, m := range models {
		if m.ID == modelChoice {
			selectedModel = m.Model
			break
		}
	}

	cfg.DefaultModel = selectedModel

	fmt.Println()
	successColor.Println("AWS Bedrock configured!")
	dimColor.Printf("Model: %s\n", selectedModel)

	return nil
}

func configureEmbeddings(cfg *config.Config, reader *bufio.Reader) {
	infoColor.Println("Embeddings are used for semantic search and code similarity.")
	fmt.Println()

	// Display embedding provider options
	for _, p := range constants.AllEmbeddingProviders {
		accentColor.Printf("  [%s] ", p.Key)
		infoColor.Printf("%-30s", p.Name)
		dimColor.Printf(" - %s\n", p.Description)
	}

	fmt.Println()
	promptColor.Printf("Select embedding provider (1-%d): ", len(constants.AllEmbeddingProviders))
	dimColor.Print("[1] ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	// Find embedding provider by key
	var selectedEmbedProvider constants.Provider
	for _, p := range constants.AllEmbeddingProviders {
		if p.Key == choice {
			selectedEmbedProvider = p.Provider
			break
		}
	}

	if selectedEmbedProvider == "" {
		// "Same as LLM provider" or default
		llmProvider := constants.Provider(cfg.DefaultProvider)
		if constants.ProviderSupportsEmbeddings(llmProvider) {
			cfg.EmbeddingProvider = cfg.DefaultProvider
		} else {
			errorColor.Printf("Error: %s doesn't support embeddings. Please select a dedicated embedding provider.\n", cfg.DefaultProvider)
			return
		}
	} else {
		cfg.EmbeddingProvider = string(selectedEmbedProvider)

		// Prompt for API key if needed
		setupInfo := constants.GetProviderSetupInfo(selectedEmbedProvider)
		if setupInfo.NeedsAPIKey {
			existingKey := getExistingAPIKeyForProvider(cfg, selectedEmbedProvider)
			if existingKey == "" {
				promptColor.Printf("%s API Key (for embeddings): ", strings.Title(string(selectedEmbedProvider)))
				key, _ := reader.ReadString('\n')
				setAPIKeyForProvider(cfg, selectedEmbedProvider, strings.TrimSpace(key))
			}
		}
	}

	cfg.DefaultEmbedModel = constants.GetDefaultEmbeddingModel(constants.Provider(cfg.EmbeddingProvider))

	fmt.Println()
	successColor.Printf("Embedding provider: %s\n", cfg.EmbeddingProvider)
	dimColor.Printf("Embedding model: %s\n", cfg.DefaultEmbedModel)
}

// getExistingAPIKeyForProvider returns the current API key for a provider from config
func getExistingAPIKeyForProvider(cfg *config.Config, provider constants.Provider) string {
	switch provider {
	case constants.ProviderOpenAI:
		return cfg.OpenAIAPIKey
	case constants.ProviderOpenRouter:
		return cfg.OpenRouterAPIKey
	case constants.ProviderVoyageAI:
		return cfg.VoyageAIAPIKey
	case constants.ProviderAnthropic:
		return cfg.AnthropicAPIKey
	default:
		return ""
	}
}

// setAPIKeyForProvider sets the API key for a provider in config
func setAPIKeyForProvider(cfg *config.Config, provider constants.Provider, key string) {
	switch provider {
	case constants.ProviderOpenAI:
		cfg.OpenAIAPIKey = key
	case constants.ProviderOpenRouter:
		cfg.OpenRouterAPIKey = key
	case constants.ProviderVoyageAI:
		cfg.VoyageAIAPIKey = key
	case constants.ProviderAnthropic:
		cfg.AnthropicAPIKey = key
	}
}

func printTutorial() {
	sections := []struct {
		title    string
		commands []string
	}{
		{
			"Ingest repositories",
			[]string{
				"devlog ingest              # Current directory",
				"devlog ingest ~/projects   # Specific path",
			},
		},
		{
			"Ask questions",
			[]string{
				"devlog ask \"What did I work on this week?\"",
				"devlog ask \"Which files have I changed the most?\"",
				"devlog ask \"Show commits about authentication\"",
			},
		},
		{
			"Manage profiles",
			[]string{
				"devlog profile list        # Show all profiles",
				"devlog profile create work # Create new profile",
				"devlog profile use work    # Switch profile",
			},
		},
		{
			"Generate work logs",
			[]string{
				"devlog worklog --days 7           # Last week",
				"devlog worklog --days 30 -o log.md  # Export to file",
			},
		},
	}

	for _, sec := range sections {
		accentColor.Printf("  %s\n", sec.title)
		for _, cmd := range sec.commands {
			dimColor.Print("    $ ")
			infoColor.Println(cmd)
		}
		fmt.Println()
	}
}
