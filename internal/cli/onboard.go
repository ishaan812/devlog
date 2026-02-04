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

	providers := []struct {
		key  string
		name string
		desc string
	}{
		{"1", "Ollama", "Free, local, private (recommended for privacy)"},
		{"2", "Anthropic", "Claude models, excellent quality"},
		{"3", "OpenAI", "GPT-4, GPT-3.5 models"},
		{"4", "Bedrock", "Claude via AWS (enterprise)"},
	}

	for _, p := range providers {
		accentColor.Printf("  [%s] ", p.key)
		infoColor.Printf("%-12s", p.name)
		dimColor.Printf(" - %s\n", p.desc)
	}

	fmt.Println()
	promptColor.Print("Select provider (1-4): ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	switch choice {
	case "1":
		cfg.DefaultProvider = "ollama"
		configureOllama(cfg, reader)
	case "2":
		cfg.DefaultProvider = "anthropic"
		if err := configureAnthropic(cfg, reader); err != nil {
			return err
		}
	case "3":
		cfg.DefaultProvider = "openai"
		if err := configureOpenAI(cfg, reader); err != nil {
			return err
		}
	case "4":
		cfg.DefaultProvider = "bedrock"
		if err := configureBedrock(cfg, reader); err != nil {
			return err
		}
	default:
		cfg.DefaultProvider = "ollama"
		infoColor.Println("Defaulting to Ollama")
		configureOllama(cfg, reader)
	}

	// Step 3: User info
	fmt.Println()
	printStep(3, "Your information (optional)")
	fmt.Println()

	promptColor.Print("Your name (for work logs): ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name != "" {
		cfg.UserName = name
	}

	promptColor.Print("Your email (for filtering commits): ")
	email, _ := reader.ReadString('\n')
	email = strings.TrimSpace(email)
	if email != "" {
		cfg.UserEmail = email
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

	// Step 4: Quick tutorial
	fmt.Println()
	printStep(4, "Quick Tutorial")
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
	fmt.Println()
	infoColor.Println("Ollama runs locally on your machine.")
	fmt.Println()

	promptColor.Print("Ollama URL (press Enter for default): ")
	dimColor.Print("[http://localhost:11434] ")
	url, _ := reader.ReadString('\n')
	url = strings.TrimSpace(url)
	if url != "" {
		cfg.OllamaBaseURL = url
	} else {
		cfg.OllamaBaseURL = "http://localhost:11434"
	}

	promptColor.Print("Default model (press Enter for default): ")
	dimColor.Print("[llama3.2] ")
	model, _ := reader.ReadString('\n')
	model = strings.TrimSpace(model)
	if model != "" {
		cfg.OllamaModel = model
	} else {
		cfg.OllamaModel = "llama3.2"
	}

	fmt.Println()
	infoColor.Println("Make sure Ollama is running:")
	dimColor.Print("  $ ")
	infoColor.Println("ollama serve")
	dimColor.Print("  $ ")
	infoColor.Println("ollama pull llama3.2")
}

func configureAnthropic(cfg *config.Config, reader *bufio.Reader) error {
	fmt.Println()
	infoColor.Println("Get your API key from: console.anthropic.com")
	fmt.Println()

	promptColor.Print("Anthropic API Key: ")
	key, _ := reader.ReadString('\n')
	key = strings.TrimSpace(key)

	if key == "" {
		errorColor.Println("API key is required for Anthropic")
		return fmt.Errorf("API key required")
	}

	cfg.AnthropicAPIKey = key

	fmt.Println()
	successColor.Println("Anthropic configured!")
	dimColor.Println("Default model: claude-3-5-sonnet-20241022")

	return nil
}

func configureOpenAI(cfg *config.Config, reader *bufio.Reader) error {
	fmt.Println()
	infoColor.Println("Get your API key from: platform.openai.com")
	fmt.Println()

	promptColor.Print("OpenAI API Key: ")
	key, _ := reader.ReadString('\n')
	key = strings.TrimSpace(key)

	if key == "" {
		errorColor.Println("API key is required for OpenAI")
		return fmt.Errorf("API key required")
	}

	cfg.OpenAIAPIKey = key

	fmt.Println()
	successColor.Println("OpenAI configured!")
	dimColor.Println("Default model: gpt-4o-mini")

	return nil
}

func configureBedrock(cfg *config.Config, reader *bufio.Reader) error {
	fmt.Println()
	infoColor.Println("AWS Bedrock requires IAM credentials with Bedrock access.")
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
	dimColor.Print("[us-east-1] ")
	region, _ := reader.ReadString('\n')
	region = strings.TrimSpace(region)
	if region == "" {
		region = "us-east-1"
	}

	cfg.AWSAccessKeyID = accessKey
	cfg.AWSSecretAccessKey = secretKey
	cfg.AWSRegion = region

	fmt.Println()
	successColor.Println("AWS Bedrock configured!")
	dimColor.Println("Default model: anthropic.claude-3-5-sonnet-20241022-v2:0")

	return nil
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
