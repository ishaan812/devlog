package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/db"
	"github.com/ishaan812/devlog/internal/tui"
)

var consoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Interactive console to browse worklogs",
	Long: `Opens a full-screen terminal UI to browse repositories and cached day-by-day worklogs.

Navigate between repos, dates, and rendered worklog content using keyboard shortcuts.
Requires at least one prior 'devlog worklog' run to populate the cache.

Examples:
  devlog console`,
	RunE: runConsole,
}

func init() {
	rootCmd.AddCommand(consoleCmd)
}

func runConsole(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w\n\nRun 'devlog onboard' to set up your configuration", err)
	}

	profileName := cfg.GetActiveProfileName()

	// Use read-only connection so we don't conflict with write operations
	dbRepo, err := db.GetReadOnlyRepositoryForProfile(profileName)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Load all codebases
	codebases, err := dbRepo.GetAllCodebases(ctx)
	if err != nil {
		return fmt.Errorf("failed to load codebases: %w", err)
	}

	if len(codebases) == 0 {
		fmt.Println("\n  No repositories found. Run 'devlog ingest' first to index a repository.")
		return nil
	}

	// Build console data: for each codebase, get worklog dates
	var consoleCodebases []tui.ConsoleCodebase
	for _, cb := range codebases {
		dates, err := dbRepo.ListWorklogDates(ctx, cb.ID, profileName)
		if err != nil {
			return fmt.Errorf("failed to list worklog dates for %s: %w", cb.Name, err)
		}

		tuiDates := make([]tui.ConsoleDate, len(dates))
		for i, d := range dates {
			tuiDates[i] = tui.ConsoleDate{
				EntryDate:   d.EntryDate,
				EntryCount:  d.EntryCount,
				CommitCount: d.CommitCount,
				Additions:   d.Additions,
				Deletions:   d.Deletions,
			}
		}

		// Check if repository has been ingested
		commitCount, err := dbRepo.GetCommitCount(ctx, cb.ID)
		if err != nil {
			commitCount = 0
		}

		consoleCodebases = append(consoleCodebases, tui.ConsoleCodebase{
			ID:          cb.ID,
			Name:        cb.Name,
			Path:        cb.Path,
			DateCount:   len(dates),
			Dates:       tuiDates,
			CommitCount: int(commitCount),
			IsIngested:  commitCount > 0,
		})
	}

	return tui.RunConsole(consoleCodebases, profileName, dbRepo)
}
