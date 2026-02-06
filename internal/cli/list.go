package cli

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/db"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List profiles and repositories",
	Long: `List all profiles and their associated repositories.

Shows the active profile, all configured profiles, and the repositories
that have been ingested into each profile.

Examples:
  devlog list              # List all profiles and repos
  devlog list profiles     # List only profiles
  devlog list repos        # List repos in current profile`,
	RunE: runList,
}

var listProfilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "List all profiles",
	RunE:  runListProfiles,
}

var listReposCmd = &cobra.Command{
	Use:   "repos",
	Short: "List repositories in current profile",
	RunE:  runListRepos,
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.AddCommand(listProfilesCmd)
	listCmd.AddCommand(listReposCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	titleColor := color.New(color.FgHiCyan, color.Bold)
	activeColor := color.New(color.FgHiGreen)
	dimColor := color.New(color.FgHiBlack)
	infoColor := color.New(color.FgHiWhite)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Println()
	titleColor.Printf("  DevLog Overview\n\n")

	// Show active profile
	activeProfile := cfg.GetActiveProfileName()
	dimColor.Print("  Active Profile: ")
	activeColor.Printf("%s\n\n", activeProfile)

	// Show all profiles
	titleColor.Printf("  Profiles\n")
	profiles := cfg.ListProfiles()
	if len(profiles) == 0 {
		dimColor.Println("    No profiles configured")
	} else {
		for _, name := range profiles {
			profile := cfg.Profiles[name]
			if name == activeProfile {
				activeColor.Printf("  → %s", name)
			} else {
				fmt.Printf("    %s", name)
			}

			if profile.Description != "" {
				dimColor.Printf(" - %s", profile.Description)
			}
			fmt.Println()

			// Show repos in this profile
			if len(profile.Repos) > 0 {
				for _, repo := range profile.Repos {
					dimColor.Printf("      • %s\n", repo)
				}
			}

			db.SetActiveProfile(name)
			dbRepo, err := db.GetRepository()
			if err != nil {
				return fmt.Errorf("failed to get repository for profile %s: %w", name, err)
			}
			ctx := context.Background()
			commitResults, err := dbRepo.ExecuteQuery(ctx, `SELECT COUNT(*) as cnt FROM commits`)
			if err != nil {
				return fmt.Errorf("failed to query commit count for profile %s: %w", name, err)
			}
			codebaseResults, err := dbRepo.ExecuteQuery(ctx, `SELECT COUNT(*) as cnt FROM codebases`)
			if err != nil {
				return fmt.Errorf("failed to query codebase count for profile %s: %w", name, err)
			}
			var commitCount, codebaseCount int64
			if len(commitResults) > 0 {
				if v, ok := commitResults[0]["cnt"].(int64); ok {
					commitCount = v
				}
			}
			if len(codebaseResults) > 0 {
				if v, ok := codebaseResults[0]["cnt"].(int64); ok {
					codebaseCount = v
				}
			}
			if commitCount > 0 || codebaseCount > 0 {
				dimColor.Printf("      ")
				infoColor.Printf("%d commits", commitCount)
				dimColor.Printf(" in ")
				infoColor.Printf("%d repos\n", codebaseCount)
			}
		}
	}

	// Show GitHub username if configured
	fmt.Println()
	dimColor.Print("  GitHub Username: ")
	if cfg.GitHubUsername != "" {
		infoColor.Printf("%s\n", cfg.GitHubUsername)
	} else {
		dimColor.Println("(not set)")
	}

	// Show LLM provider
	dimColor.Print("  LLM Provider: ")
	if cfg.DefaultProvider != "" {
		infoColor.Printf("%s\n", cfg.DefaultProvider)
	} else {
		dimColor.Println("ollama (default)")
	}

	fmt.Println()
	return nil
}

func runListProfiles(cmd *cobra.Command, args []string) error {
	titleColor := color.New(color.FgHiCyan, color.Bold)
	activeColor := color.New(color.FgHiGreen)
	dimColor := color.New(color.FgHiBlack)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Println()
	titleColor.Printf("  Profiles\n\n")

	activeProfile := cfg.GetActiveProfileName()
	profiles := cfg.ListProfiles()

	if len(profiles) == 0 {
		dimColor.Println("  No profiles configured")
		fmt.Println()
		return nil
	}

	for _, name := range profiles {
		profile := cfg.Profiles[name]
		if name == activeProfile {
			activeColor.Printf("  → %s", name)
			dimColor.Print(" (active)")
		} else {
			fmt.Printf("    %s", name)
		}

		if profile.Description != "" {
			dimColor.Printf(" - %s", profile.Description)
		}
		fmt.Println()
	}

	fmt.Println()
	return nil
}

func runListRepos(cmd *cobra.Command, args []string) error {
	titleColor := color.New(color.FgHiCyan, color.Bold)
	dimColor := color.New(color.FgHiBlack)
	infoColor := color.New(color.FgHiWhite)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	profileName := cfg.GetActiveProfileName()
	profile := cfg.GetActiveProfile()

	fmt.Println()
	titleColor.Printf("  Repositories in '%s'\n\n", profileName)

	if profile == nil || len(profile.Repos) == 0 {
		dimColor.Println("  No repositories ingested yet.")
		dimColor.Println("  Run 'devlog ingest' in a git repository to get started.")
		fmt.Println()
		return nil
	}

	ctx := context.Background()
	db.SetActiveProfile(profileName)
	dbRepo, err := db.GetRepository()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	for _, repoPath := range profile.Repos {
		infoColor.Printf("  • %s\n", repoPath)
		codebase, err := dbRepo.GetCodebaseByPath(ctx, repoPath)
		if err != nil {
			return fmt.Errorf("failed to get codebase for %s: %w", repoPath, err)
		}
		if codebase != nil {
			commitCount, err := dbRepo.GetCommitCount(ctx, codebase.ID)
			if err != nil {
				return fmt.Errorf("failed to get commit count for %s: %w", repoPath, err)
			}
			fileCount, err := dbRepo.GetFileChangeCount(ctx, codebase.ID)
			if err != nil {
				return fmt.Errorf("failed to get file change count for %s: %w", repoPath, err)
			}
			dimColor.Printf("    %d commits, %d file changes\n", commitCount, fileCount)
			if codebase.Summary != "" {
				dimColor.Printf("    %s\n", truncate(codebase.Summary, 60))
			}
		}
	}

	fmt.Println()
	return nil
}
