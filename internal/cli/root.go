package cli

import (
	"fmt"
	"os"

	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/db"
	"github.com/spf13/cobra"
)

var (
	dbPath      string
	verbose     bool
	profileFlag string
)

var rootCmd = &cobra.Command{
	Use:   "devlog",
	Short: "DevLog - Track and analyze your development activity",
	Long: `DevLog is a CLI tool that helps you track your development activity
by analyzing git commit history and providing insights through natural language queries.

Use 'devlog ingest' to scan a repository and 'devlog ask' to query your activity.

Profiles allow you to maintain separate databases for different work contexts.
Use 'devlog profile' to manage profiles.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip profile setup for onboard command (it handles its own setup)
		if cmd.Name() == "onboard" {
			return nil
		}

		// If custom DB path is set, use it (legacy behavior)
		if dbPath != "" {
			db.SetDBPath(dbPath)
			return nil
		}

		// Load config to get profile settings
		cfg, err := config.Load()
		if err != nil {
			VerboseLog("Warning: failed to load config: %v", err)
			// Continue with defaults
			cfg = &config.Config{}
		}

		// Migrate old database if needed
		if err := config.MigrateOldDB(); err != nil {
			VerboseLog("Warning: failed to migrate old database: %v", err)
		}

		// Ensure default profile exists
		if err := cfg.EnsureDefaultProfile(); err != nil {
			VerboseLog("Warning: failed to ensure default profile: %v", err)
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
			VerboseLog("Warning: failed to save config: %v", err)
		}

		return nil
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "Custom database path (overrides profile)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().StringVar(&profileFlag, "profile", "", "Use specific profile (overrides active profile)")
}

func IsVerbose() bool {
	return verbose
}

func VerboseLog(format string, args ...interface{}) {
	if verbose {
		fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
	}
}
