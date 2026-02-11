package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/db"
)

var (
	verbose     bool
	profileFlag string
)

var rootCmd = &cobra.Command{
	Use:   "devlog",
	Short: "DevLog - Track and analyze your development activity",
	Long: `DevLog is a CLI tool that helps you track your development activity
by analyzing git commit history and providing insights through natural language queries.

Use 'devlog ingest' to scan a repository and 'devlog worklog' to view your activity.

Profiles allow you to maintain separate databases for different work contexts.
Use 'devlog profile' to manage profiles.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip profile setup for commands that don't need it
		if cmd.Name() == "onboard" || cmd.Name() == "update" {
			return nil
		}

		// Load config to get profile settings
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Migrate old database if needed
		if err := config.MigrateOldDB(); err != nil {
			return fmt.Errorf("failed to migrate old database: %w", err)
		}

		// Ensure default profile exists
		if err := cfg.EnsureDefaultProfile(); err != nil {
			return fmt.Errorf("failed to ensure default profile: %w", err)
		}

		// Determine which profile to use
		profileName := cfg.GetActiveProfileName()
		if profileFlag != "" {
			// Override with command-line flag
			if cfg.Profiles != nil && cfg.Profiles[profileFlag] != nil {
				profileName = profileFlag
			} else {
				return fmt.Errorf("profile '%s' not found", profileFlag)
			}
		}

		// Set active profile for DB operations
		db.SetActiveProfile(profileName)

		// Save config if we created the default profile
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		return nil
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().StringVar(&profileFlag, "profile", "", "Use specific profile (overrides active profile)")
}

func IsVerbose() bool {
	return verbose
}

func VerboseLog(format string, args ...any) {
	if verbose {
		fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
	}
}
