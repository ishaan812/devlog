package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/constants"
)

var (
	modelsSetProvider string
	modelsSetModel    string
	modelsSetAPIKey   string
	modelsSetGlobal   bool
)

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Manage LLM model configuration",
	Long: `View and configure the LLM provider, model, and API key for your profiles.

Each profile has its own LLM configuration. Use 'models set --global' to
apply the same configuration to all profiles at once.

Examples:
  devlog models                      # Show current config for active profile
  devlog models list                 # List available providers and models
  devlog models set                  # Interactive setup for active profile
  devlog models set --global         # Apply to all profiles
  devlog models set --provider anthropic --model claude-sonnet-4-20250514 --api-key sk-...`,
	RunE: runModelsShow,
}

var modelsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available providers and models",
	RunE:  runModelsList,
}

var modelsSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set LLM provider, model, and API key",
	Long: `Configure the LLM provider, model, and API key for the active profile.
Use --global to apply the same configuration to all profiles.`,
	RunE: runModelsSet,
}

func init() {
	rootCmd.AddCommand(modelsCmd)
	modelsCmd.AddCommand(modelsListCmd)
	modelsCmd.AddCommand(modelsSetCmd)

	modelsSetCmd.Flags().StringVar(&modelsSetProvider, "provider", "", "LLM provider (e.g. anthropic, openai, ollama)")
	modelsSetCmd.Flags().StringVar(&modelsSetModel, "model", "", "Model name")
	modelsSetCmd.Flags().StringVar(&modelsSetAPIKey, "api-key", "", "API key")
	modelsSetCmd.Flags().BoolVar(&modelsSetGlobal, "global", false, "Apply to all profiles")
}

func runModelsShow(cmd *cobra.Command, args []string) error {
	titleColor := color.New(color.FgHiCyan, color.Bold)
	infoColor := color.New(color.FgHiWhite)
	dimColor := color.New(color.FgHiBlack)
	successColor := color.New(color.FgHiGreen)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	profileName := cfg.GetActiveProfileName()
	profile := cfg.GetActiveProfile()

	fmt.Println()
	titleColor.Printf("  LLM Configuration")
	dimColor.Printf("  (profile: %s)\n\n", profileName)

	if profile == nil || profile.DefaultProvider == "" {
		dimColor.Println("  No LLM configured for this profile.")
		dimColor.Println("  Run 'devlog models set' to configure.")
		fmt.Println()
		return nil
	}

	successColor.Print("  Provider:  ")
	infoColor.Println(profile.DefaultProvider)

	if profile.DefaultModel != "" {
		successColor.Print("  Model:     ")
		infoColor.Println(profile.DefaultModel)
	}

	// Show masked API key for the active provider
	key := cfg.GetEffectiveAPIKey(profile.DefaultProvider)
	if key != "" {
		successColor.Print("  API Key:   ")
		dimColor.Println(maskAPIKey(key))
	}

	if profile.OllamaBaseURL != "" {
		dimColor.Printf("  Base URL:  %s\n", profile.OllamaBaseURL)
	}

	fmt.Println()
	return nil
}

func runModelsList(cmd *cobra.Command, args []string) error {
	titleColor := color.New(color.FgHiCyan, color.Bold)
	infoColor := color.New(color.FgHiWhite)
	dimColor := color.New(color.FgHiBlack)
	accentColor := color.New(color.FgHiMagenta)

	fmt.Println()
	titleColor.Println("  Available Providers & Models")
	fmt.Println()

	for _, p := range constants.AllProviders {
		if !p.SupportsLLM {
			continue
		}

		providerKey := strings.ToLower(p.Name)
		accentColor.Printf("  %s", p.Name)
		dimColor.Printf("  %s\n", p.Description)

		models := constants.GetLLMModels(constants.Provider(providerKey))
		if len(models) > 0 {
			for _, m := range models {
				infoColor.Printf("    %-40s", m.Model)
				dimColor.Printf("  %s\n", m.Description)
			}
		} else {
			dimColor.Println("    (no pre-defined models)")
		}
		fmt.Println()
	}

	dimColor.Println("  Use 'devlog models set --provider <name> --model <model>' to configure.")
	fmt.Println()
	return nil
}

func runModelsSet(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.EnsureDefaultProfile(); err != nil {
		return fmt.Errorf("failed to ensure default profile: %w", err)
	}

	profileName := cfg.GetActiveProfileName()
	profile := cfg.GetActiveProfile()
	if profile == nil {
		return fmt.Errorf("active profile '%s' not found", profileName)
	}

	// If no flags provided, run interactive mode
	if modelsSetProvider == "" && modelsSetModel == "" && modelsSetAPIKey == "" {
		return runModelsSetInteractive(cfg, profile, profileName)
	}

	// Flag-based mode
	return runModelsSetFlags(cfg, profile, profileName)
}

func runModelsSetFlags(cfg *config.Config, profile *config.Profile, profileName string) error {
	successColor := color.New(color.FgHiGreen)

	if modelsSetProvider != "" {
		profile.DefaultProvider = modelsSetProvider
	}
	if modelsSetModel != "" {
		profile.DefaultModel = modelsSetModel
	}
	if modelsSetAPIKey != "" {
		setAPIKeyOnProfile(profile, profile.DefaultProvider, modelsSetAPIKey)
	}

	if modelsSetGlobal {
		if err := cfg.ApplyLLMConfigToAllProfiles(profileName); err != nil {
			return fmt.Errorf("failed to apply to all profiles: %w", err)
		}
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if modelsSetGlobal {
		successColor.Printf("LLM config updated for all profiles (%s / %s)\n", profile.DefaultProvider, profile.DefaultModel)
	} else {
		successColor.Printf("LLM config updated for profile '%s' (%s / %s)\n", profileName, profile.DefaultProvider, profile.DefaultModel)
	}
	return nil
}

func runModelsSetInteractive(cfg *config.Config, profile *config.Profile, profileName string) error {
	reader := bufio.NewReader(os.Stdin)

	titleColor := color.New(color.FgHiCyan, color.Bold)
	promptColor := color.New(color.FgHiYellow)
	successColor := color.New(color.FgHiGreen)
	dimColor := color.New(color.FgHiBlack)
	infoColor := color.New(color.FgHiWhite)
	accentColor := color.New(color.FgHiMagenta)

	fmt.Println()
	titleColor.Printf("  Configure LLM")
	dimColor.Printf("  (profile: %s)\n\n", profileName)

	// Step 1: Pick provider
	for _, p := range constants.AllProviders {
		if !p.SupportsLLM {
			continue
		}
		accentColor.Printf("  [%s] ", p.Key)
		infoColor.Printf("%-12s", p.Name)
		dimColor.Printf(" - %s\n", p.Description)
	}
	fmt.Println()

	promptColor.Print("  Select provider: ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	var selectedProvider constants.Provider
	for _, p := range constants.AllProviders {
		if (p.Key == choice || strings.EqualFold(p.Name, choice)) && p.SupportsLLM {
			selectedProvider = constants.Provider(strings.ToLower(p.Name))
			break
		}
	}
	if selectedProvider == "" {
		return fmt.Errorf("invalid provider selection: %s", choice)
	}
	profile.DefaultProvider = string(selectedProvider)

	// Step 2: API key / config
	setupInfo := constants.GetProviderSetupInfo(selectedProvider)
	if setupInfo.NeedsAPIKey {
		fmt.Println()
		if setupInfo.APIKeyURL != "" {
			dimColor.Printf("  Get a key at: %s\n", setupInfo.APIKeyURL)
		}
		promptColor.Printf("  API Key: ")
		key, _ := reader.ReadString('\n')
		key = strings.TrimSpace(key)
		if key != "" {
			setAPIKeyOnProfile(profile, string(selectedProvider), key)
		}
	} else if selectedProvider == constants.ProviderOllama {
		fmt.Println()
		promptColor.Printf("  Ollama URL [http://localhost:11434]: ")
		url, _ := reader.ReadString('\n')
		url = strings.TrimSpace(url)
		if url != "" {
			profile.OllamaBaseURL = url
		} else {
			profile.OllamaBaseURL = "http://localhost:11434"
		}
	}

	// Step 3: Pick model
	models := constants.GetLLMModels(selectedProvider)
	if len(models) > 0 {
		fmt.Println()
		infoColor.Println("  Available models:")
		for _, m := range models {
			accentColor.Printf("  [%s] ", m.ID)
			infoColor.Printf("%-40s", m.Model)
			dimColor.Printf(" %s\n", m.Description)
		}
		fmt.Println()
		promptColor.Print("  Select model [1]: ")
		modelChoice, _ := reader.ReadString('\n')
		modelChoice = strings.TrimSpace(modelChoice)
		if modelChoice == "" {
			modelChoice = "1"
		}

		selectedModel := ""
		for _, m := range models {
			if m.ID == modelChoice {
				selectedModel = m.Model
				break
			}
		}
		if selectedModel == "" {
			// If they typed a model name directly, use it
			selectedModel = modelChoice
		}
		profile.DefaultModel = selectedModel
	} else {
		// No predefined models; ask for model name
		defaultModel := constants.GetDefaultModel(selectedProvider)
		fmt.Println()
		promptColor.Printf("  Model name [%s]: ", defaultModel)
		modelInput, _ := reader.ReadString('\n')
		modelInput = strings.TrimSpace(modelInput)
		if modelInput == "" {
			modelInput = defaultModel
		}
		profile.DefaultModel = modelInput
	}

	// Step 4: Apply globally?
	fmt.Println()
	promptColor.Print("  Apply to all profiles? [y/N]: ")
	globalChoice, _ := reader.ReadString('\n')
	globalChoice = strings.ToLower(strings.TrimSpace(globalChoice))
	applyGlobal := globalChoice == "y" || globalChoice == "yes"

	if applyGlobal {
		if err := cfg.ApplyLLMConfigToAllProfiles(profileName); err != nil {
			return fmt.Errorf("failed to apply globally: %w", err)
		}
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println()
	if applyGlobal {
		successColor.Printf("  LLM set to %s / %s for all profiles\n", profile.DefaultProvider, profile.DefaultModel)
	} else {
		successColor.Printf("  LLM set to %s / %s for profile '%s'\n", profile.DefaultProvider, profile.DefaultModel, profileName)
	}
	fmt.Println()
	return nil
}

// setAPIKeyOnProfile sets the appropriate API key field on the profile.
func setAPIKeyOnProfile(profile *config.Profile, provider, key string) {
	switch constants.Provider(provider) {
	case constants.ProviderAnthropic:
		profile.AnthropicAPIKey = key
	case constants.ProviderOpenAI:
		profile.OpenAIAPIKey = key
	case constants.ProviderChatGPT:
		profile.ChatGPTAccessToken = key
	case constants.ProviderOpenRouter:
		profile.OpenRouterAPIKey = key
	case constants.ProviderGemini:
		profile.GeminiAPIKey = key
	case constants.ProviderBedrock:
		profile.AWSAccessKeyID = key
	}
}
