package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/ishaan812/devlog/internal/db"
	"github.com/ishaan812/devlog/internal/git"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var (
	branchCodebase string
)

var branchCmd = &cobra.Command{
	Use:   "branch",
	Short: "Manage and view branch information",
	Long: `Manage branches and their metadata in the active profile.

Without a subcommand, lists all indexed branches for the current codebase.

Examples:
  devlog branch                      # List branches
  devlog branch list                 # Same as above
  devlog branch show <name>          # Show branch details
  devlog branch story <name>         # Add/edit branch story
  devlog branch set-default <name>   # Set default branch`,
	RunE: runBranchList,
}

var branchListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all indexed branches",
	Long:  `List all branches that have been indexed in the current codebase.`,
	RunE:  runBranchList,
}

var branchShowCmd = &cobra.Command{
	Use:   "show [name]",
	Short: "Show branch details",
	Long:  `Show detailed information about a specific branch, including commit counts and story.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runBranchShow,
}

var branchStoryCmd = &cobra.Command{
	Use:   "story <name>",
	Short: "Add or edit branch story",
	Long: `Add or edit a story/description for a branch.

A branch story describes what work is being done on that branch,
making it easier to recall context when switching between branches.

Examples:
  devlog branch story feature/auth       # Interactive editor
  devlog branch story feature/auth -m "Implementing OAuth2 login"`,
	Args: cobra.ExactArgs(1),
	RunE: runBranchStory,
}

var branchSetDefaultCmd = &cobra.Command{
	Use:   "set-default <name>",
	Short: "Set the default branch",
	Long:  `Set the default/main branch for the current codebase.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runBranchSetDefault,
}

var (
	branchStoryMessage string
)

func init() {
	rootCmd.AddCommand(branchCmd)
	branchCmd.AddCommand(branchListCmd)
	branchCmd.AddCommand(branchShowCmd)
	branchCmd.AddCommand(branchStoryCmd)
	branchCmd.AddCommand(branchSetDefaultCmd)

	// Global flag for specifying codebase path
	branchCmd.PersistentFlags().StringVar(&branchCodebase, "codebase", "", "Codebase path (default: current directory)")

	// Story command flag
	branchStoryCmd.Flags().StringVarP(&branchStoryMessage, "message", "m", "", "Branch story/description")
}

func runBranchList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	titleColor := color.New(color.FgHiCyan, color.Bold)
	successColor := color.New(color.FgHiGreen, color.Bold)
	infoColor := color.New(color.FgWhite)
	dimColor := color.New(color.FgHiBlack)

	codebasePath := branchCodebase
	if codebasePath == "" {
		codebasePath, _ = filepath.Abs(".")
	}

	dbRepo, err := db.GetRepository()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	codebase, err := dbRepo.GetCodebaseByPath(ctx, codebasePath)
	if err != nil {
		return fmt.Errorf("failed to get codebase: %w", err)
	}

	if codebase == nil {
		return fmt.Errorf("codebase not indexed. Run 'devlog ingest' first")
	}

	branches, err := dbRepo.GetBranchesByCodebase(ctx, codebase.ID)
	if err != nil {
		return fmt.Errorf("failed to get branches: %w", err)
	}
	_ = successColor
	_ = infoColor

	// Header
	fmt.Println()
	titleColor.Printf("  Branches - %s\n", codebase.Name)
	dimColor.Println("  " + strings.Repeat("─", 40))
	fmt.Println()

	if len(branches) == 0 {
		dimColor.Println("  No branches indexed yet.")
		dimColor.Println("  Run 'devlog ingest' to index branches.")
		fmt.Println()
		return nil
	}

	// List branches
	for _, branch := range branches {
		// Branch name with default indicator
		if branch.IsDefault {
			successColor.Printf("  (*) %s", branch.Name)
			dimColor.Print(" (default)")
		} else {
			infoColor.Printf("      %s", branch.Name)
		}

		// Commit count
		dimColor.Printf(" - %d commits", branch.CommitCount)

		// Status
		if branch.Status != "" && branch.Status != "active" {
			dimColor.Printf(" [%s]", branch.Status)
		}

		fmt.Println()

		// Show story preview if exists
		if branch.Story != "" {
			storyPreview := branch.Story
			if len(storyPreview) > 60 {
				storyPreview = storyPreview[:57] + "..."
			}
			dimColor.Printf("        %s\n", storyPreview)
		}
	}

	fmt.Println()
	dimColor.Printf("  Total: %d branch(es)\n", len(branches))
	fmt.Println()

	return nil
}

func runBranchShow(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	titleColor := color.New(color.FgHiCyan, color.Bold)
	successColor := color.New(color.FgHiGreen)
	infoColor := color.New(color.FgWhite)
	dimColor := color.New(color.FgHiBlack)

	codebasePath := branchCodebase
	if codebasePath == "" {
		codebasePath, _ = filepath.Abs(".")
	}

	dbRepo, err := db.GetRepository()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	codebase, err := dbRepo.GetCodebaseByPath(ctx, codebasePath)
	if err != nil {
		return fmt.Errorf("failed to get codebase: %w", err)
	}

	if codebase == nil {
		return fmt.Errorf("codebase not indexed. Run 'devlog ingest' first")
	}

	var branchName string
	if len(args) > 0 {
		branchName = args[0]
	} else {
		repo, err := git.OpenRepo(codebasePath)
		if err != nil {
			return fmt.Errorf("specify a branch name or run from a git repository")
		}
		branchName, err = repo.GetCurrentBranch()
		if err != nil {
			return fmt.Errorf("specify a branch name or run from a git repository")
		}
	}

	branch, err := dbRepo.GetBranch(ctx, codebase.ID, branchName)
	if err != nil {
		return fmt.Errorf("failed to get branch: %w", err)
	}

	if branch == nil {
		return fmt.Errorf("branch '%s' not found. Has it been ingested?", branchName)
	}
	_ = successColor

	// Display branch details
	fmt.Println()
	titleColor.Printf("  Branch: %s\n", branch.Name)
	dimColor.Println("  " + strings.Repeat("─", 40))
	fmt.Println()

	// Basic info
	if branch.IsDefault {
		successColor.Println("  Default branch")
		fmt.Println()
	}

	infoColor.Print("  Commits:     ")
	fmt.Printf("%d\n", branch.CommitCount)

	if branch.BaseBranch != "" {
		infoColor.Print("  Base branch: ")
		fmt.Printf("%s\n", branch.BaseBranch)
	}

	if branch.Status != "" {
		infoColor.Print("  Status:      ")
		fmt.Printf("%s\n", branch.Status)
	}

	// Timestamps
	if !branch.CreatedAt.IsZero() {
		dimColor.Print("  Created:     ")
		dimColor.Printf("%s\n", branch.CreatedAt.Format("Jan 2, 2006"))
	}

	if !branch.UpdatedAt.IsZero() {
		dimColor.Print("  Updated:     ")
		dimColor.Printf("%s\n", branch.UpdatedAt.Format("Jan 2, 2006 15:04"))
	}

	// Story
	if branch.Story != "" {
		fmt.Println()
		titleColor.Println("  Story")
		fmt.Println()
		// Format story with indentation
		lines := strings.Split(branch.Story, "\n")
		for _, line := range lines {
			infoColor.Printf("  %s\n", line)
		}
	}

	fmt.Println()
	titleColor.Println("  Recent Commits")
	fmt.Println()

	commits, err := dbRepo.GetBranchCommits(ctx, branch.ID, 5)
	if err == nil && len(commits) > 0 {
		for _, c := range commits {
			msg := strings.Split(c.Message, "\n")[0]
			if len(msg) > 60 {
				msg = msg[:57] + "..."
			}
			dimColor.Printf("  %s ", c.CommittedAt.Format("Jan 2"))
			infoColor.Printf("%s ", c.Hash[:7])
			fmt.Printf("%s\n", msg)
		}
	} else {
		dimColor.Println("  No commits found")
	}

	fmt.Println()

	return nil
}

func runBranchStory(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	successColor := color.New(color.FgHiGreen)
	dimColor := color.New(color.FgHiBlack)

	branchName := args[0]

	codebasePath := branchCodebase
	if codebasePath == "" {
		codebasePath, _ = filepath.Abs(".")
	}

	dbRepo, err := db.GetRepository()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	codebase, err := dbRepo.GetCodebaseByPath(ctx, codebasePath)
	if err != nil {
		return fmt.Errorf("failed to get codebase: %w", err)
	}

	if codebase == nil {
		return fmt.Errorf("codebase not indexed. Run 'devlog ingest' first")
	}

	branch, err := dbRepo.GetBranch(ctx, codebase.ID, branchName)
	if err != nil {
		return fmt.Errorf("failed to get branch: %w", err)
	}

	if branch == nil {
		return fmt.Errorf("branch '%s' not found. Has it been ingested?", branchName)
	}

	story := branchStoryMessage

	if story == "" {
		fmt.Println()
		dimColor.Printf("  Current story for '%s':\n", branchName)
		if branch.Story != "" {
			fmt.Printf("  %s\n", branch.Story)
		} else {
			dimColor.Println("  (none)")
		}
		fmt.Println()

		prompt := promptui.Prompt{Label: "Enter new story", Default: branch.Story}
		story, err = prompt.Run()
		if err != nil {
			return fmt.Errorf("cancelled")
		}
	}

	branch.Story = story
	branch.UpdatedAt = time.Now()

	if err := dbRepo.UpsertBranch(ctx, branch); err != nil {
		return fmt.Errorf("failed to update branch: %w", err)
	}

	fmt.Println()
	successColor.Printf("  Updated story for '%s'\n", branchName)
	fmt.Println()

	return nil
}

func runBranchSetDefault(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	successColor := color.New(color.FgHiGreen)

	branchName := args[0]

	codebasePath := branchCodebase
	if codebasePath == "" {
		codebasePath, _ = filepath.Abs(".")
	}

	dbRepo, err := db.GetRepository()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	codebase, err := dbRepo.GetCodebaseByPath(ctx, codebasePath)
	if err != nil {
		return fmt.Errorf("failed to get codebase: %w", err)
	}

	if codebase == nil {
		return fmt.Errorf("codebase not indexed. Run 'devlog ingest' first")
	}

	branch, err := dbRepo.GetBranch(ctx, codebase.ID, branchName)
	if err != nil {
		return fmt.Errorf("failed to get branch: %w", err)
	}

	if branch == nil {
		return fmt.Errorf("branch '%s' not found. Has it been ingested?", branchName)
	}

	if err := dbRepo.ClearDefaultBranch(ctx, codebase.ID); err != nil {
		return fmt.Errorf("failed to clear default branch: %w", err)
	}

	branch.IsDefault = true
	branch.UpdatedAt = time.Now()
	if err := dbRepo.UpsertBranch(ctx, branch); err != nil {
		return fmt.Errorf("failed to update branch: %w", err)
	}

	codebase.DefaultBranch = branchName
	if err := dbRepo.UpsertCodebase(ctx, codebase); err != nil {
		return fmt.Errorf("failed to update codebase: %w", err)
	}

	fmt.Println()
	successColor.Printf("  Set '%s' as default branch\n", branchName)
	fmt.Println()

	return nil
}
