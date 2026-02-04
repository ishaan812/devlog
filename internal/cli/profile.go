package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/db"
)

var (
	deleteProfileData bool
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage devlog profiles",
	Long: `Manage devlog profiles (work contexts with isolated data).

Profiles allow you to maintain separate databases for different work contexts,
such as personal projects vs. work projects.

Without a subcommand, shows the current active profile.`,
	RunE: runProfileShow,
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all profiles",
	Long:  `List all profiles. The active profile is marked with (*).`,
	RunE:  runProfileList,
}

var profileCreateCmd = &cobra.Command{
	Use:   "create <name> [description]",
	Short: "Create a new profile",
	Long:  `Create a new profile with the given name and optional description.`,
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runProfileCreate,
}

var profileUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Switch to a profile",
	Long:  `Switch to an existing profile. All subsequent commands will use this profile.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runProfileUse,
}

var profileDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a profile",
	Long:  `Delete a profile. Use --data to also delete the profile's database.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runProfileDelete,
}

var profileReposCmd = &cobra.Command{
	Use:   "repos",
	Short: "List repositories in the active profile",
	Long:  `List all repositories that have been ingested in the active profile.`,
	RunE:  runProfileRepos,
}

func init() {
	rootCmd.AddCommand(profileCmd)
	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileCreateCmd)
	profileCmd.AddCommand(profileUseCmd)
	profileCmd.AddCommand(profileDeleteCmd)
	profileCmd.AddCommand(profileReposCmd)

	profileDeleteCmd.Flags().BoolVar(&deleteProfileData, "data", false, "Also delete the profile's database")
}

func runProfileShow(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	titleColor := color.New(color.FgHiCyan, color.Bold)
	infoColor := color.New(color.FgWhite)
	dimColor := color.New(color.FgHiBlack)

	profileName := cfg.GetActiveProfileName()
	profile := cfg.GetActiveProfile()

	titleColor.Println("\nActive Profile")
	fmt.Println()

	infoColor.Printf("  Name: %s\n", profileName)

	if profile != nil {
		if profile.Description != "" {
			infoColor.Printf("  Description: %s\n", profile.Description)
		}
		dimColor.Printf("  Created: %s\n", profile.CreatedAt)
		infoColor.Printf("  Repositories: %d\n", len(profile.Repos))
	}

	dbPath := config.GetProfileDBPath(profileName)
	if info, err := os.Stat(dbPath); err == nil {
		dimColor.Printf("  Database: %s (%.2f MB)\n", dbPath, float64(info.Size())/(1024*1024))
	} else {
		dimColor.Printf("  Database: %s (not created)\n", dbPath)
	}

	fmt.Println()
	return nil
}

func runProfileList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	titleColor := color.New(color.FgHiCyan, color.Bold)
	activeColor := color.New(color.FgHiGreen, color.Bold)
	infoColor := color.New(color.FgWhite)
	dimColor := color.New(color.FgHiBlack)

	titleColor.Println("\nProfiles")
	fmt.Println()

	if len(cfg.Profiles) == 0 {
		dimColor.Println("  No profiles found. Run 'devlog onboard' to create one.")
		fmt.Println()
		return nil
	}

	// Sort profile names
	names := cfg.ListProfiles()
	sort.Strings(names)

	activeProfile := cfg.GetActiveProfileName()

	for _, name := range names {
		profile := cfg.Profiles[name]
		isActive := name == activeProfile

		if isActive {
			activeColor.Printf("  (*) %s", name)
		} else {
			infoColor.Printf("      %s", name)
		}

		if profile.Description != "" {
			dimColor.Printf(" - %s", profile.Description)
		}

		dimColor.Printf(" (%d repos)\n", len(profile.Repos))
	}

	fmt.Println()
	return nil
}

func runProfileCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	description := ""
	if len(args) > 1 {
		description = args[1]
	}

	// Validate name
	if strings.ContainsAny(name, "/\\:*?\"<>|") {
		return fmt.Errorf("profile name cannot contain special characters: /\\:*?\"<>|")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.CreateProfile(name, description); err != nil {
		return err
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	successColor := color.New(color.FgHiGreen)
	successColor.Printf("Created profile '%s'\n", name)
	fmt.Printf("Use 'devlog profile use %s' to switch to it\n", name)

	return nil
}

func runProfileUse(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.SetActiveProfile(name); err != nil {
		return err
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Update the DB manager's active profile
	db.SetActiveProfile(name)

	successColor := color.New(color.FgHiGreen)
	successColor.Printf("Switched to profile '%s'\n", name)

	return nil
}

func runProfileDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Close any open database connection for this profile
	db.CloseDB(name)

	if err := cfg.DeleteProfile(name, deleteProfileData); err != nil {
		return err
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	successColor := color.New(color.FgHiGreen)
	if deleteProfileData {
		successColor.Printf("Deleted profile '%s' and its data\n", name)
	} else {
		successColor.Printf("Deleted profile '%s'\n", name)
		dimColor := color.New(color.FgHiBlack)
		dimColor.Printf("Note: Database files were not deleted. Use --data to delete them.\n")
	}

	return nil
}

func runProfileRepos(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	titleColor := color.New(color.FgHiCyan, color.Bold)
	infoColor := color.New(color.FgWhite)
	dimColor := color.New(color.FgHiBlack)

	profileName := cfg.GetActiveProfileName()
	profile := cfg.GetActiveProfile()

	titleColor.Printf("\nRepositories in '%s'\n", profileName)
	fmt.Println()

	if profile == nil || len(profile.Repos) == 0 {
		dimColor.Println("  No repositories in this profile.")
		dimColor.Println("  Use 'devlog ingest <path>' to add a repository.")
		fmt.Println()
		return nil
	}

	for i, repo := range profile.Repos {
		// Check if the repo path still exists
		exists := true
		if _, err := os.Stat(repo); os.IsNotExist(err) {
			exists = false
		}

		repoName := filepath.Base(repo)
		if exists {
			infoColor.Printf("  %d. %s\n", i+1, repoName)
			dimColor.Printf("     %s\n", repo)
		} else {
			dimColor.Printf("  %d. %s (missing)\n", i+1, repoName)
			dimColor.Printf("     %s\n", repo)
		}
	}

	fmt.Println()
	return nil
}
