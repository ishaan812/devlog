package cli

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/db"
	"github.com/ishaan812/devlog/internal/llm"
	"github.com/ishaan812/devlog/internal/prompts"
)

var (
	commitAll      bool
	commitProvider string
	commitModel    string
)

var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Generate a commit message from your current changes",
	Long: `Analyze your current git diff and generate a commit message using LLM.

By default, only staged changes (git diff --cached) are analyzed.
Use --all to include unstaged changes too.

Examples:
  devlog commit             # Generate message from staged changes
  devlog commit --all       # Include unstaged changes too`,
	RunE: runCommitMessage,
}

func init() {
	rootCmd.AddCommand(commitCmd)

	commitCmd.Flags().BoolVarP(&commitAll, "all", "a", false, "Include unstaged changes (uses git diff HEAD)")
	commitCmd.Flags().StringVar(&commitProvider, "provider", "", "LLM provider override")
	commitCmd.Flags().StringVar(&commitModel, "model", "", "LLM model override")
}

func runCommitMessage(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	dimColor := color.New(color.FgHiBlack)
	titleColor := color.New(color.FgHiCyan, color.Bold)
	msgColor := color.New(color.FgHiWhite)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w\n\nRun 'devlog onboard' to set up your configuration", err)
	}

	// Get the diff using git CLI
	diff, err := getGitDiff(commitAll)
	if err != nil {
		return fmt.Errorf("failed to get git diff: %w", err)
	}

	if strings.TrimSpace(diff) == "" {
		if commitAll {
			fmt.Println("No changes found. Working tree is clean.")
		} else {
			fmt.Println("No staged changes found. Stage changes with 'git add' first, or use --all to include unstaged changes.")
		}
		return nil
	}

	// Load project context from codebase summary if available
	projectContext := "(No project context available)"
	codebasePath, err := filepath.Abs(".")
	if err == nil {
		dbRepo, dbErr := db.GetRepository()
		if dbErr == nil {
			codebase, cbErr := dbRepo.GetCodebaseByPath(ctx, codebasePath)
			if cbErr == nil && codebase != nil && codebase.Summary != "" {
				projectContext = codebase.Summary
			}
		}
	}

	// Create LLM client
	client, err := createCommitClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w\n\nRun 'devlog onboard' to configure your LLM provider", err)
	}

	if commitAll {
		dimColor.Println("  Analyzing all changes (staged + unstaged)...")
	} else {
		dimColor.Println("  Analyzing staged changes...")
	}

	// Generate commit message
	prompt := prompts.BuildCommitMessagePrompt(projectContext, diff)

	llmCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	result, err := client.Complete(llmCtx, prompt)
	if err != nil {
		return fmt.Errorf("failed to generate commit message: %w", err)
	}

	result = strings.TrimSpace(result)

	// Display the result
	fmt.Println()
	titleColor.Println("  Commit Message")
	dimColor.Println("  " + strings.Repeat("─", 50))
	fmt.Println()

	// Split title and body for display
	lines := strings.SplitN(result, "\n", 2)
	msgColor.Printf("  %s\n", lines[0])
	if len(lines) > 1 {
		body := strings.TrimSpace(lines[1])
		if body != "" {
			fmt.Println()
			for _, line := range strings.Split(body, "\n") {
				dimColor.Printf("  %s\n", line)
			}
		}
	}

	fmt.Println()
	dimColor.Println("  " + strings.Repeat("─", 50))
	dimColor.Println("  Copy and use with: git commit -m \"<title>\" -m \"<body>\"")
	fmt.Println()

	return nil
}

// getGitDiff runs git diff and returns the output
func getGitDiff(all bool) (string, error) {
	var cmd *exec.Cmd
	if all {
		cmd = exec.Command("git", "diff", "HEAD")
	} else {
		cmd = exec.Command("git", "diff", "--cached")
	}

	output, err := cmd.Output()
	if err != nil {
		// If --cached returns nothing and all is false, the user may not have staged anything
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git diff failed: %s", string(exitErr.Stderr))
		}
		return "", err
	}

	return string(output), nil
}

func createCommitClient(cfg *config.Config) (llm.Client, error) {
	selectedProvider := commitProvider
	if selectedProvider == "" {
		selectedProvider = cfg.DefaultProvider
	}
	if selectedProvider == "" {
		return nil, fmt.Errorf("no provider configured; run 'devlog onboard' first")
	}
	selectedModel := commitModel
	if selectedModel == "" {
		selectedModel = cfg.DefaultModel
	}
	llmCfg := llm.Config{Provider: llm.Provider(selectedProvider), Model: selectedModel}
	switch llmCfg.Provider {
	case llm.ProviderOpenAI:
		llmCfg.APIKey = cfg.GetAPIKey("openai")
	case llm.ProviderAnthropic:
		llmCfg.APIKey = cfg.GetAPIKey("anthropic")
	case llm.ProviderOpenRouter:
		llmCfg.APIKey = cfg.GetAPIKey("openrouter")
	case llm.ProviderGemini:
		llmCfg.APIKey = cfg.GetAPIKey("gemini")
	case llm.ProviderBedrock:
		llmCfg.AWSAccessKeyID = cfg.AWSAccessKeyID
		llmCfg.AWSSecretAccessKey = cfg.AWSSecretAccessKey
		llmCfg.AWSRegion = cfg.AWSRegion
	case llm.ProviderOllama:
		if cfg.OllamaBaseURL != "" {
			llmCfg.BaseURL = cfg.OllamaBaseURL
		}
		if selectedModel == "" && cfg.OllamaModel != "" {
			llmCfg.Model = cfg.OllamaModel
		}
	}
	return llm.NewClient(llmCfg)
}
