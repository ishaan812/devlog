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

	client, err := createCommitClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w\n\nRun 'devlog onboard' to configure your LLM provider", err)
	}

	if commitAll {
		dimColor.Println("  Analyzing all changes (staged + unstaged)...")
	} else {
		dimColor.Println("  Analyzing staged changes...")
	}

	prompt := prompts.BuildCommitMessagePrompt(projectContext, diff)

	llmCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	result, err := client.Complete(llmCtx, prompt)
	if err != nil {
		return fmt.Errorf("failed to generate commit message: %w", err)
	}

	result = strings.TrimSpace(result)

	fmt.Println()
	titleColor.Println("  Commit Message")
	dimColor.Println("  " + strings.Repeat("─", 50))
	fmt.Println()

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

func getGitDiff(all bool) (string, error) {
	var cmd *exec.Cmd
	if all {
		cmd = exec.Command("git", "diff", "HEAD")
	} else {
		cmd = exec.Command("git", "diff", "--cached")
	}

	output, err := cmd.Output()
	if err != nil {
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
		selectedProvider = cfg.GetEffectiveProvider()
	}
	if selectedProvider == "" {
		return nil, fmt.Errorf("no provider configured; run 'devlog onboard' first")
	}
	selectedModel := commitModel
	if selectedModel == "" {
		selectedModel = cfg.GetEffectiveModel()
	}
	llmCfg := llm.Config{Provider: llm.Provider(selectedProvider), Model: selectedModel}
	switch llmCfg.Provider {
	case llm.ProviderOpenAI:
		llmCfg.APIKey = cfg.GetEffectiveAPIKey("openai")
	case llm.ProviderChatGPT:
		llmCfg.APIKey = cfg.GetEffectiveAPIKey("chatgpt")
	case llm.ProviderAnthropic:
		llmCfg.APIKey = cfg.GetEffectiveAPIKey("anthropic")
	case llm.ProviderOpenRouter:
		llmCfg.APIKey = cfg.GetEffectiveAPIKey("openrouter")
	case llm.ProviderGemini:
		llmCfg.APIKey = cfg.GetEffectiveAPIKey("gemini")
	case llm.ProviderBedrock:
		llmCfg.AWSAccessKeyID = cfg.GetEffectiveAWSAccessKeyID()
		llmCfg.AWSSecretAccessKey = cfg.GetEffectiveAWSSecretAccessKey()
		llmCfg.AWSRegion = cfg.GetEffectiveAWSRegion()
	case llm.ProviderOllama:
		if url := cfg.GetEffectiveOllamaBaseURL(); url != "" {
			llmCfg.BaseURL = url
		}
		if selectedModel == "" {
			if model := cfg.GetEffectiveOllamaModel(); model != "" {
				llmCfg.Model = model
			}
		}
	}
	return llm.NewClient(llmCfg)
}
