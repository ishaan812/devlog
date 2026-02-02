package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/db"
	"github.com/spf13/cobra"
)

var (
	clearForce   bool
	clearProfile string
)

var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear database data",
	Long: `Clear all data from the database for the current profile.

This will delete all ingested commits, file changes, codebases, and indexes.
Use with caution - this action cannot be undone.

Examples:
  devlog clear                    # Clear current profile (with confirmation)
  devlog clear --force            # Skip confirmation
  devlog clear --profile work     # Clear specific profile`,
	RunE: runClear,
}

func init() {
	rootCmd.AddCommand(clearCmd)

	clearCmd.Flags().BoolVarP(&clearForce, "force", "f", false, "Skip confirmation prompt")
	clearCmd.Flags().StringVar(&clearProfile, "profile", "", "Profile to clear (default: active profile)")
}

func runClear(cmd *cobra.Command, args []string) error {
	warnColor := color.New(color.FgHiYellow, color.Bold)
	successColor := color.New(color.FgHiGreen)
	dimColor := color.New(color.FgHiBlack)

	// Load config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Determine which profile to clear
	profileName := clearProfile
	if profileName == "" {
		profileName = cfg.GetActiveProfileName()
	}

	// Get current stats before clearing
	db.SetActiveProfile(profileName)
	database, err := db.GetDB()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Count current data
	var commitCount, fileChangeCount, codebaseCount, fileIndexCount int
	database.QueryRow(`SELECT COUNT(*) FROM commits`).Scan(&commitCount)
	database.QueryRow(`SELECT COUNT(*) FROM file_changes`).Scan(&fileChangeCount)
	database.QueryRow(`SELECT COUNT(*) FROM codebases`).Scan(&codebaseCount)
	database.QueryRow(`SELECT COUNT(*) FROM file_indexes`).Scan(&fileIndexCount)

	fmt.Println()
	warnColor.Printf("  ⚠️  Clear Database\n\n")
	dimColor.Printf("  Profile: %s\n\n", profileName)
	dimColor.Println("  This will delete:")
	fmt.Printf("    • %d commits\n", commitCount)
	fmt.Printf("    • %d file changes\n", fileChangeCount)
	fmt.Printf("    • %d codebases\n", codebaseCount)
	fmt.Printf("    • %d file indexes\n", fileIndexCount)
	fmt.Println()

	if commitCount == 0 && codebaseCount == 0 {
		dimColor.Println("  Database is already empty.")
		fmt.Println()
		return nil
	}

	// Confirmation prompt
	if !clearForce {
		warnColor.Print("  Are you sure you want to delete all data? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			fmt.Println()
			dimColor.Println("  Cancelled.")
			fmt.Println()
			return nil
		}
	}

	// Clear data in correct order (respecting foreign keys)
	fmt.Println()
	dimColor.Println("  Clearing data...")

	// Delete in reverse dependency order
	tables := []string{
		"file_changes",
		"ingest_cursors",
		"commits",
		"branches",
		"file_indexes",
		"folders",
		"codebases",
		"developers",
	}

	for _, table := range tables {
		_, err := database.Exec(fmt.Sprintf("DELETE FROM %s", table))
		if err != nil {
			VerboseLog("Warning: failed to clear %s: %v", table, err)
		}
	}

	// Checkpoint to ensure changes are persisted
	database.Exec("CHECKPOINT")

	fmt.Println()
	successColor.Printf("  ✓ Database cleared successfully\n")
	fmt.Println()

	return nil
}
