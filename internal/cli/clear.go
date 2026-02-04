package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/db"
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
	ctx := context.Background()
	warnColor := color.New(color.FgHiYellow, color.Bold)
	successColor := color.New(color.FgHiGreen)
	dimColor := color.New(color.FgHiBlack)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	profileName := clearProfile
	if profileName == "" {
		profileName = cfg.GetActiveProfileName()
	}

	db.SetActiveProfile(profileName)
	dbRepo, err := db.GetRepository()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	getCount := func(query string) int64 {
		results, _ := dbRepo.ExecuteQuery(ctx, query)
		if len(results) > 0 {
			if v, ok := results[0]["cnt"].(int64); ok {
				return v
			}
		}
		return 0
	}

	commitCount := getCount("SELECT COUNT(*) as cnt FROM commits")
	fileChangeCount := getCount("SELECT COUNT(*) as cnt FROM file_changes")
	codebaseCount := getCount("SELECT COUNT(*) as cnt FROM codebases")
	fileIndexCount := getCount("SELECT COUNT(*) as cnt FROM file_indexes")

	fmt.Println()
	warnColor.Printf("  Warning: Clear Database\n\n")
	dimColor.Printf("  Profile: %s\n\n", profileName)
	dimColor.Println("  This will delete:")
	fmt.Printf("    %d commits\n", commitCount)
	fmt.Printf("    %d file changes\n", fileChangeCount)
	fmt.Printf("    %d codebases\n", codebaseCount)
	fmt.Printf("    %d file indexes\n", fileIndexCount)
	fmt.Println()

	if commitCount == 0 && codebaseCount == 0 {
		dimColor.Println("  Database is already empty.")
		fmt.Println()
		return nil
	}

	if !clearForce {
		warnColor.Print("  Are you sure you want to delete all data? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			fmt.Println()
			dimColor.Println("  Canceled.")
			fmt.Println()
			return nil
		}
	}

	fmt.Println()
	dimColor.Println("  Clearing data...")

	tables := []string{
		"file_changes", "ingest_cursors", "commits", "branches",
		"file_indexes", "folders", "codebases", "developers",
	}

	database := dbRepo.DB()
	for _, table := range tables {
		if _, err := database.Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			VerboseLog("Warning: failed to clear %s: %v", table, err)
		}
	}
	database.Exec("CHECKPOINT")

	fmt.Println()
	successColor.Printf("  Database cleared successfully\n")
	fmt.Println()

	return nil
}
