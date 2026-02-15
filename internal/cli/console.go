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
	Long: `Opens a full-screen terminal UI to browse repositories and cached worklogs.

Navigate between repos and a hierarchical timeline (months > weeks > days) of your work.
View daily entries, weekly summaries, and monthly summaries directly in the console.
Requires at least one prior 'devlog worklog' run to populate the cache.

Timeline Features:
  - Expandable/collapsible months and weeks (press Enter to toggle)
  - Weekly summaries generated automatically for worklogs spanning >7 days
  - Hierarchical navigation shows your work at different time scales

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

	// Build console data: for each codebase, get worklog dates, weeks, and months
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

		// Load weeks
		weeks, err := dbRepo.ListWorklogWeeks(ctx, cb.ID, profileName)
		if err != nil {
			weeks = nil // Non-fatal, just won't show weeks
		}

		tuiWeeks := make([]tui.ConsoleWeek, len(weeks))
		for i, w := range weeks {
			// Find dates that belong to this week
			var weekDates []tui.ConsoleDate
			for _, d := range tuiDates {
				if !d.EntryDate.Before(w.WeekStart) && !d.EntryDate.After(w.WeekEnd) {
					weekDates = append(weekDates, d)
				}
			}

			tuiWeeks[i] = tui.ConsoleWeek{
				WeekStart:   w.WeekStart,
				WeekEnd:     w.WeekEnd,
				DateCount:   w.DateCount,
				EntryCount:  w.EntryCount,
				CommitCount: w.CommitCount,
				Additions:   w.Additions,
				Deletions:   w.Deletions,
				Dates:       weekDates,
			}
		}

		// Load months
		months, err := dbRepo.ListWorklogMonths(ctx, cb.ID, profileName)
		if err != nil {
			months = nil // Non-fatal, just won't show months
		}

		tuiMonths := make([]tui.ConsoleMonth, len(months))
		for i, m := range months {
			// Find weeks that belong to this month
			var monthWeeks []tui.ConsoleWeek
			for _, w := range tuiWeeks {
				if !w.WeekStart.Before(m.MonthStart) && !w.WeekStart.After(m.MonthEnd) {
					monthWeeks = append(monthWeeks, w)
				}
			}

			tuiMonths[i] = tui.ConsoleMonth{
				MonthStart:  m.MonthStart,
				MonthEnd:    m.MonthEnd,
				DateCount:   m.DateCount,
				WeekCount:   m.WeekCount,
				EntryCount:  m.EntryCount,
				CommitCount: m.CommitCount,
				Additions:   m.Additions,
				Deletions:   m.Deletions,
				Weeks:       monthWeeks,
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
			Weeks:       tuiWeeks,
			Months:      tuiMonths,
			CommitCount: int(commitCount),
			IsIngested:  commitCount > 0,
		})
	}

	return tui.RunConsole(consoleCodebases, profileName, dbRepo)
}
