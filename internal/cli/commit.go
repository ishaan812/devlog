package cli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

By default, both staged and unstaged changes are analyzed.
Use --staged-only to analyze only staged changes.

Examples:
  devlog commit                # Generate message from all changes (staged + unstaged)
  devlog commit --staged-only  # Only analyze staged changes`,
	RunE: runCommitMessage,
}

func init() {
	rootCmd.AddCommand(commitCmd)

	commitCmd.Flags().BoolVarP(&commitAll, "staged-only", "s", false, "Only analyze staged changes (git diff --cached)")
	commitCmd.Flags().StringVar(&commitProvider, "provider", "", "LLM provider override")
	commitCmd.Flags().StringVar(&commitModel, "model", "", "LLM model override")
}

func runCommitMessage(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	dimColor := color.New(color.FgHiBlack)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w\n\nRun 'devlog onboard' to set up your configuration", err)
	}

	// By default, show all changes (staged + unstaged), unless --staged-only is set
	stagedOnly := commitAll
	diff, err := getGitDiff(stagedOnly)
	if err != nil {
		return fmt.Errorf("failed to get git diff: %w", err)
	}

	if strings.TrimSpace(diff) == "" {
		if stagedOnly {
			fmt.Println("No staged changes found. Stage changes with 'git add' first.")
		} else {
			fmt.Println("No changes found. Working tree is clean.")
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

	if stagedOnly {
		dimColor.Println("  Analyzing staged changes...")
	} else {
		dimColor.Println("  Analyzing all changes (staged + unstaged)...")
	}

	prompt := prompts.BuildCommitMessagePrompt(projectContext, diff)

	llmCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	result, err := client.Complete(llmCtx, prompt)
	if err != nil {
		return fmt.Errorf("failed to generate commit message: %w", err)
	}

	result = strings.TrimSpace(result)

	// Launch TUI to confirm and execute git commands
	return runCommitTUI(result, stagedOnly)
}

func getGitDiff(stagedOnly bool) (string, error) {
	var cmd *exec.Cmd
	if stagedOnly {
		cmd = exec.Command("git", "diff", "--cached")
	} else {
		cmd = exec.Command("git", "diff", "HEAD")
	}

	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
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

// ── Commit TUI ─────────────────────────────────────────────────────────────

var (
	commitTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("86")).
				Padding(0, 1)

	commitMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)

	commitDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Padding(0, 1)

	commitHighlightStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("86")).
				Bold(true)

	commitErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196")).
				Padding(0, 1)

	commitSuccessStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("82")).
				Padding(0, 1)
)

type commitTUIModel struct {
	commitMessage string
	stagedOnly    bool
	step          int // 0 = show message, 1 = confirm stage, 2 = confirm commit, 3 = done/error
	cursor        int
	status        string
	err           error
	quitting      bool
}

func (m commitTUIModel) Init() tea.Cmd {
	return nil
}

func (m commitTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit

		case "y", "Y":
			if m.step == 0 {
				// User confirmed the message, check if we need to stage
				if m.stagedOnly {
					// Already staged, skip to commit
					m.step = 2
					m.cursor = 0
				} else {
					// Ask if they want to stage
					m.step = 1
					m.cursor = 0
				}
			} else if m.step == 1 {
				// User wants to stage
				if err := gitAddAll(); err != nil {
					m.err = err
					m.step = 3
					return m, tea.Quit
				}
				m.status = "All changes staged successfully!"
				m.step = 2
				m.cursor = 0
			} else if m.step == 2 {
				// User wants to commit
				if err := gitCommit(m.commitMessage); err != nil {
					m.err = err
					m.step = 3
					return m, tea.Quit
				}
				m.status = "Committed successfully!"
				m.step = 3
				return m, tea.Quit
			}
			return m, nil

		case "n", "N":
			if m.step == 0 {
				// User doesn't want to use this message
				m.quitting = true
				return m, tea.Quit
			} else if m.step == 1 {
				// User doesn't want to stage, skip to commit question
				m.step = 2
				m.cursor = 0
			} else if m.step == 2 {
				// User doesn't want to commit
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil

		case "enter":
			// Same as 'y' for convenience
			return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
		}
	}

	return m, nil
}

func (m commitTUIModel) View() string {
	if m.quitting && m.err == nil && m.status == "" {
		return commitDimStyle.Render("Canceled.\n")
	}

	if m.step == 3 {
		if m.err != nil {
			return commitErrorStyle.Render(fmt.Sprintf("Error: %v\n", m.err))
		}
		return commitSuccessStyle.Render(m.status + "\n")
	}

	var s strings.Builder

	s.WriteString("\n")
	s.WriteString(commitTitleStyle.Render("Generated Commit Message"))
	s.WriteString("\n\n")

	// Show the commit message
	lines := strings.SplitN(m.commitMessage, "\n", 2)
	s.WriteString(commitHighlightStyle.Render("  " + lines[0]))
	s.WriteString("\n")
	if len(lines) > 1 {
		body := strings.TrimSpace(lines[1])
		if body != "" {
			s.WriteString("\n")
			for _, line := range strings.Split(body, "\n") {
				s.WriteString(commitDimStyle.Render("  " + line))
				s.WriteString("\n")
			}
		}
	}

	s.WriteString("\n")
	s.WriteString(commitDimStyle.Render("  " + strings.Repeat("─", 60)))
	s.WriteString("\n\n")

	if m.step == 0 {
		s.WriteString(commitMsgStyle.Render("  Use this commit message? (y/n)"))
		s.WriteString("\n")
	} else if m.step == 1 {
		s.WriteString(commitSuccessStyle.Render("  Message approved!"))
		s.WriteString("\n\n")
		s.WriteString(commitMsgStyle.Render("  Run 'git add .' to stage all changes? (y/n)"))
		s.WriteString("\n")
	} else if m.step == 2 {
		if m.status != "" {
			s.WriteString(commitSuccessStyle.Render("  " + m.status))
			s.WriteString("\n\n")
		}
		s.WriteString(commitMsgStyle.Render("  Commit with this message? (y/n)"))
		s.WriteString("\n")
	}

	s.WriteString("\n")
	s.WriteString(commitDimStyle.Render("  Press 'q' or 'Ctrl+C' to cancel"))
	s.WriteString("\n")

	return s.String()
}

func runCommitTUI(commitMessage string, stagedOnly bool) error {
	model := commitTUIModel{
		commitMessage: commitMessage,
		stagedOnly:    stagedOnly,
		step:          0,
	}

	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// Check if there was an error during git operations
	if m, ok := finalModel.(commitTUIModel); ok {
		if m.err != nil {
			return m.err
		}
	}

	return nil
}

func gitAddAll() error {
	cmd := exec.Command("git", "add", ".")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git add failed: %s", string(output))
	}
	return nil
}

func gitCommit(message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git commit failed: %s", string(output))
	}
	return nil
}
