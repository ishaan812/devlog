package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/db"
	"github.com/ishaan812/devlog/internal/tui"
)

var (
	obsidianVaultPath  string
	obsidianRepoPath   string
	obsidianRootFolder string
	obsidianDryRun     bool
	obsidianForce      bool
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export cached worklogs to external formats",
	Long: `Export cached worklog entries to external destinations.

Use subcommands to export in specific formats (for example Obsidian).`,
}

var exportObsidianCmd = &cobra.Command{
	Use:   "obsidian",
	Short: "Export cached worklogs to an Obsidian vault",
	Long: `Export cached worklog entries to well-organized Obsidian markdown files.

This command exports cached entries only:
  - daily logs (day_updates)
  - weekly summaries (week_summary)
  - monthly summaries (month_summary)

It writes only new/changed entries by default using export signatures.
Use --force to rewrite all files.`,
	RunE: runExportObsidian,
}

var exportObsidianStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Obsidian export coverage and pending diffs",
	RunE:  runExportObsidianStatus,
}

var exportObsidianConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Show or update saved Obsidian export settings",
	RunE:  runExportObsidianConfig,
}

func init() {
	rootCmd.AddCommand(exportCmd)
	exportCmd.AddCommand(exportObsidianCmd)
	exportObsidianCmd.AddCommand(exportObsidianStatusCmd)
	exportObsidianCmd.AddCommand(exportObsidianConfigCmd)

	exportObsidianCmd.PersistentFlags().StringVar(&obsidianVaultPath, "vault", "", "Path to Obsidian vault (saved per profile + repo)")
	exportObsidianCmd.PersistentFlags().StringVar(&obsidianRepoPath, "repo", ".", "Repository path to export from")
	exportObsidianCmd.PersistentFlags().StringVar(&obsidianRootFolder, "root", "", "Root folder inside vault (default: DevLog)")
	exportObsidianCmd.PersistentFlags().BoolVar(&obsidianDryRun, "dry-run", false, "Show what would be exported without writing files")
	exportObsidianCmd.PersistentFlags().BoolVar(&obsidianForce, "force", false, "Rewrite all entries even if already exported")
}

type obsidianExportContext struct {
	cfg         *config.Config
	dbRepo      *db.SQLRepository
	codebase    *db.Codebase
	repoPath    string
	repoName    string
	profileName string
	vaultPath   string
	rootFolder  string
	loc         *time.Location
}

type obsidianExportItem struct {
	EntryType    string
	EntryDate    time.Time
	BranchID     string
	Signature    string
	RelativePath string
	Markdown     string
}

type obsidianExportSummary struct {
	Scanned            int
	Exported           int
	SkippedUnchanged   int
	PendingInDryRun    int
	ByTypeScanned      map[string]int
	ByTypeExported     map[string]int
	ByTypeSkipped      map[string]int
	ByTypePending      map[string]int
	LastExportedAtByTy map[string]time.Time
}

func runExportObsidian(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exportCtx, err := resolveObsidianExportContext(true, true)
	if err != nil {
		return err
	}

	items, err := buildObsidianExportItems(ctx, exportCtx)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Println("No cached worklog entries found to export.")
		fmt.Println("Run `devlog worklog --days <n>` first to populate the cache.")
		return nil
	}

	summary, err := applyObsidianExport(ctx, exportCtx, items, obsidianDryRun, obsidianForce)
	if err != nil {
		return err
	}

	if obsidianDryRun {
		fmt.Printf("Dry run complete for vault: %s\n", exportCtx.vaultPath)
		fmt.Printf("Scanned %d entries, would export %d, unchanged %d\n",
			summary.Scanned, summary.PendingInDryRun, summary.SkippedUnchanged)
	} else {
		fmt.Printf("Export complete: %s\n", exportCtx.vaultPath)
		fmt.Printf("Scanned %d entries, exported %d, unchanged %d\n",
			summary.Scanned, summary.Exported, summary.SkippedUnchanged)
	}
	printTypeBreakdown(summary)
	return nil
}

func runExportObsidianStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exportCtx, err := resolveObsidianExportContext(false, false)
	if err != nil {
		return err
	}

	items, err := buildObsidianExportItems(ctx, exportCtx)
	if err != nil {
		return err
	}

	summary := &obsidianExportSummary{
		ByTypeScanned:      make(map[string]int),
		ByTypeExported:     make(map[string]int),
		ByTypeSkipped:      make(map[string]int),
		ByTypePending:      make(map[string]int),
		LastExportedAtByTy: make(map[string]time.Time),
	}

	for _, item := range items {
		summary.Scanned++
		summary.ByTypeScanned[item.EntryType]++

		state, err := exportCtx.dbRepo.GetWorklogExportState(ctx, exportCtx.codebase.ID, exportCtx.profileName, item.EntryType, item.EntryDate, item.BranchID)
		if err != nil {
			return fmt.Errorf("failed to load export state: %w", err)
		}

		if exportEntryIsCurrent(exportCtx, item, state) {
			summary.SkippedUnchanged++
			summary.ByTypeSkipped[item.EntryType]++
			if state.ExportedAt.After(summary.LastExportedAtByTy[item.EntryType]) {
				summary.LastExportedAtByTy[item.EntryType] = state.ExportedAt
			}
			continue
		}
		summary.PendingInDryRun++
		summary.ByTypePending[item.EntryType]++
	}

	fmt.Printf("Obsidian export status for repo: %s\n", exportCtx.repoPath)
	if exportCtx.vaultPath == "" {
		fmt.Println("Vault path: not configured")
	} else {
		fmt.Printf("Vault path: %s\n", exportCtx.vaultPath)
	}
	fmt.Printf("Root folder: %s\n", exportCtx.rootFolder)
	fmt.Printf("Cached entries: %d\n", summary.Scanned)
	fmt.Printf("Up-to-date exports: %d\n", summary.SkippedUnchanged)
	fmt.Printf("Pending export diffs: %d\n", summary.PendingInDryRun)
	printStatusTypeBreakdown(summary)

	for _, t := range []string{"day_updates", "week_summary", "month_summary"} {
		if ts, ok := summary.LastExportedAtByTy[t]; ok && !ts.IsZero() {
			fmt.Printf("Last %s export: %s\n", t, ts.Format(time.RFC3339))
		}
	}
	return nil
}

func runExportObsidianConfig(cmd *cobra.Command, args []string) error {
	exportCtx, err := resolveObsidianExportContext(false, true)
	if err != nil {
		return err
	}
	fmt.Printf("Repo: %s\n", exportCtx.repoPath)
	if exportCtx.vaultPath == "" {
		fmt.Println("Vault path: not configured")
	} else {
		fmt.Printf("Vault path: %s\n", exportCtx.vaultPath)
	}
	fmt.Printf("Root folder: %s\n", exportCtx.rootFolder)
	return nil
}

func resolveObsidianExportContext(requireVault bool, persistSettings bool) (*obsidianExportContext, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	dbRepo, err := db.GetRepository()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	repoPath, err := filepath.Abs(obsidianRepoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve repo path: %w", err)
	}
	codebase, err := dbRepo.GetCodebaseByPath(context.Background(), repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to look up codebase: %w", err)
	}
	if codebase == nil {
		return nil, fmt.Errorf("no indexed repository found at %s\n\nRun `devlog ingest %s` first", repoPath, repoPath)
	}

	profileName := cfg.GetActiveProfileName()
	saved := cfg.GetObsidianVault(profileName, repoPath)
	resolvedVault := ""
	resolvedRoot := "Devlog"
	if saved != nil {
		resolvedVault = saved.VaultPath
		if strings.TrimSpace(saved.RootFolder) != "" {
			resolvedRoot = strings.TrimSpace(saved.RootFolder)
		}
	}

	if strings.TrimSpace(obsidianVaultPath) != "" {
		vaultAbs, err := filepath.Abs(obsidianVaultPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve vault path: %w", err)
		}
		resolvedVault = vaultAbs
	}
	if strings.TrimSpace(obsidianRootFolder) != "" {
		resolvedRoot = strings.TrimSpace(obsidianRootFolder)
	}

	if persistSettings && strings.TrimSpace(resolvedVault) != "" && (strings.TrimSpace(obsidianVaultPath) != "" || strings.TrimSpace(obsidianRootFolder) != "") {
		if err := cfg.SaveObsidianVault(profileName, repoPath, resolvedVault, resolvedRoot); err != nil {
			return nil, fmt.Errorf("failed to save obsidian settings: %w", err)
		}
		if err := cfg.Save(); err != nil {
			return nil, fmt.Errorf("failed to save config: %w", err)
		}
	}

	if requireVault && strings.TrimSpace(resolvedVault) == "" {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			promptedPath, promptErr := tui.RunPathPrompt(
				"Obsidian Vault Path",
				"Enter vault path and press Enter. Esc cancels.",
				"",
			)
			if promptErr != nil {
				return nil, fmt.Errorf("obsidian vault path prompt canceled")
			}
			vaultAbs, absErr := filepath.Abs(promptedPath)
			if absErr != nil {
				return nil, fmt.Errorf("failed to resolve vault path: %w", absErr)
			}
			resolvedVault = vaultAbs

			if persistSettings {
				if err := cfg.SaveObsidianVault(profileName, repoPath, resolvedVault, resolvedRoot); err != nil {
					return nil, fmt.Errorf("failed to save obsidian settings: %w", err)
				}
				if err := cfg.Save(); err != nil {
					return nil, fmt.Errorf("failed to save config: %w", err)
				}
			}
		}
		if strings.TrimSpace(resolvedVault) == "" {
			return nil, fmt.Errorf("obsidian vault path is not configured\n\nUse `devlog export obsidian --vault <path>` to set it")
		}
	}

	loc := getProfileTimezone(cfg)
	return &obsidianExportContext{
		cfg:         cfg,
		dbRepo:      dbRepo,
		codebase:    codebase,
		repoPath:    repoPath,
		repoName:    codebase.Name,
		profileName: profileName,
		vaultPath:   resolvedVault,
		rootFolder:  resolvedRoot,
		loc:         loc,
	}, nil
}

func buildObsidianExportItems(ctx context.Context, exportCtx *obsidianExportContext) ([]obsidianExportItem, error) {
	entries, err := exportCtx.dbRepo.ListWorklogEntriesForExport(ctx, exportCtx.codebase.ID, exportCtx.profileName)
	if err != nil {
		return nil, fmt.Errorf("failed to load cached worklog entries: %w", err)
	}

	dailyByDate := make(map[string][]db.WorklogEntry)
	var weekly []db.WorklogEntry
	var monthly []db.WorklogEntry

	for _, e := range entries {
		switch e.EntryType {
		case "day_updates":
			dateKey := e.EntryDate.In(exportCtx.loc).Format("2006-01-02")
			dailyByDate[dateKey] = append(dailyByDate[dateKey], e)
		case "week_summary":
			weekly = append(weekly, e)
		case "month_summary":
			monthly = append(monthly, e)
		}
	}

	var items []obsidianExportItem

	var dailyKeys []string
	for k := range dailyByDate {
		dailyKeys = append(dailyKeys, k)
	}
	sort.Strings(dailyKeys)
	for _, dateKey := range dailyKeys {
		dayEntries := dailyByDate[dateKey]
		if len(dayEntries) == 0 {
			continue
		}
		sort.Slice(dayEntries, func(i, j int) bool {
			return dayEntries[i].BranchName < dayEntries[j].BranchName
		})

		date := dayEntries[0].EntryDate.In(exportCtx.loc)
		relPath := filepath.Join(exportBasePath(exportCtx), "daily", date.Format("2006"), date.Format("01"), date.Format("2006-01-02")+".md")
		content := renderDailyObsidianMarkdown(exportCtx, date, dayEntries)
		sig := computeExportSignature("day_updates", date, "", dayEntries, content)

		items = append(items, obsidianExportItem{
			EntryType:    "day_updates",
			EntryDate:    date,
			BranchID:     "",
			Signature:    sig,
			RelativePath: relPath,
			Markdown:     content,
		})
	}

	sort.Slice(weekly, func(i, j int) bool {
		return weekly[i].EntryDate.Before(weekly[j].EntryDate)
	})
	for _, e := range weekly {
		weekStart := e.EntryDate.In(exportCtx.loc)
		relPath := filepath.Join(exportBasePath(exportCtx), "weekly", weekStart.Format("2006"), weekRangeNoteID(weekStart)+".md")
		content := renderWeeklyObsidianMarkdown(exportCtx, weekStart, e)
		sig := computeExportSignature("week_summary", weekStart, "", []db.WorklogEntry{e}, content)
		items = append(items, obsidianExportItem{
			EntryType:    "week_summary",
			EntryDate:    weekStart,
			BranchID:     "",
			Signature:    sig,
			RelativePath: relPath,
			Markdown:     content,
		})
	}

	sort.Slice(monthly, func(i, j int) bool {
		return monthly[i].EntryDate.Before(monthly[j].EntryDate)
	})
	for _, e := range monthly {
		monthStart := e.EntryDate.In(exportCtx.loc)
		relPath := filepath.Join(exportBasePath(exportCtx), "monthly", monthStart.Format("2006"), monthStart.Format("2006-01")+".md")
		content := renderMonthlyObsidianMarkdown(exportCtx, monthStart, e)
		sig := computeExportSignature("month_summary", monthStart, "", []db.WorklogEntry{e}, content)
		items = append(items, obsidianExportItem{
			EntryType:    "month_summary",
			EntryDate:    monthStart,
			BranchID:     "",
			Signature:    sig,
			RelativePath: relPath,
			Markdown:     content,
		})
	}

	return items, nil
}

func applyObsidianExport(ctx context.Context, exportCtx *obsidianExportContext, items []obsidianExportItem, dryRun, force bool) (*obsidianExportSummary, error) {
	summary := &obsidianExportSummary{
		ByTypeScanned:      make(map[string]int),
		ByTypeExported:     make(map[string]int),
		ByTypeSkipped:      make(map[string]int),
		ByTypePending:      make(map[string]int),
		LastExportedAtByTy: make(map[string]time.Time),
	}
	now := time.Now()

	for _, item := range items {
		summary.Scanned++
		summary.ByTypeScanned[item.EntryType]++

		existing, err := exportCtx.dbRepo.GetWorklogExportState(ctx, exportCtx.codebase.ID, exportCtx.profileName, item.EntryType, item.EntryDate, item.BranchID)
		if err != nil {
			return nil, fmt.Errorf("failed to read export state: %w", err)
		}
		upToDate := exportEntryIsCurrent(exportCtx, item, existing)
		if upToDate && !force {
			summary.SkippedUnchanged++
			summary.ByTypeSkipped[item.EntryType]++
			if existing.ExportedAt.After(summary.LastExportedAtByTy[item.EntryType]) {
				summary.LastExportedAtByTy[item.EntryType] = existing.ExportedAt
			}
			continue
		}

		if dryRun {
			summary.PendingInDryRun++
			summary.ByTypePending[item.EntryType]++
			continue
		}

		outPath := filepath.Join(exportCtx.vaultPath, item.RelativePath)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create export directory %s: %w", filepath.Dir(outPath), err)
		}
		if err := os.WriteFile(outPath, []byte(item.Markdown), 0644); err != nil {
			return nil, fmt.Errorf("failed to write exported file %s: %w", outPath, err)
		}

		stateID := exportStateID(exportCtx.codebase.ID, exportCtx.profileName, item.EntryType, item.EntryDate, item.BranchID)
		state := &db.WorklogExportState{
			ID:          stateID,
			CodebaseID:  exportCtx.codebase.ID,
			ProfileName: exportCtx.profileName,
			EntryType:   item.EntryType,
			EntryDate:   item.EntryDate,
			BranchID:    item.BranchID,
			Signature:   item.Signature,
			FilePath:    item.RelativePath,
			ExportedAt:  now,
		}
		if err := exportCtx.dbRepo.UpsertWorklogExportState(ctx, state); err != nil {
			return nil, fmt.Errorf("failed to save export state: %w", err)
		}

		summary.Exported++
		summary.ByTypeExported[item.EntryType]++
		summary.LastExportedAtByTy[item.EntryType] = now
	}

	return summary, nil
}

func renderDailyObsidianMarkdown(exportCtx *obsidianExportContext, date time.Time, entries []db.WorklogEntry) string {
	weekStart := getWeekStart(date, exportCtx.loc)
	weekRef := weekRangeNoteID(weekStart)
	monthRef := date.In(exportCtx.loc).Format("2006-01")

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("type: daily-worklog\n")
	sb.WriteString(fmt.Sprintf("date: %s\n", date.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("profile: %s\n", exportCtx.profileName))
	sb.WriteString(fmt.Sprintf("repo: %s\n", exportCtx.repoName))
	sb.WriteString(fmt.Sprintf("week: %s\n", weekStart.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("month: %s\n", monthRef))
	sb.WriteString("tags:\n")
	sb.WriteString("  - worklog\n")
	sb.WriteString("  - daily\n")
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("# Daily Worklog - %s\n\n", date.Format("Monday, January 2, 2006")))
	sb.WriteString(fmt.Sprintf("Week: [[%s]]\n", weekRef))
	sb.WriteString(fmt.Sprintf("Month: [[%s]]\n\n", monthRef))
	sb.WriteString("---\n\n")

	for _, entry := range entries {
		branch := strings.TrimSpace(entry.BranchName)
		if branch == "" {
			branch = "unknown"
		}
		sb.WriteString(fmt.Sprintf("## Branch: %s\n\n", branch))
		sb.WriteString(strings.TrimSpace(entry.Content))
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String()) + "\n"
}

func renderWeeklyObsidianMarkdown(exportCtx *obsidianExportContext, weekStart time.Time, entry db.WorklogEntry) string {
	weekEnd := weekStart.AddDate(0, 0, 6)
	monthRef := weekStart.In(exportCtx.loc).Format("2006-01")

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("type: weekly-summary\n")
	sb.WriteString(fmt.Sprintf("date: %s\n", weekStart.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("week_start: %s\n", weekStart.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("week_end: %s\n", weekEnd.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("profile: %s\n", exportCtx.profileName))
	sb.WriteString(fmt.Sprintf("repo: %s\n", exportCtx.repoName))
	sb.WriteString("tags:\n")
	sb.WriteString("  - worklog\n")
	sb.WriteString("  - weekly\n")
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("# Weekly Summary - %s to %s\n\n", weekStart.Format("Jan 2"), weekEnd.Format("Jan 2, 2006")))
	sb.WriteString(fmt.Sprintf("Month: [[%s]]\n\n", monthRef))
	sb.WriteString("---\n\n")
	sb.WriteString(strings.TrimSpace(entry.Content))
	sb.WriteString("\n")
	return strings.TrimSpace(sb.String()) + "\n"
}

func renderMonthlyObsidianMarkdown(exportCtx *obsidianExportContext, monthStart time.Time, entry db.WorklogEntry) string {
	monthRef := monthStart.In(exportCtx.loc).Format("2006-01")

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("type: monthly-summary\n")
	sb.WriteString(fmt.Sprintf("date: %s\n", monthStart.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("month: %s\n", monthRef))
	sb.WriteString(fmt.Sprintf("profile: %s\n", exportCtx.profileName))
	sb.WriteString(fmt.Sprintf("repo: %s\n", exportCtx.repoName))
	sb.WriteString("tags:\n")
	sb.WriteString("  - worklog\n")
	sb.WriteString("  - monthly\n")
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("# Monthly Summary - %s\n\n", monthStart.Format("January 2006")))
	sb.WriteString("---\n\n")
	sb.WriteString(strings.TrimSpace(entry.Content))
	sb.WriteString("\n")
	return strings.TrimSpace(sb.String()) + "\n"
}

func computeExportSignature(entryType string, entryDate time.Time, branchID string, entries []db.WorklogEntry, renderedMarkdown string) string {
	parts := []string{
		entryType,
		entryDate.Format("2006-01-02"),
		branchID,
	}
	for _, e := range entries {
		parts = append(parts, strings.Join([]string{
			e.EntryType,
			e.EntryDate.Format("2006-01-02"),
			e.BranchID,
			e.BranchName,
			e.CommitHashes,
			computeSHA256(e.Content),
		}, "|"))
	}
	parts = append(parts, computeSHA256(renderedMarkdown))
	sort.Strings(parts)
	return computeSHA256(strings.Join(parts, "\n"))
}

func computeSHA256(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func exportBasePath(exportCtx *obsidianExportContext) string {
	profileName := sanitizePathComponent(exportCtx.profileName)
	if profileName == "" {
		profileName = "default"
	}
	repoName := sanitizePathComponent(exportCtx.repoName)
	if repoName == "" {
		repoName = "unknown_repo"
	}
	return filepath.Join(exportCtx.rootFolder, profileName, repoName)
}

func sanitizePathComponent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, " ", "_")
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_")
	return replacer.Replace(s)
}

func exportStateID(codebaseID, profileName, entryType string, entryDate time.Time, branchID string) string {
	key := strings.Join([]string{
		codebaseID,
		profileName,
		entryType,
		entryDate.Format("2006-01-02"),
		branchID,
	}, "|")
	return "wxs-" + computeSHA256(key)
}

func weekRangeNoteID(weekStart time.Time) string {
	weekEnd := weekStart.AddDate(0, 0, 6)
	return fmt.Sprintf("%s_to_%s", weekStart.Format("2006-01-02"), weekEnd.Format("2006-01-02"))
}

func exportEntryIsCurrent(exportCtx *obsidianExportContext, item obsidianExportItem, state *db.WorklogExportState) bool {
	if state == nil {
		return false
	}
	if state.Signature != item.Signature {
		return false
	}
	if state.FilePath != item.RelativePath {
		return false
	}
	targetPath := filepath.Join(exportCtx.vaultPath, item.RelativePath)
	if _, err := os.Stat(targetPath); err != nil {
		return false
	}
	return true
}

func printTypeBreakdown(summary *obsidianExportSummary) {
	for _, t := range []string{"day_updates", "week_summary", "month_summary"} {
		scanned := summary.ByTypeScanned[t]
		if scanned == 0 {
			continue
		}
		fmt.Printf("- %s: scanned=%d exported=%d unchanged=%d\n",
			t, scanned, summary.ByTypeExported[t], summary.ByTypeSkipped[t])
	}
}

func printStatusTypeBreakdown(summary *obsidianExportSummary) {
	for _, t := range []string{"day_updates", "week_summary", "month_summary"} {
		scanned := summary.ByTypeScanned[t]
		if scanned == 0 {
			continue
		}
		fmt.Printf("- %s: total=%d up_to_date=%d pending=%d\n",
			t, scanned, summary.ByTypeSkipped[t], summary.ByTypePending[t])
	}
}
