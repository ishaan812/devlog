package cli

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/db"
	"github.com/ishaan812/devlog/internal/git"
	"github.com/ishaan812/devlog/internal/indexer"
	"github.com/ishaan812/devlog/internal/llm"
	"github.com/ishaan812/devlog/internal/prompts"
	"github.com/ishaan812/devlog/internal/tui"
)

var githubNoReplyRegex = regexp.MustCompile(`^(?:\d+\+)?([^@]+)@users\.noreply\.github\.com$`)

func extractGitHubUsername(email string) string {
	matches := githubNoReplyRegex.FindStringSubmatch(strings.ToLower(email))
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

func isUserCommitByGitHub(authorEmail string, githubUsername string) bool {
	if githubUsername == "" {
		return false
	}
	extractedUsername := extractGitHubUsername(authorEmail)
	return strings.EqualFold(extractedUsername, githubUsername)
}

func isMergeSyncCommit(commit *git.Commit, baseBranch string, isDefault bool, baseBranchHashes map[string]bool) bool {
	if isDefault || baseBranch == "" || commit.NumParents() < 2 || len(baseBranchHashes) == 0 {
		return false
	}
	hasBaseParent := false
	_ = commit.Parents().ForEach(func(parent *git.Commit) error {
		if baseBranchHashes[parent.Hash.String()] {
			hasBaseParent = true
		}
		return nil
	})
	if !hasBaseParent {
		return false
	}
	// A non-default branch merge commit that pulls in the base branch history is
	// treated as branch-sync work (possibly with conflict resolution), not feature work.
	return true
}

const (
	indexSoftLimit = 500
	indexHardLimit = 1000
)

const (
	summaryModeAuto     = "auto"
	summaryModeFull     = "full"
	summaryModeTargeted = "targeted"
	summaryModeOff      = "off"
)

var (
	ingestDays              int
	ingestAll               bool
	ingestSince             string
	ingestBranches          []string
	ingestAllBranches       bool
	ingestReselectBranch    bool
	ingestSkipSummaries     bool
	ingestSummaryMode       string
	ingestTargetedLookback  int
	ingestTargetedFolders   int
	ingestTargetedChildren  int
	ingestTargetedMinFiles  int
	ingestTargetedHighChurn int
	ingestMaxFiles          int
	ingestAllFiles          bool
	ingestGitOnly           bool
	ingestIndexOnly         bool
	ingestSkipCommitSums    bool
	ingestFillSummaries     bool
	ingestForceReindex      bool
	ingestSkipWorklog       bool
	ingestReselectFolders   bool
	ingestPreparedSelection *BranchSelection
)

var ingestCmd = &cobra.Command{
	Use:   "ingest [path]",
	Short: "Ingest git history and index codebase",
	Long: `Ingest git commit history and index codebase for semantic search.

This unified command performs two phases:
  1. Git History Ingestion - Scan commits and store in the database
  2. Codebase Indexing - Generate summaries for search

The repository is automatically added to the active profile.

Branch selections are saved per repo. On subsequent ingests, you'll be prompted:
  [Enter] Use current selection  [m] Modify  [r] Reselect all

The main/default branch is ingested fully, while feature branches only
ingest commits unique to that branch (not on the main branch).

Examples:
  devlog ingest                       # Prompts for branch action if saved, or full selection
  devlog ingest --reselect-branches   # Skip prompt, go straight to full branch selection
  devlog ingest ~/projects/myapp      # Ingest specific path
  devlog ingest --all-branches        # Ingest all branches without prompting
  devlog ingest --branches main,dev   # Ingest specific branches
  devlog ingest --days 90             # Last 90 days of git history
  devlog ingest --all                 # Full git history
  devlog ingest --git-only            # Only git history, skip indexing
  devlog ingest --index-only          # Only indexing, skip git history
  devlog ingest --summary-mode auto   # Auto summary mode (full/targeted/off)
  devlog ingest --all-files           # Index all files (bypass 500/1000 limits)
  devlog ingest --reselect-folders    # Re-prompt for which folders to index`,
	Args: cobra.MaximumNArgs(1),
	RunE: runIngest,
}

func init() {
	rootCmd.AddCommand(ingestCmd)

	ingestCmd.Flags().IntVar(&ingestDays, "days", 30, "Number of days of history to ingest")
	ingestCmd.Flags().BoolVar(&ingestAll, "all", false, "Ingest full git history (ignores --days)")
	ingestCmd.Flags().StringVar(&ingestSince, "since", "", "Ingest commits since date (YYYY-MM-DD)")
	ingestCmd.Flags().StringSliceVar(&ingestBranches, "branches", nil, "Specific branches to ingest (comma-separated)")
	ingestCmd.Flags().BoolVar(&ingestAllBranches, "all-branches", false, "Ingest all branches without prompting")
	ingestCmd.Flags().BoolVar(&ingestReselectBranch, "reselect-branches", false, "Re-select branches (ignore saved selection)")
	ingestCmd.Flags().BoolVar(&ingestSkipSummaries, "skip-summaries", false, "Skip LLM-generated summaries")
	ingestCmd.Flags().StringVar(&ingestSummaryMode, "summary-mode", summaryModeAuto, "Summary strategy: auto|full|targeted|off")
	ingestCmd.Flags().IntVar(&ingestTargetedLookback, "targeted-lookback-days", 120, "How many days of user commits to use for targeted path activity")
	ingestCmd.Flags().IntVar(&ingestTargetedFolders, "targeted-max-folders", 40, "Maximum active folders to summarize in targeted mode")
	ingestCmd.Flags().IntVar(&ingestTargetedChildren, "targeted-max-children", 12, "Maximum direct files/subfolders to include in folder summary context")
	ingestCmd.Flags().IntVar(&ingestTargetedMinFiles, "targeted-min-files", 2, "Minimum distinct touched files needed to directly mark folder active")
	ingestCmd.Flags().IntVar(&ingestTargetedHighChurn, "targeted-high-churn", 500, "Minimum folder churn required for incremental re-summarization in targeted mode")
	ingestCmd.Flags().IntVar(&ingestMaxFiles, "max-files", 0, "Maximum files to index (overrides limits, 0 = use defaults)")
	ingestCmd.Flags().BoolVar(&ingestAllFiles, "all-files", false, "Index all files (bypass soft/hard limits)")
	ingestCmd.Flags().BoolVar(&ingestGitOnly, "git-only", false, "Only ingest git history")
	ingestCmd.Flags().BoolVar(&ingestIndexOnly, "index-only", false, "Only index codebase")
	ingestCmd.Flags().BoolVar(&ingestSkipCommitSums, "skip-commit-summaries", false, "Skip LLM-generated commit summaries")
	ingestCmd.Flags().BoolVar(&ingestFillSummaries, "fill-summaries", false, "Generate summaries for existing commits that are missing them")
	ingestCmd.Flags().BoolVar(&ingestForceReindex, "force-reindex", false, "Force re-indexing all files, ignoring content hashes")
	ingestCmd.Flags().BoolVar(&ingestSkipWorklog, "skip-worklog", false, "Skip worklog generation prompt after ingestion")
	ingestCmd.Flags().BoolVar(&ingestReselectFolders, "reselect-folders", false, "Re-prompt for index folder selection")
}

// acquireIngestLock prevents concurrent ingest runs (which would conflict on DuckDB's exclusive lock).
// Returns a release function to call when done, or an error if another ingest is running.
func acquireIngestLock() (release func(), err error) {
	lockPath := filepath.Join(config.GetDevlogDir(), "ingest.lock")
	release = func() {
		_ = os.Remove(lockPath)
	}

	data, err := os.ReadFile(lockPath)
	if err == nil {
		if pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data))); parseErr == nil && processExists(pid) {
			return nil, fmt.Errorf("another 'devlog ingest' is already running (PID %d).\n"+
				"Kill it with: kill -9 %d\n"+
				"If it won't die (stuck), run: pkill -9 -f 'devlog ingest'\n"+
				"If still stuck, reboot your Mac to clear zombie processes",
				pid, pid)
		}
		// Stale lock (process no longer exists)
		_ = os.Remove(lockPath)
	}

	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return release, fmt.Errorf("create devlog dir: %w", err)
	}
	if err := os.WriteFile(lockPath, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		return release, fmt.Errorf("acquire ingest lock: %w", err)
	}
	return release, nil
}

func processExists(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func runIngest(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	release, err := acquireIngestLock()
	if err != nil {
		return err
	}
	defer release()

	titleColor := color.New(color.FgHiCyan, color.Bold)
	successColor := color.New(color.FgHiGreen)
	dimColor := color.New(color.FgHiBlack)
	promptColor := color.New(color.FgYellow)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.EnsureDefaultProfile(); err != nil {
		return fmt.Errorf("failed to ensure default profile: %w", err)
	}

	db.SetActiveProfile(cfg.GetActiveProfileName())

	profileName := cfg.GetActiveProfileName()
	if err := cfg.AddRepoToProfile(profileName, absPath); err != nil {
		VerboseLog("Warning: failed to add repo to profile: %v", err)
	} else {
		if err := cfg.Save(); err != nil {
			VerboseLog("Warning: failed to save config: %v", err)
		}
	}

	fmt.Println()
	titleColor.Printf("  Ingesting Repository\n")
	dimColor.Printf("  %s\n", absPath)
	dimColor.Printf("  Profile: %s\n\n", profileName)

	gitHistoryIngested := false
	canIngestGit := !ingestIndexOnly
	if canIngestGit {
		selection, err := prepareBranchSelection(absPath, cfg)
		if err != nil {
			VerboseLog("Git branch selection warning: %v", err)
			dimColor.Printf("  Note: Git branch selection skipped (%v)\n\n", err)
			canIngestGit = false
		} else {
			ingestPreparedSelection = selection
		}
	}

	if !ingestGitOnly {
		if err := indexCodebase(absPath, cfg); err != nil {
			return fmt.Errorf("indexing failed: %w", err)
		}
	}

	if canIngestGit {
		if err := ingestGitHistory(absPath, cfg); err != nil {
			VerboseLog("Git ingest warning: %v", err)
			dimColor.Printf("  Note: Git ingestion skipped (%v)\n\n", err)
		} else {
			gitHistoryIngested = true
		}
	}

	fmt.Println()
	successColor.Printf("  Ingestion Complete!\n\n")

	// Prompt for worklog generation if git history was ingested
	if gitHistoryIngested && !ingestSkipWorklog {
		promptColor.Printf("  Generate worklog from ingested commits? [Y/n]: ")
		var input string
		fmt.Scanln(&input)
		input = strings.ToLower(strings.TrimSpace(input))

		if input == "" || input == "y" || input == "yes" {
			fmt.Println()
			if err := generateWorklogAfterIngest(absPath, cfg); err != nil {
				dimColor.Printf("  Warning: Failed to generate worklog: %v\n", err)
				dimColor.Println("  You can manually generate it with 'devlog worklog'")
			}
		} else {
			dimColor.Println("  Skipped worklog generation")
			dimColor.Println("  Use 'devlog worklog' to generate it later")
		}
	} else if gitHistoryIngested {
		dimColor.Println("  Use 'devlog worklog' to view your development activity")
	}
	fmt.Println()

	return nil
}

type BranchSelection struct {
	MainBranch       string
	SelectedBranches []string
}

type branchSelectionMode string

const (
	branchSelectionModeAutomatic branchSelectionMode = "automatic"
	branchSelectionModeManual    branchSelectionMode = "manual"
)

func generateWorklogAfterIngest(absPath string, cfg *config.Config) error {
	ctx := context.Background()
	titleColor := color.New(color.FgHiCyan, color.Bold)
	successColor := color.New(color.FgHiGreen)
	dimColor := color.New(color.FgHiBlack)

	titleColor.Printf("  Generating Worklog\n")

	dbRepo, err := db.GetRepository()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	codebase, err := dbRepo.GetCodebaseByPath(ctx, absPath)
	if err != nil || codebase == nil {
		return fmt.Errorf("codebase not found at %s", absPath)
	}

	loc := time.UTC
	if tzName := cfg.GetTimezone(); tzName != "" {
		if tzLoc, err := time.LoadLocation(tzName); err == nil {
			loc = tzLoc
		}
	}

	// Determine date range based on ingest flags
	endDate := time.Now().In(loc)
	var startDate time.Time
	var days int

	if ingestAll {
		// For full history, get the earliest commit date
		earliestCommit, err := dbRepo.GetEarliestCommitDate(ctx, codebase.ID)
		if err == nil && !earliestCommit.IsZero() {
			startDate = earliestCommit.In(loc)
			days = int(endDate.Sub(startDate).Hours() / 24)
		} else {
			startDate = endDate.AddDate(0, 0, -30)
			days = 30
		}
	} else if ingestSince != "" {
		sinceDate, err := time.Parse("2006-01-02", ingestSince)
		if err != nil {
			return fmt.Errorf("invalid date format: %w", err)
		}
		startDate = sinceDate.In(loc)
		days = int(endDate.Sub(startDate).Hours() / 24)
	} else {
		days = ingestDays
		startDate = endDate.AddDate(0, 0, -days)
	}

	dimColor.Printf("  Period: %s to %s (%d days)\n", startDate.Format("Jan 2"), endDate.Format("Jan 2, 2006"), days)

	commits, err := queryCommitsForWorklog(ctx, dbRepo, codebase, startDate, endDate, cfg)
	if err != nil {
		return fmt.Errorf("failed to query commits: %w", err)
	}

	if len(commits) == 0 {
		dimColor.Println("  No commits found in the ingested range")
		return nil
	}

	dimColor.Printf("  Found %d commits\n\n", len(commits))

	// Create LLM client if needed
	var client llm.Client
	if !ingestSkipSummaries && !ingestSkipCommitSums {
		client, err = createLLMClient(cfg)
		if err != nil {
			dimColor.Printf("  Skipping LLM summaries: %v\n", err)
		}
	}

	projectContext := getProjectContext(codebase)
	codebaseContext := getCodebaseContext(codebase)
	nameOfUser := getWorklogUserName(cfg)

	style := cfg.GetWorklogStyle()
	if style == "" {
		style = "non-technical"
	}

	cache := &worklogCacheContext{
		dbRepo:                dbRepo,
		codebaseID:            codebase.ID,
		profileName:           cfg.GetActiveProfileName(),
		loc:                   loc,
		noCache:               false,
		ChangedDailySummaries: make(map[time.Time]bool),
	}

	groups := groupByDate(commits, loc)
	markdown, err := generateWorklogMarkdown(groups, client, cfg, loc, projectContext, codebaseContext, cache, style, nameOfUser)
	if err != nil {
		return fmt.Errorf("failed to generate markdown: %w", err)
	}

	// Keep ingest-generated worklogs in parity with `devlog worklog` by
	// persisting weekly/monthly summary cache entries when the date range spans
	// enough time and LLM generation is enabled.
	if client != nil {
		if days > 7 {
			dimColor.Println("  Generating weekly summaries...")
			if err := generateWeeklySummaries(ctx, cache, groups, client, projectContext, codebaseContext, loc, style, nameOfUser); err != nil {
				dimColor.Printf("  Warning: weekly summary generation failed: %v\n", err)
				VerboseLog("Warning: failed to generate weekly summaries after ingest: %v", err)
			}
		}
		if days > 28 {
			dimColor.Println("  Generating monthly summaries...")
			if err := generateMonthlySummaries(ctx, cache, client, projectContext, codebaseContext, loc, style, startDate, endDate, nameOfUser); err != nil {
				dimColor.Printf("  Warning: monthly summary generation failed: %v\n", err)
				VerboseLog("Warning: failed to generate monthly summaries after ingest: %v", err)
			}
		}
	}

	outputPath := fmt.Sprintf("worklog_%s_%s.md", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	if err := os.WriteFile(outputPath, []byte(markdown), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Println()
	successColor.Printf("  Worklog generated: %s\n", outputPath)

	return nil
}

func ingestGitHistory(absPath string, cfg *config.Config) error {
	ctx := context.Background()
	titleColor := color.New(color.FgHiCyan, color.Bold)
	successColor := color.New(color.FgHiGreen)
	dimColor := color.New(color.FgHiBlack)
	infoColor := color.New(color.FgHiWhite)
	titleColor.Printf("  Git History\n")

	VerboseLog("Opening repository at %s", absPath)
	repo, err := git.OpenRepo(absPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	VerboseLog("Initializing database")
	dbRepo, err := db.GetRepository()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	codebase, err := dbRepo.GetCodebaseByPath(ctx, absPath)
	if err != nil {
		return fmt.Errorf("failed to get codebase: %w", err)
	}

	if codebase == nil {
		defaultBranch, err := repo.GetDefaultBranch()
		if err != nil {
			return fmt.Errorf("failed to detect default branch: %w", err)
		}
		codebase = &db.Codebase{
			ID:            uuid.New().String(),
			Path:          absPath,
			Name:          filepath.Base(absPath),
			DefaultBranch: defaultBranch,
			IndexedAt:     time.Now(),
		}
		if err := dbRepo.UpsertCodebase(ctx, codebase); err != nil {
			return fmt.Errorf("failed to create codebase: %w", err)
		}
	}

	userEmail := cfg.GetEffectiveUserEmail()
	if userEmail == "" {
		var gitErr error
		userEmail, gitErr = repo.GetUserEmail()
		if gitErr != nil {
			VerboseLog("Warning: failed to get git user email: %v", gitErr)
		}
	}
	githubUsername := cfg.GetEffectiveGitHubUsername()

	if userEmail != "" {
		userName := cfg.GetEffectiveUserName()
		if userName == "" {
			var nameErr error
			userName, nameErr = repo.GetUserName()
			if nameErr != nil {
				VerboseLog("Warning: failed to get git user name: %v", nameErr)
			}
		}
		dev := &db.Developer{
			ID:            userEmail,
			Name:          userName,
			Email:         userEmail,
			IsCurrentUser: true,
		}
		if err := dbRepo.UpsertDeveloper(ctx, dev); err != nil {
			return fmt.Errorf("failed to upsert developer: %w", err)
		}
		dbRepo.SetCurrentUser(ctx, userEmail)
	}
	allBranches, err := repo.ListBranches()
	if err != nil {
		return fmt.Errorf("failed to list branches: %w", err)
	}

	if len(allBranches) == 0 {
		dimColor.Println("  No branches found in repository")
		return nil
	}

	selection := ingestPreparedSelection
	ingestPreparedSelection = nil
	if selection == nil {
		selection, err = selectBranches(allBranches, codebase.DefaultBranch, cfg, absPath, repo, userEmail, githubUsername)
		if err != nil {
			return fmt.Errorf("branch selection failed: %w", err)
		}
	}

	if selection.MainBranch != codebase.DefaultBranch {
		codebase.DefaultBranch = selection.MainBranch
		dbRepo.UpsertCodebase(ctx, codebase)
	}

	sinceDate, err := resolveIngestSinceDate()
	if err != nil {
		return fmt.Errorf("invalid date format (use YYYY-MM-DD): %w", err)
	}
	if ingestAll {
		dimColor.Println("  Ingesting full history...")
	} else if ingestSince != "" {
		// Load timezone for display
		loc := time.UTC
		if tzName := cfg.GetTimezone(); tzName != "" {
			if tzLoc, err := time.LoadLocation(tzName); err == nil {
				loc = tzLoc
			}
		}
		dimColor.Printf("  Since %s...\n", sinceDate.In(loc).Format("Jan 2, 2006"))
	} else {
		dimColor.Printf("  Last %d days...\n", ingestDays)
	}

	var totalCommits, totalFiles int
	var llmClient llm.Client
	if !ingestSkipCommitSums && !ingestSkipSummaries {
		if llmClient, err = createLLMClient(cfg); err != nil {
			return fmt.Errorf("failed to initialize LLM client: %w\n\nTo skip summaries, use: --skip-summaries or --skip-commit-summaries", err)
		}
	}

	existingHashes, err := dbRepo.GetExistingCommitHashes(ctx, codebase.ID)
	if err != nil {
		return fmt.Errorf("failed to get existing commit hashes: %w", err)
	}
	VerboseLog("Found %d existing commits in database", len(existingHashes))

	selectedMap := make(map[string]bool)
	for _, b := range selection.SelectedBranches {
		selectedMap[b] = true
	}

	fmt.Println()
	dimColor.Printf("  Scanning branches...\n")

	for _, branchInfo := range allBranches {
		if branchInfo.Name != selection.MainBranch {
			continue
		}
		dimColor.Printf("    Processing %s (main)...\n", branchInfo.Name)
		branchInfo.IsDefault = true
		commits, files, err := ingestBranch(ctx, dbRepo, repo, codebase, branchInfo, "", sinceDate, userEmail, githubUsername, llmClient, existingHashes)
		if err != nil {
			warnColor := color.New(color.FgHiYellow)
			warnColor.Printf("    Skipping %s: %v\n", branchInfo.Name, err)
			continue
		}
		totalCommits += commits
		totalFiles += files
		if commits > 0 {
			infoColor.Printf("    %s (main): %d commits\n", branchInfo.Name, commits)
		} else {
			dimColor.Printf("    %s (main): no new commits\n", branchInfo.Name)
		}
	}

	for _, branchInfo := range allBranches {
		if branchInfo.Name == selection.MainBranch || !selectedMap[branchInfo.Name] {
			continue
		}
		dimColor.Printf("    Processing %s...\n", branchInfo.Name)
		commits, files, err := ingestBranch(ctx, dbRepo, repo, codebase, branchInfo, selection.MainBranch, sinceDate, userEmail, githubUsername, llmClient, existingHashes)
		if err != nil {
			warnColor := color.New(color.FgHiYellow)
			warnColor.Printf("    Skipping %s: %v\n", branchInfo.Name, err)
			continue
		}
		totalCommits += commits
		totalFiles += files
		if commits > 0 {
			infoColor.Printf("    %s: %d commits\n", branchInfo.Name, commits)
		}
	}

	if ingestFillSummaries && llmClient != nil {
		fillCount, err := fillMissingSummaries(ctx, dbRepo, repo, codebase, llmClient)
		if err != nil {
			return fmt.Errorf("failed to fill missing summaries: %w", err)
		} else if fillCount > 0 {
			successColor.Printf("  Generated %d missing commit summaries\n", fillCount)
		}
	}

	fmt.Println()
	if totalCommits == 0 {
		dimColor.Println("  No new commits in time range")
	} else {
		successColor.Printf("  Ingested %d commits, %d file changes\n", totalCommits, totalFiles)
	}

	commitCount, err := dbRepo.GetCommitCount(ctx, codebase.ID)
	if err != nil {
		return fmt.Errorf("failed to get commit count: %w", err)
	}
	fileCount, err := dbRepo.GetFileChangeCount(ctx, codebase.ID)
	if err != nil {
		return fmt.Errorf("failed to get file change count: %w", err)
	}
	infoColor.Printf("  Total: %d commits, %d file changes\n\n", commitCount, fileCount)

	return nil
}

func prepareBranchSelection(absPath string, cfg *config.Config) (*BranchSelection, error) {
	ctx := context.Background()
	repo, err := git.OpenRepo(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}
	dbRepo, err := db.GetRepository()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	codebase, err := dbRepo.GetCodebaseByPath(ctx, absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get codebase: %w", err)
	}
	if codebase == nil {
		defaultBranch, err := repo.GetDefaultBranch()
		if err != nil {
			return nil, fmt.Errorf("failed to detect default branch: %w", err)
		}
		codebase = &db.Codebase{
			ID:            uuid.New().String(),
			Path:          absPath,
			Name:          filepath.Base(absPath),
			DefaultBranch: defaultBranch,
			IndexedAt:     time.Now(),
		}
		if err := dbRepo.UpsertCodebase(ctx, codebase); err != nil {
			return nil, fmt.Errorf("failed to create codebase: %w", err)
		}
	}

	userEmail := cfg.GetEffectiveUserEmail()
	if userEmail == "" {
		if detectedEmail, gitErr := repo.GetUserEmail(); gitErr == nil {
			userEmail = detectedEmail
		}
	}
	githubUsername := cfg.GetEffectiveGitHubUsername()

	allBranches, err := repo.ListBranches()
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}
	if len(allBranches) == 0 {
		return nil, fmt.Errorf("no branches found in repository")
	}

	selection, err := selectBranches(allBranches, codebase.DefaultBranch, cfg, absPath, repo, userEmail, githubUsername)
	if err != nil {
		return nil, err
	}
	if selection.MainBranch != codebase.DefaultBranch {
		codebase.DefaultBranch = selection.MainBranch
		_ = dbRepo.UpsertCodebase(ctx, codebase)
	}
	return selection, nil
}

func selectBranches(branches []git.BranchInfo, detectedDefault string, cfg *config.Config, repoPath string, repo *git.Repository, userEmail, githubUsername string) (*BranchSelection, error) {
	dimColor := color.New(color.FgHiBlack)
	infoColor := color.New(color.FgCyan)
	promptColor := color.New(color.FgYellow)

	if len(ingestBranches) > 0 {
		mainBranch := ingestBranches[0]
		return &BranchSelection{
			MainBranch:       mainBranch,
			SelectedBranches: ingestBranches,
		}, nil
	}

	if ingestAllBranches {
		var branchNames []string
		mainBranch := detectedDefault
		for _, b := range branches {
			branchNames = append(branchNames, b.Name)
			if b.IsDefault {
				mainBranch = b.Name
			}
		}
		return &BranchSelection{
			MainBranch:       mainBranch,
			SelectedBranches: branchNames,
		}, nil
	}

	profileName := cfg.GetActiveProfileName()
	saved := cfg.GetBranchSelection(profileName, repoPath)

	if saved != nil && len(saved.SelectedBranches) > 0 && !ingestReselectBranch {
		branchMap := make(map[string]bool)
		for _, b := range branches {
			branchMap[b.Name] = true
		}

		validBranches := []string{}
		for _, b := range saved.SelectedBranches {
			if branchMap[b] {
				validBranches = append(validBranches, b)
			}
		}

		if branchMap[saved.MainBranch] && len(validBranches) > 0 {
			fmt.Println()
			infoColor.Printf("  Saved branch selection:\n")
			dimColor.Printf("    Main: %s\n", saved.MainBranch)
			dimColor.Printf("    Branches: %s\n", strings.Join(validBranches, ", "))
			fmt.Println()

			promptColor.Printf("  [Enter] Use current selection  [a] Auto  [m] Manual modify  [r] Reselect manual: ")

			var input string
			fmt.Scanln(&input)
			input = strings.ToLower(strings.TrimSpace(input))

			switch input {
			case "", "y", "yes":
				fmt.Println()
				return &BranchSelection{
					MainBranch:       saved.MainBranch,
					SelectedBranches: validBranches,
				}, nil

			case "a", "auto", "automatic":
				fmt.Println()
				autoSelection, err := runAutomaticBranchSelection(branches, detectedDefault, repo, userEmail, githubUsername, infoColor, dimColor)
				if err != nil {
					return nil, err
				}
				return saveBranchSelectionRaw(cfg, profileName, repoPath, autoSelection.MainBranch, autoSelection.SelectedBranches, dimColor), nil

			case "m", "modify":
				fmt.Println()
				selection, err := tui.RunBranchSelectionWithPreselected(branches, saved.MainBranch, validBranches)
				if err != nil {
					return nil, err
				}
				return saveBranchSelection(cfg, profileName, repoPath, selection, dimColor)

			case "r", "reselect":
			default:
				fmt.Println()
			}
		}
	}

	mode := askBranchSelectionMode(promptColor)
	if mode == branchSelectionModeAutomatic {
		autoSelection, err := runAutomaticBranchSelection(branches, detectedDefault, repo, userEmail, githubUsername, infoColor, dimColor)
		if err != nil {
			return nil, err
		}
		return saveBranchSelectionRaw(cfg, profileName, repoPath, autoSelection.MainBranch, autoSelection.SelectedBranches, dimColor), nil
	}

	fmt.Println()
	selection, err := tui.RunBranchSelection(branches, detectedDefault)
	if err != nil {
		return nil, err
	}

	return saveBranchSelection(cfg, profileName, repoPath, selection, dimColor)
}

func askBranchSelectionMode(promptColor *color.Color) branchSelectionMode {
	fmt.Println()
	promptColor.Printf("  Branch selection mode: [a] Automatic (commits by you)  [m] Manual selection [default: a]: ")

	var input string
	fmt.Scanln(&input)
	input = strings.ToLower(strings.TrimSpace(input))

	switch input {
	case "m", "manual":
		return branchSelectionModeManual
	default:
		return branchSelectionModeAutomatic
	}
}

func runAutomaticBranchSelection(branches []git.BranchInfo, detectedDefault string, repo *git.Repository, userEmail, githubUsername string, infoColor, dimColor *color.Color) (*BranchSelection, error) {
	mainBranch := detectedDefault
	for _, b := range branches {
		if b.IsDefault {
			mainBranch = b.Name
			break
		}
	}

	if strings.TrimSpace(userEmail) == "" && strings.TrimSpace(githubUsername) == "" {
		dimColor.Println("  Automatic branch selection requires user email or GitHub username; falling back to main branch only.")
		return &BranchSelection{
			MainBranch:       mainBranch,
			SelectedBranches: []string{mainBranch},
		}, nil
	}

	autoBranches, err := repo.FindBranchesWithUserCommits(userEmail, githubUsername)
	if err != nil {
		return nil, fmt.Errorf("failed to detect branches with user commits: %w", err)
	}

	selectedSet := make(map[string]bool, len(autoBranches)+1)
	for _, name := range autoBranches {
		selectedSet[name] = true
	}
	selectedSet[mainBranch] = true

	selectedBranches := make([]string, 0, len(branches))
	for _, b := range branches {
		if selectedSet[b.Name] {
			selectedBranches = append(selectedBranches, b.Name)
		}
	}

	if len(selectedBranches) == 0 {
		selectedBranches = []string{mainBranch}
	}

	infoColor.Printf("  Automatic branch selection found %d branch(es) with commits by you.\n", len(autoBranches))
	dimColor.Printf("  Selected: %s\n", strings.Join(selectedBranches, ", "))

	return &BranchSelection{
		MainBranch:       mainBranch,
		SelectedBranches: selectedBranches,
	}, nil
}

func saveBranchSelectionRaw(cfg *config.Config, profileName, repoPath, mainBranch string, selectedBranches []string, dimColor *color.Color) *BranchSelection {
	fmt.Println()
	dimColor.Printf("  Selected %d branch(es): %s\n", len(selectedBranches), strings.Join(selectedBranches, ", "))

	if err := cfg.SaveBranchSelection(profileName, repoPath, mainBranch, selectedBranches); err != nil {
		VerboseLog("Warning: failed to save branch selection: %v", err)
	} else {
		if err := cfg.Save(); err != nil {
			VerboseLog("Warning: failed to save config: %v", err)
		} else {
			dimColor.Printf("  (branch selection saved)\n")
		}
	}
	fmt.Println()

	return &BranchSelection{
		MainBranch:       mainBranch,
		SelectedBranches: selectedBranches,
	}
}

func saveBranchSelection(cfg *config.Config, profileName, repoPath string, selection *tui.BranchSelection, dimColor *color.Color) (*BranchSelection, error) {
	fmt.Println()
	dimColor.Printf("  Selected %d branch(es): %s\n", len(selection.SelectedBranches), strings.Join(selection.SelectedBranches, ", "))

	if err := cfg.SaveBranchSelection(profileName, repoPath, selection.MainBranch, selection.SelectedBranches); err != nil {
		VerboseLog("Warning: failed to save branch selection: %v", err)
	} else {
		if err := cfg.Save(); err != nil {
			VerboseLog("Warning: failed to save config: %v", err)
		} else {
			dimColor.Printf("  (branch selection saved)\n")
		}
	}
	fmt.Println()

	return &BranchSelection{
		MainBranch:       selection.MainBranch,
		SelectedBranches: selection.SelectedBranches,
	}, nil
}

func ingestBranch(ctx context.Context, dbRepo *db.SQLRepository, repo *git.Repository, codebase *db.Codebase, branchInfo git.BranchInfo, baseBranch string, sinceDate time.Time, userEmail string, githubUsername string, llmClient llm.Client, existingHashes map[string]bool) (int, int, error) {
	branch, err := dbRepo.GetBranch(ctx, codebase.ID, branchInfo.Name)
	if err != nil {
		return 0, 0, err
	}

	isDefault := branchInfo.Name == codebase.DefaultBranch || branchInfo.IsDefault

	if branch == nil {
		branch = &db.Branch{
			ID:         uuid.New().String(),
			CodebaseID: codebase.ID,
			Name:       branchInfo.Name,
			IsDefault:  isDefault,
			BaseBranch: baseBranch,
			Status:     "active",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if err := dbRepo.UpsertBranch(ctx, branch); err != nil {
			VerboseLog("Warning: failed to create branch %s: %v", branchInfo.Name, err)
		}
	}

	lastHash, err := dbRepo.GetBranchCursor(ctx, codebase.ID, branchInfo.Name)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get branch cursor for %s: %w", branchInfo.Name, err)
	}

	var commitHashes []string
	if isDefault || baseBranch == "" {
		commitHashes, err = repo.GetCommitsOnBranchSince(branchInfo.Name, "", sinceDate)
	} else {
		commitHashes, err = repo.GetCommitsOnBranchSince(branchInfo.Name, baseBranch, sinceDate)
	}
	if err != nil {
		VerboseLog("Error getting commits for branch %s: %v", branchInfo.Name, err)
		return 0, 0, err
	}

	var newCommitHashes []string
	for _, hash := range commitHashes {
		if hash == lastHash {
			VerboseLog("Stopping at cursor hash: %s", lastHash)
			break
		}
		if existingHashes[hash] {
			continue
		}
		newCommitHashes = append(newCommitHashes, hash)
	}

	VerboseLog("Branch %s: %d total commits, %d new to process", branchInfo.Name, len(commitHashes), len(newCommitHashes))

	var commitCount, fileCount int
	var firstHash, latestHash string
	baseBranchHashes := map[string]bool{}
	if !isDefault && baseBranch != "" {
		if hashes, hashErr := repo.GetCommitHashSet(baseBranch); hashErr == nil {
			baseBranchHashes = hashes
		} else {
			VerboseLog("Warning: failed to load base branch hash set for %s: %v", baseBranch, hashErr)
		}
	}

	for _, hash := range newCommitHashes {
		gitCommit, err := repo.GetCommit(hash)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to get commit %s: %w", hash, err)
		}

		if !sinceDate.IsZero() && gitCommit.Author.When.Before(sinceDate) {
			VerboseLog("Skipping commit %s: before date filter", hash[:8])
			continue
		}

		if latestHash == "" {
			latestHash = hash
		}
		firstHash = hash

		author := gitCommit.Author
		dev := &db.Developer{ID: author.Email, Name: author.Name, Email: author.Email}
		if err := dbRepo.UpsertDeveloper(ctx, dev); err != nil {
			return 0, 0, fmt.Errorf("failed to upsert developer %s: %w", author.Email, err)
		}

		isUserCommit := (userEmail != "" && strings.EqualFold(author.Email, userEmail)) || isUserCommitByGitHub(author.Email, githubUsername)
		parentCount := gitCommit.NumParents()
		isMergeSync := isMergeSyncCommit(gitCommit, baseBranch, isDefault, baseBranchHashes)

		stats, fileChanges, err := getCommitStats(repo, gitCommit)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to get stats for commit %s: %w", hash[:8], err)
		}

		var commitSummary string
		if isUserCommit && !isMergeSync && llmClient != nil && len(fileChanges) > 0 {
			projectCtx := ""
			if codebase != nil {
				projectCtx = codebase.Summary
			}
			summary, err := generateCommitSummary(llmClient, gitCommit.Message, fileChanges, projectCtx)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to generate commit summary for %s: %w", hash[:8], err)
			}
			commitSummary = summary
		}

		commit := &db.Commit{
			ID:                uuid.New().String(),
			Hash:              hash,
			CodebaseID:        codebase.ID,
			BranchID:          branch.ID,
			AuthorEmail:       author.Email,
			Message:           strings.TrimSpace(gitCommit.Message),
			Summary:           commitSummary,
			CommittedAt:       author.When,
			Stats:             stats,
			IsUserCommit:      isUserCommit,
			IsOnDefaultBranch: isDefault,
			ParentCount:       parentCount,
			IsMergeSync:       isMergeSync,
		}

		if err := dbRepo.UpsertCommit(ctx, commit); err != nil {
			VerboseLog("Warning: failed to insert commit %s: %v", hash[:8], err)
			continue
		}

		existingHashes[hash] = true

		for _, fc := range fileChanges {
			fc.CommitID = commit.ID
			if err := dbRepo.CreateFileChange(ctx, fc); err != nil {
				VerboseLog("Warning: failed to insert file change: %v", err)
			}
			fileCount++
		}
		if isUserCommit && len(fileChanges) > 0 {
			updateCodebaseTouchActivity(codebase, author.When, fileChanges)
		}

		commitCount++
	}

	if commitCount > 0 || branch.ID != "" {
		branch.CommitCount = commitCount
		branch.IsDefault = isDefault
		if firstHash != "" {
			branch.FirstCommitHash = firstHash
		}
		if latestHash != "" {
			branch.LastCommitHash = latestHash
		}
		branch.UpdatedAt = time.Now()
		if err := dbRepo.UpsertBranch(ctx, branch); err != nil {
			return 0, 0, fmt.Errorf("failed to upsert branch %s: %w", branchInfo.Name, err)
		}

		if latestHash != "" {
			if err := dbRepo.UpdateBranchCursor(ctx, codebase.ID, branchInfo.Name, latestHash); err != nil {
				return 0, 0, fmt.Errorf("failed to update branch cursor for %s: %w", branchInfo.Name, err)
			}
		}
		if commitCount > 0 {
			if err := dbRepo.UpsertCodebase(ctx, codebase); err != nil {
				VerboseLog("Warning: failed to persist incremental codebase touch activity: %v", err)
			}
		}
	}

	return commitCount, fileCount, nil
}

func updateCodebaseTouchActivity(codebase *db.Codebase, committedAt time.Time, fileChanges []*db.FileChange) {
	if codebase == nil {
		return
	}
	if codebase.TouchActivity == nil {
		codebase.TouchActivity = map[string]any{}
	}

	type folderDelta struct {
		touches int
		churn   int
	}
	deltas := make(map[string]folderDelta)
	for _, fc := range fileChanges {
		if fc == nil || fc.FilePath == "" {
			continue
		}
		folderPath := normalizeFolderPath(filepath.Dir(fc.FilePath))
		delta := deltas[folderPath]
		delta.touches++
		delta.churn += fc.Additions + fc.Deletions
		deltas[folderPath] = delta
	}

	for folderPath, delta := range deltas {
		raw := parseTouchEntry(codebase.TouchActivity[folderPath])
		raw["touch_count"] = intMax(toInt(raw["touch_count"])+delta.touches, 0)
		raw["churn"] = intMax(toInt(raw["churn"])+delta.churn, 0)

		lastTouched := committedAt
		if existingTime, ok := parseTimeAny(raw["last_touched_at"]); ok && existingTime.After(lastTouched) {
			lastTouched = existingTime
		}
		raw["last_touched_at"] = lastTouched.Format(time.RFC3339)
		raw["updated_at"] = time.Now().Format(time.RFC3339)
		codebase.TouchActivity[folderPath] = raw
	}
}

func getCommitStats(repo *git.Repository, commit *git.Commit) (db.JSON, []*db.FileChange, error) {
	stats := db.JSON{
		"additions":     0,
		"deletions":     0,
		"files_changed": 0,
	}

	var fileChanges []*db.FileChange

	// Get parent commit for diff
	parentIter := commit.Parents()
	parent, err := parentIter.Next()
	if err != nil {
		// Initial commit has no parent — that's fine, return empty stats
		return stats, fileChanges, nil
	}

	parentTree, err := parent.Tree()
	if err != nil {
		return stats, fileChanges, fmt.Errorf("failed to get parent tree: %w", err)
	}

	commitTree, err := commit.Tree()
	if err != nil {
		return stats, fileChanges, fmt.Errorf("failed to get commit tree: %w", err)
	}

	changes, err := parentTree.Diff(commitTree)
	if err != nil {
		return stats, fileChanges, fmt.Errorf("failed to diff trees: %w", err)
	}

	var totalAdditions, totalDeletions int

	for _, change := range changes {
		fc := &db.FileChange{
			ID: uuid.New().String(),
		}

		action, err := change.Action()
		if err != nil {
			return stats, fileChanges, fmt.Errorf("failed to get change action: %w", err)
		}

		switch action.String() {
		case "Insert":
			fc.ChangeType = "add"
			fc.FilePath = change.To.Name
		case "Delete":
			fc.ChangeType = "delete"
			fc.FilePath = change.From.Name
		case "Modify":
			fc.ChangeType = "modify"
			fc.FilePath = change.To.Name
		default:
			continue
		}

		patch, err := change.Patch()
		if err == nil {
			patchStr := patch.String()
			for _, line := range strings.Split(patchStr, "\n") {
				if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
					fc.Additions++
					totalAdditions++
				} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
					fc.Deletions++
					totalDeletions++
				}
			}
			if len(patchStr) < 10000 {
				fc.Patch = patchStr
			}
		}

		fileChanges = append(fileChanges, fc)
	}

	stats["additions"] = totalAdditions
	stats["deletions"] = totalDeletions
	stats["files_changed"] = len(fileChanges)

	return stats, fileChanges, nil
}

func indexCodebase(absPath string, cfg *config.Config) error {
	ctx := context.Background()
	titleColor := color.New(color.FgHiCyan, color.Bold)
	successColor := color.New(color.FgHiGreen)
	infoColor := color.New(color.FgHiWhite)
	dimColor := color.New(color.FgHiBlack)
	warnColor := color.New(color.FgHiYellow)
	promptColor := color.New(color.FgHiYellow)

	titleColor.Printf("  Codebase Indexing\n")

	dbRepo, err := db.GetRepository()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	profileName := cfg.GetActiveProfileName()
	savedFolders := cfg.GetIndexFolders(profileName, absPath)
	if ingestReselectFolders {
		savedFolders = nil // Force fresh folder selection
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Scanning files..."
	s.Color("cyan")
	s.Start()

	scanResult, err := indexer.ScanCodebase(absPath, 500*1024, savedFolders)
	if err != nil {
		s.Stop()
		return fmt.Errorf("failed to scan codebase: %w", err)
	}
	s.Stop()

	countFolderStats := func(sr *indexer.ScanResult) (totalFolders int, internalFolders int) {
		for folderPath := range sr.Folders {
			if folderPath == "." {
				continue
			}
			totalFolders++
			if folderPath == "internal" || strings.HasPrefix(folderPath, "internal/") {
				internalFolders++
			}
		}
		return totalFolders, internalFolders
	}
	totalFolders, internalFolders := countFolderStats(scanResult)
	dimColor.Printf("  Scan stats: %d files, %d folders total, %d internal folders\n", len(scanResult.Files), totalFolders, internalFolders)

	// Soft limit: if > 500 files, no saved config (or --reselect-folders), and not --all-files, prompt for folder selection
	needsFolderPrompt := len(scanResult.Files) > indexSoftLimit && !ingestAllFiles && ingestMaxFiles == 0 && len(savedFolders) == 0
	if needsFolderPrompt {
		allFolders := indexer.AllFoldersWithCounts(scanResult)
		if len(allFolders) > 0 {
			promptColor.Printf("  Found %d files (limit %d). Select folders from full tree to index:\n", len(scanResult.Files), indexSoftLimit)
			tuiFolders := make([]tui.FolderInfo, 0, len(allFolders))
			for _, f := range allFolders {
				tuiFolders = append(tuiFolders, tui.FolderInfo{Path: f.Path, FileCount: f.FileCount})
			}
			selection, err := tui.RunFolderSelection(tuiFolders)
			if err != nil {
				return err
			}
			if len(selection.SelectedFolders) == 0 {
				return fmt.Errorf("no folders selected")
			}
			if err := cfg.SaveIndexFolders(profileName, absPath, selection.SelectedFolders); err != nil {
				return fmt.Errorf("failed to save folder selection: %w", err)
			}
			dimColor.Printf("  Saved folder selection. Re-scanning...\n")
			s.Start()
			scanResult, err = indexer.ScanCodebase(absPath, 500*1024, selection.SelectedFolders)
			s.Stop()
			if err != nil {
				return fmt.Errorf("failed to re-scan codebase: %w", err)
			}
			totalFolders, internalFolders = countFolderStats(scanResult)
			dimColor.Printf("  Re-scan stats: %d files, %d folders total, %d internal folders\n", len(scanResult.Files), totalFolders, internalFolders)
		}
	}

	// Hard limit: cap at 1000 unless --all-files or --max-files
	if ingestMaxFiles > 0 {
		if len(scanResult.Files) > ingestMaxFiles {
			scanResult.Files = scanResult.Files[:ingestMaxFiles]
			warnColor.Printf("  Limited to %d files (--max-files)\n", ingestMaxFiles)
		}
	} else if !ingestAllFiles && len(scanResult.Files) > indexHardLimit {
		scanResult.Files = scanResult.Files[:indexHardLimit]
		warnColor.Printf("  Limited to %d files (use --all-files to index all)\n", indexHardLimit)
	}

	successColor.Printf("  Found %d files in %d folders (%d internal folders)\n", len(scanResult.Files), totalFolders, internalFolders)

	summaryMode, modeReason := resolveSummaryMode(len(scanResult.Files))
	dimColor.Printf("  Summary mode: %s (%s)\n", summaryMode, modeReason)
	enableSummaries := summaryMode != summaryModeOff

	techStack := indexer.DetectTechStack(scanResult.Files)
	if len(techStack) > 0 {
		var techs []string
		for tech := range techStack {
			techs = append(techs, tech)
		}
		dimColor.Printf("  Tech: %s\n", joinMax(techs, 5))
	}

	codebase, err := dbRepo.GetCodebaseByPath(ctx, absPath)
	if err != nil {
		return fmt.Errorf("failed to get codebase by path: %w", err)
	}
	isFirstIndex := codebase == nil
	if codebase == nil {
		codebase = &db.Codebase{
			ID:   uuid.New().String(),
			Path: absPath,
			Name: scanResult.Name,
		}
	}
	codebase.IndexedAt = time.Now()
	codebase.TechStack = techStack

	var llmClient llm.Client
	var summarizer *indexer.Summarizer

	if enableSummaries {
		if llmClient, err = createLLMClient(cfg); err != nil {
			return fmt.Errorf("failed to initialize LLM client: %w\n\nTo skip file/folder summaries, use: --summary-mode off", err)
		}
		summarizer = indexer.NewSummarizer(llmClient, IsVerbose())
	}
	shouldSummarizeCodebase := enableSummaries && summarizer != nil && (isFirstIndex || ingestForceReindex || strings.TrimSpace(codebase.Summary) == "")
	if shouldSummarizeCodebase {
		readmeContent := ""
		for _, readmeName := range []string{"README.md", "readme.md", "Readme.md"} {
			readmePath := filepath.Join(absPath, readmeName)
			if data, err := os.ReadFile(readmePath); err == nil {
				readmeContent = string(data)
				break
			}
		}

		s.Suffix = " Generating codebase summary..."
		s.Start()
		summary, err := summarizer.SummarizeCodebase(ctx, scanResult, readmeContent)
		s.Stop()
		if err != nil {
			return fmt.Errorf("failed to generate codebase summary: %w\n\nTo skip summaries, use: --summary-mode off", err)
		}
		codebase.Summary = summary
		if codebase.Summary != "" {
			infoColor.Printf("  Summary: %s\n", truncate(codebase.Summary, 80))
		}
	}

	targetedPlan := targetedSummaryPlan{}
	if summaryMode == summaryModeTargeted {
		targetedSinceDate, dateErr := resolveIngestSinceDate()
		if dateErr != nil {
			return fmt.Errorf("invalid ingest window for targeted summaries: %w", dateErr)
		}
		targetedPlan, err = buildTargetedSummaryPlan(ctx, dbRepo, codebase, scanResult, targetedSinceDate, targetedSummaryOptions{
			LookbackDays:       ingestTargetedLookback,
			MaxActiveFolders:   ingestTargetedFolders,
			MinDistinctFiles:   ingestTargetedMinFiles,
			MaxChildItems:      ingestTargetedChildren,
			HighChurnThreshold: ingestTargetedHighChurn,
			FallbackTopFolders: 6,
		})
		if err != nil {
			return fmt.Errorf("failed to compute targeted summary plan: %w", err)
		}
		if targetedPlan.Digest != "" {
			codebase.ProjectContext = mergeProjectContext(codebase.ProjectContext, targetedPlan.Digest)
		}
		if targetedPlan.Reason != "" {
			dimColor.Printf("  Targeted paths: %s\n", targetedPlan.Reason)
		}
	}
	if err := dbRepo.UpsertCodebase(ctx, codebase); err != nil {
		return fmt.Errorf("failed to save codebase: %w", err)
	}

	existingFiles, err := dbRepo.GetExistingFileHashes(ctx, codebase.ID)
	if err != nil {
		return fmt.Errorf("failed to fetch existing files: %w", err)
	}

	existingFolders, err := dbRepo.GetExistingFolderPaths(ctx, codebase.ID)
	if err != nil {
		return fmt.Errorf("failed to fetch existing folders: %w", err)
	}
	existingFolderList, err := dbRepo.GetFoldersByCodebase(ctx, codebase.ID)
	if err != nil {
		return fmt.Errorf("failed to fetch existing folder metadata: %w", err)
	}
	existingFolderMeta := make(map[string]db.Folder, len(existingFolderList))
	for _, folder := range existingFolderList {
		existingFolderMeta[folder.Path] = folder
	}

	currentFilePaths := make(map[string]bool)
	for _, f := range scanResult.Files {
		currentFilePaths[f.Path] = true
	}
	currentFolderPaths := make(map[string]bool)
	for path := range scanResult.Folders {
		currentFolderPaths[path] = true
	}

	var newFiles, changedFiles, unchangedFiles []indexer.FileInfo
	for _, fileInfo := range scanResult.Files {
		existing, exists := existingFiles[fileInfo.Path]
		if !exists {
			newFiles = append(newFiles, fileInfo)
		} else if ingestForceReindex || existing.ContentHash != fileInfo.Hash {
			changedFiles = append(changedFiles, fileInfo)
		} else {
			unchangedFiles = append(unchangedFiles, fileInfo)
		}
	}

	var deletedFilePaths []string
	for path := range existingFiles {
		if !currentFilePaths[path] {
			deletedFilePaths = append(deletedFilePaths, path)
		}
	}

	var deletedFolderPaths []string
	for path := range existingFolders {
		if !currentFolderPaths[path] {
			deletedFolderPaths = append(deletedFolderPaths, path)
		}
	}

	if !isFirstIndex && !ingestForceReindex {
		dimColor.Printf("  Incremental: %d new, %d changed, %d unchanged, %d deleted\n",
			len(newFiles), len(changedFiles), len(unchangedFiles), len(deletedFilePaths))
	}

	if len(deletedFilePaths) > 0 {
		if err := dbRepo.DeleteFileIndexesByPaths(ctx, codebase.ID, deletedFilePaths); err != nil {
			VerboseLog("Warning: failed to delete removed files: %v", err)
		}
		VerboseLog("Deleted %d removed file indexes", len(deletedFilePaths))
	}
	if len(deletedFolderPaths) > 0 {
		if err := dbRepo.DeleteFoldersByPaths(ctx, codebase.ID, deletedFolderPaths); err != nil {
			VerboseLog("Warning: failed to delete removed folders: %v", err)
		}
		VerboseLog("Deleted %d removed folder indexes", len(deletedFolderPaths))
	}

	fmt.Println()
	dimColor.Printf("  Indexing folders...")
	folderCount := 0
	folderIDMap := make(map[string]string)

	for folderPath, folderInfo := range scanResult.Folders {
		folderID := existingFolders[folderPath]
		if folderID == "" {
			folderID = uuid.New().String()
		}
		folderIDMap[folderPath] = folderID

		folder := &db.Folder{
			ID:         folderID,
			CodebaseID: codebase.ID,
			Path:       folderPath,
			Name:       folderInfo.Name,
			Depth:      folderInfo.Depth,
			ParentPath: folderInfo.ParentPath,
			FileCount:  len(folderInfo.Files),
			IndexedAt:  time.Now(),
		}
		if existing, ok := existingFolderMeta[folderPath]; ok {
			folder.Summary = existing.Summary
			folder.Purpose = existing.Purpose
		}

		isNewFolder := existingFolders[folderPath] == ""
		if enableSummaries && summarizer != nil && len(folderInfo.Files) > 0 {
			shouldSummarizeFolder := false
			switch summaryMode {
			case summaryModeFull:
				shouldSummarizeFolder = folderInfo.Depth <= 2 && (isNewFolder || ingestForceReindex)
			case summaryModeTargeted:
				shouldSummarizeFolder = isFirstIndex || ingestForceReindex || (targetedPlan.ActiveFolders[folderPath] && targetedPlan.HighChurnFolders[folderPath])
			}
			if shouldSummarizeFolder {
				summary, err := summarizer.SummarizeFolder(ctx, folderInfo, targetedPlan.TouchedFilesByFolder[folderPath], ingestTargetedChildren)
				if err != nil {
					return fmt.Errorf("failed to generate folder summary for %s: %w\n\nTo skip summaries, use: --summary-mode off", folderPath, err)
				}
				folder.Summary = composeFolderSummary(summary)
				folder.Purpose = summary.Purpose
			}
		}

		if err := dbRepo.UpsertFolder(ctx, folder); err != nil {
			VerboseLog("Warning: failed to save folder %s: %v", folderPath, err)
		}

		folderCount++
		fmt.Printf("\r  Processed %d/%d folders", folderCount, len(scanResult.Folders))
	}
	fmt.Println()

	filesToProcess := append(newFiles, changedFiles...)
	dimColor.Printf("  Indexing files...")
	fileCount := 0
	summarizedCount := 0
	totalFiles := len(filesToProcess) + len(unchangedFiles)

	for _, fileInfo := range filesToProcess {
		existingInfo := existingFiles[fileInfo.Path]
		fileID := existingInfo.ID
		if fileID == "" {
			fileID = uuid.New().String()
		}

		folderPath := filepath.Dir(fileInfo.Path)
		if folderPath == "." {
			folderPath = "."
		}
		folderID := folderIDMap[folderPath]

		file := &db.FileIndex{
			ID:          fileID,
			CodebaseID:  codebase.ID,
			FolderID:    folderID,
			Path:        fileInfo.Path,
			Name:        fileInfo.Name,
			Extension:   fileInfo.Extension,
			Language:    fileInfo.Language,
			SizeBytes:   fileInfo.Size,
			LineCount:   indexer.CountLines(fileInfo.Content),
			ContentHash: fileInfo.Hash,
			Summary:     existingInfo.Summary,
			IndexedAt:   time.Now(),
		}

		shouldSummarizeTargetedFile := summaryMode == summaryModeTargeted &&
			(targetedPlan.HighChurnFolders[folderPath] || isFirstIndex || ingestForceReindex)
		if enableSummaries && summarizer != nil && shouldSummarizeFile(fileInfo) &&
			((summaryMode == summaryModeFull) || shouldSummarizeTargetedFile) {
			summary, err := summarizer.SummarizeFile(ctx, fileInfo)
			if err != nil {
				return fmt.Errorf("failed to generate file summary for %s: %w\n\nTo skip summaries, use: --summary-mode off", fileInfo.Path, err)
			}
			file.Summary = summary.Summary
			file.Purpose = summary.Purpose
			file.KeyExports = summary.KeyExports
			summarizedCount++
		}

		if err := dbRepo.UpsertFileIndex(ctx, file); err != nil {
			return fmt.Errorf("failed to save file %s: %w", fileInfo.Path, err)
		}

		fileCount++
		if fileCount%10 == 0 || fileCount == len(filesToProcess) {
			fmt.Printf("\r  Processed %d/%d files (summarizing)", fileCount, len(filesToProcess))
		}
	}

	for _, fileInfo := range unchangedFiles {
		existingInfo := existingFiles[fileInfo.Path]

		folderPath := filepath.Dir(fileInfo.Path)
		if folderPath == "." {
			folderPath = "."
		}
		folderID := folderIDMap[folderPath]

		file := &db.FileIndex{
			ID:          existingInfo.ID,
			CodebaseID:  codebase.ID,
			FolderID:    folderID,
			Path:        fileInfo.Path,
			Name:        fileInfo.Name,
			Extension:   fileInfo.Extension,
			Language:    fileInfo.Language,
			SizeBytes:   fileInfo.Size,
			LineCount:   indexer.CountLines(fileInfo.Content),
			ContentHash: fileInfo.Hash,
			Summary:     existingInfo.Summary, // Preserve existing summary
			IndexedAt:   time.Now(),
		}

		if err := dbRepo.UpsertFileIndex(ctx, file); err != nil {
			return fmt.Errorf("failed to save unchanged file %s: %w", fileInfo.Path, err)
		}
	}

	fmt.Printf("\r  Processed %d/%d files                    \n", totalFiles, totalFiles)

	fmt.Println()
	stats, err := dbRepo.GetCodebaseStats(ctx, codebase.ID)
	if err != nil {
		return fmt.Errorf("failed to get codebase stats: %w", err)
	}
	dimColor.Printf("  Folders:    ")
	infoColor.Printf("%d\n", stats.FolderCount)
	dimColor.Printf("  Files:      ")
	infoColor.Printf("%d\n", stats.FileCount)
	dimColor.Printf("  Total size: ")
	infoColor.Printf("%s\n", formatBytes(stats.TotalSize))
	dimColor.Printf("  Lines:      ")
	infoColor.Printf("%d\n", stats.TotalLines)

	if summarizedCount > 0 {
		dimColor.Printf("  Summaries:  ")
		infoColor.Printf("%d files (new/changed)\n", summarizedCount)
	}

	if len(deletedFilePaths) > 0 {
		dimColor.Printf("  Removed:    ")
		infoColor.Printf("%d files\n", len(deletedFilePaths))
	}

	return nil
}

type targetedSummaryOptions struct {
	LookbackDays       int
	MaxActiveFolders   int
	MinDistinctFiles   int
	MaxChildItems      int
	HighChurnThreshold int
	FallbackTopFolders int
}

type folderTouchEvidence struct {
	FilePath    string
	FolderPath  string
	TouchCount  int
	CommittedAt time.Time
	Additions   int
	Deletions   int
}

type targetedSummaryPlan struct {
	ActiveFolders        map[string]bool
	HighChurnFolders     map[string]bool
	RankedFolders        []string
	TouchedFilesByFolder map[string][]string
	Digest               string
	Reason               string
}

type folderActivity struct {
	distinctTouched map[string]bool
	touchCount      int
	churn           int
	lastTouched     time.Time
}

func resolveSummaryMode(totalFiles int) (string, string) {
	if ingestSkipSummaries {
		return summaryModeOff, "--skip-summaries alias"
	}

	mode := strings.ToLower(strings.TrimSpace(ingestSummaryMode))
	switch mode {
	case "", summaryModeAuto:
		if totalFiles > indexSoftLimit {
			return summaryModeTargeted, fmt.Sprintf("auto-selected for %d files", totalFiles)
		}
		return summaryModeFull, fmt.Sprintf("auto-selected for %d files", totalFiles)
	case summaryModeFull:
		return summaryModeFull, "explicit"
	case summaryModeTargeted:
		return summaryModeTargeted, "explicit"
	case summaryModeOff:
		return summaryModeOff, "explicit"
	default:
		if totalFiles > indexSoftLimit {
			return summaryModeTargeted, fmt.Sprintf("unknown mode '%s', fell back to auto-targeted", mode)
		}
		return summaryModeFull, fmt.Sprintf("unknown mode '%s', fell back to auto-full", mode)
	}
}

func resolveIngestSinceDate() (time.Time, error) {
	if ingestAll {
		return time.Time{}, nil
	}
	if ingestSince != "" {
		sinceDate, err := time.Parse("2006-01-02", ingestSince)
		if err != nil {
			return time.Time{}, err
		}
		return sinceDate, nil
	}
	days := ingestDays
	if days <= 0 {
		days = 30
	}
	return time.Now().AddDate(0, 0, -days), nil
}

func buildTargetedSummaryPlan(
	ctx context.Context,
	dbRepo *db.SQLRepository,
	codebase *db.Codebase,
	scanResult *indexer.ScanResult,
	sinceDate time.Time,
	opts targetedSummaryOptions,
) (targetedSummaryPlan, error) {
	evidence, err := loadFolderTouchEvidence(ctx, dbRepo, codebase, sinceDate, opts.LookbackDays)
	if err != nil {
		return targetedSummaryPlan{}, err
	}
	return computeTargetedSummaryPlan(scanResult, evidence, sinceDate, opts), nil
}

func loadFolderTouchEvidence(ctx context.Context, dbRepo *db.SQLRepository, codebase *db.Codebase, sinceDate time.Time, lookbackDays int) ([]folderTouchEvidence, error) {
	if sinceDate.IsZero() && lookbackDays > 0 {
		sinceDate = time.Now().AddDate(0, 0, -lookbackDays)
	}
	result := make([]folderTouchEvidence, 0)
	if codebase != nil && len(codebase.TouchActivity) > 0 {
		result = make([]folderTouchEvidence, 0, len(codebase.TouchActivity))
		for folderPath, raw := range codebase.TouchActivity {
			entry := parseTouchEntry(raw)
			lastTouched, ok := parseTimeAny(entry["last_touched_at"])
			if !ok {
				continue
			}
			if !sinceDate.IsZero() && lastTouched.Before(sinceDate) {
				continue
			}
			result = append(result, folderTouchEvidence{
				FolderPath:  normalizeFolderPath(folderPath),
				TouchCount:  toInt(entry["touch_count"]),
				CommittedAt: lastTouched,
				Additions:   toInt(entry["churn"]),
			})
		}
		if len(result) > 0 {
			return result, nil
		}
	}

	// Backward-compatible fallback for existing databases: rebuild from ingested commits.
	if codebase == nil {
		return nil, nil
	}
	query := `
		SELECT fc.file_path, c.committed_at, fc.additions, fc.deletions
		FROM file_changes fc
		INNER JOIN commits c ON c.id = fc.commit_id
		WHERE c.codebase_id = $1
		  AND c.is_user_commit = TRUE
	`
	args := []any{codebase.ID}
	if !sinceDate.IsZero() {
		query += " AND c.committed_at >= $2"
		args = append(args, sinceDate)
	}
	query += " ORDER BY c.committed_at DESC"

	rows, err := dbRepo.ExecuteQueryWithArgs(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	result = make([]folderTouchEvidence, 0, len(rows))
	for _, row := range rows {
		filePath := normalizeFolderPath(parseRowString(row["file_path"]))
		if filePath == "" || filePath == "." {
			continue
		}
		folderPath := normalizeFolderPath(filepath.Dir(filePath))
		result = append(result, folderTouchEvidence{
			FilePath:    filePath,
			FolderPath:  folderPath,
			TouchCount:  1,
			CommittedAt: parseRowTime(row["committed_at"]),
			Additions:   parseRowInt(row["additions"]),
			Deletions:   parseRowInt(row["deletions"]),
		})
	}
	return result, nil
}

func computeTargetedSummaryPlan(scanResult *indexer.ScanResult, evidence []folderTouchEvidence, sinceDate time.Time, opts targetedSummaryOptions) targetedSummaryPlan {
	plan := targetedSummaryPlan{
		ActiveFolders:        map[string]bool{".": true},
		HighChurnFolders:     map[string]bool{},
		TouchedFilesByFolder: map[string][]string{},
	}
	if opts.MaxActiveFolders <= 0 {
		opts.MaxActiveFolders = 40
	}
	if opts.MinDistinctFiles <= 0 {
		opts.MinDistinctFiles = 2
	}
	if opts.FallbackTopFolders <= 0 {
		opts.FallbackTopFolders = 6
	}
	if opts.HighChurnThreshold <= 0 {
		opts.HighChurnThreshold = 500
	}

	scannedFolders := make(map[string]bool, len(scanResult.Folders))
	for path := range scanResult.Folders {
		scannedFolders[normalizeFolderPath(path)] = true
	}

	activity := make(map[string]*folderActivity)
	for _, item := range evidence {
		folderPath := normalizeFolderPath(item.FolderPath)
		if !scannedFolders[folderPath] {
			continue
		}
		entry := activity[folderPath]
		if entry == nil {
			entry = &folderActivity{distinctTouched: map[string]bool{}}
			activity[folderPath] = entry
		}
		if item.FilePath != "" {
			entry.distinctTouched[item.FilePath] = true
		}
		increment := item.TouchCount
		if increment <= 0 {
			increment = 1
		}
		entry.touchCount += increment
		entry.churn += item.Additions + item.Deletions
		if item.CommittedAt.After(entry.lastTouched) {
			entry.lastTouched = item.CommittedAt
		}
	}

	if len(activity) == 0 {
		plan.RankedFolders = bootstrapTopFolders(scanResult, opts.FallbackTopFolders)
		for _, path := range plan.RankedFolders {
			plan.ActiveFolders[path] = true
			plan.TouchedFilesByFolder[path] = []string{"bootstrap structural context"}
		}
		plan.Digest = buildTargetedDigest(plan.RankedFolders, sinceDate, true)
		plan.Reason = "no user commit history, used structural bootstrap"
		return plan
	}

	directlyActive := make(map[string]bool)
	scoreByFolder := map[string]float64{".": 1}
	for folderPath, item := range activity {
		distinctCount := len(item.distinctTouched)
		score := computeFolderActivityScore(item)
		scoreByFolder[folderPath] += score
		if intMax(distinctCount, item.touchCount) >= opts.MinDistinctFiles {
			directlyActive[folderPath] = true
		}
		if item.churn >= opts.HighChurnThreshold {
			plan.HighChurnFolders[folderPath] = true
		}
	}

	if len(directlyActive) == 0 {
		topFolder := ""
		topScore := -1.0
		for folderPath, item := range activity {
			score := computeFolderActivityScore(item)
			if score > topScore {
				topFolder = folderPath
				topScore = score
			}
		}
		if topFolder != "" {
			directlyActive[topFolder] = true
		}
	}

	for folderPath := range directlyActive {
		ancestors := folderAndAncestors(folderPath)
		propagation := 0.9
		for _, ancestor := range ancestors {
			if !scannedFolders[ancestor] {
				continue
			}
			plan.ActiveFolders[ancestor] = true
			scoreByFolder[ancestor] += scoreByFolder[folderPath] * propagation
			propagation *= 0.7
		}
	}

	activeList := make([]string, 0, len(plan.ActiveFolders))
	for folderPath := range plan.ActiveFolders {
		if scannedFolders[folderPath] {
			activeList = append(activeList, folderPath)
		}
	}
	sort.Slice(activeList, func(i, j int) bool {
		left := scoreByFolder[activeList[i]]
		right := scoreByFolder[activeList[j]]
		if left == right {
			leftDepth := strings.Count(activeList[i], "/")
			rightDepth := strings.Count(activeList[j], "/")
			if leftDepth == rightDepth {
				return activeList[i] < activeList[j]
			}
			return leftDepth < rightDepth
		}
		return left > right
	})

	selected := activeList
	if len(selected) > opts.MaxActiveFolders {
		selected = selected[:opts.MaxActiveFolders]
	}

	selectedSet := map[string]bool{".": true}
	for _, path := range selected {
		for _, ancestor := range folderAndAncestors(path) {
			if scannedFolders[ancestor] {
				selectedSet[ancestor] = true
			}
		}
	}
	plan.ActiveFolders = selectedSet

	ranked := make([]string, 0, len(selectedSet))
	for path := range selectedSet {
		ranked = append(ranked, path)
	}
	sort.Slice(ranked, func(i, j int) bool {
		left := scoreByFolder[ranked[i]]
		right := scoreByFolder[ranked[j]]
		if left == right {
			return ranked[i] < ranked[j]
		}
		return left > right
	})
	plan.RankedFolders = ranked

	for folderPath, item := range activity {
		if !selectedSet[folderPath] {
			continue
		}
		files := make([]string, 0, len(item.distinctTouched))
		for filePath := range item.distinctTouched {
			files = append(files, filePath)
		}
		sort.Strings(files)
		if len(files) > opts.MaxChildItems && opts.MaxChildItems > 0 {
			files = files[:opts.MaxChildItems]
		}
		plan.TouchedFilesByFolder[folderPath] = files
	}
	for folderPath := range selectedSet {
		if len(plan.TouchedFilesByFolder[folderPath]) > 0 {
			continue
		}
		var inherited []string
		prefix := folderPath + "/"
		if folderPath == "." {
			prefix = ""
		}
		for _, item := range evidence {
			if prefix == "" || strings.HasPrefix(item.FilePath, prefix) {
				inherited = append(inherited, item.FilePath)
			}
		}
		sort.Strings(inherited)
		inherited = uniqueStrings(inherited)
		if len(inherited) > opts.MaxChildItems && opts.MaxChildItems > 0 {
			inherited = inherited[:opts.MaxChildItems]
		}
		if len(inherited) > 0 {
			plan.TouchedFilesByFolder[folderPath] = inherited
		}
	}

	plan.Digest = buildTargetedDigest(topNNonRoot(plan.RankedFolders, 8), sinceDate, false)
	plan.Reason = fmt.Sprintf("computed from %d touched folders, selected %d active paths", len(activity), len(selectedSet))
	return plan
}

func computeFolderActivityScore(item *folderActivity) float64 {
	if item == nil {
		return 0
	}
	distinctWeight := float64(len(item.distinctTouched) * 4)
	touchWeight := float64(item.touchCount)
	churnWeight := float64(item.churn) / 30.0
	recencyWeight := 0.0
	if !item.lastTouched.IsZero() {
		daysAgo := time.Since(item.lastTouched).Hours() / 24
		recencyWeight = math.Max(0, 45-daysAgo)
	}
	return distinctWeight + touchWeight + churnWeight + recencyWeight
}

func folderAndAncestors(path string) []string {
	path = normalizeFolderPath(path)
	ancestors := []string{path}
	for path != "." {
		path = normalizeFolderPath(filepath.Dir(path))
		if path == "" {
			path = "."
		}
		ancestors = append(ancestors, path)
		if path == "." {
			break
		}
	}
	return ancestors
}

func bootstrapTopFolders(scanResult *indexer.ScanResult, max int) []string {
	type topFolder struct {
		path      string
		fileCount int
	}
	var folders []topFolder
	for path, info := range scanResult.Folders {
		if info.Depth == 1 {
			folders = append(folders, topFolder{path: path, fileCount: len(info.Files)})
		}
	}
	sort.Slice(folders, func(i, j int) bool {
		if folders[i].fileCount == folders[j].fileCount {
			return folders[i].path < folders[j].path
		}
		return folders[i].fileCount > folders[j].fileCount
	})
	if max <= 0 || len(folders) <= max {
		result := []string{"."}
		for _, folder := range folders {
			result = append(result, folder.path)
		}
		return result
	}
	result := []string{"."}
	for _, folder := range folders[:max] {
		result = append(result, folder.path)
	}
	return result
}

func topNNonRoot(paths []string, max int) []string {
	var result []string
	for _, path := range paths {
		if path == "." {
			continue
		}
		result = append(result, path)
		if max > 0 && len(result) >= max {
			break
		}
	}
	return result
}

func buildTargetedDigest(paths []string, sinceDate time.Time, fallback bool) string {
	var sb strings.Builder
	sb.WriteString("[Targeted ingest context]\n")
	if fallback {
		sb.WriteString("No user commit history was available; generated bootstrap folder context.\n")
	} else {
		if sinceDate.IsZero() {
			sb.WriteString("Active paths derived from user commits across full ingest history.\n")
		} else {
			sb.WriteString(fmt.Sprintf("Active paths derived from user commits since %s.\n", sinceDate.Format("2006-01-02")))
		}
	}
	if len(paths) == 0 {
		sb.WriteString("- No active paths detected")
		return sb.String()
	}
	sb.WriteString("Top active paths:\n")
	for _, path := range paths {
		sb.WriteString("- ")
		sb.WriteString(path)
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

func mergeProjectContext(existing, digest string) string {
	if digest == "" {
		return existing
	}
	parts := strings.Split(existing, "\n\n[Targeted ingest context]")
	base := strings.TrimSpace(parts[0])
	if base == "" {
		return digest
	}
	return base + "\n\n" + digest
}

func composeFolderSummary(summary *indexer.FolderSummary) string {
	if summary == nil {
		return ""
	}
	var lines []string
	if summary.Summary != "" {
		lines = append(lines, summary.Summary)
	}
	if summary.Themes != "" {
		lines = append(lines, "Themes: "+summary.Themes)
	}
	if len(summary.FileDescriptions) > 0 {
		lines = append(lines, "Files:")
		for _, item := range summary.FileDescriptions {
			lines = append(lines, "- "+item)
		}
	}
	if len(summary.SubfolderDescriptions) > 0 {
		lines = append(lines, "Subfolders:")
		for _, item := range summary.SubfolderDescriptions {
			lines = append(lines, "- "+item)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func normalizeFolderPath(path string) string {
	clean := strings.Trim(strings.TrimSpace(filepath.Clean(path)), string(os.PathSeparator))
	if clean == "" || clean == "." {
		return "."
	}
	return clean
}

func parseRowString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return ""
	}
}

func parseRowInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float32:
		return int(t)
	case float64:
		return int(t)
	default:
		return 0
	}
}

func parseRowTime(v any) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t
	default:
		return time.Time{}
	}
}

func uniqueStrings(items []string) []string {
	if len(items) == 0 {
		return items
	}
	result := []string{items[0]}
	for i := 1; i < len(items); i++ {
		if items[i] != items[i-1] {
			result = append(result, items[i])
		}
	}
	return result
}

func intMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func parseTouchEntry(raw any) map[string]any {
	switch t := raw.(type) {
	case map[string]any:
		return t
	default:
		return map[string]any{}
	}
}

func toInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float32:
		return int(t)
	case float64:
		return int(t)
	case string:
		if parsed, err := strconv.Atoi(t); err == nil {
			return parsed
		}
	}
	return 0
}

func parseTimeAny(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case string:
		parsed, err := time.Parse(time.RFC3339, t)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func createLLMClient(cfg *config.Config) (llm.Client, error) {
	provider := cfg.GetEffectiveProvider()
	if provider == "" {
		return nil, fmt.Errorf("no provider configured; run 'devlog onboard' first")
	}
	llmCfg := llm.Config{Provider: llm.Provider(provider), Model: cfg.GetEffectiveModel()}
	switch llmCfg.Provider {
	case llm.ProviderOpenAI:
		llmCfg.APIKey = cfg.GetEffectiveAPIKey("openai")
	case llm.ProviderChatGPT:
		llmCfg.APIKey = cfg.GetEffectiveAPIKey("chatgpt")
	case llm.ProviderAnthropic:
		llmCfg.APIKey = cfg.GetEffectiveAPIKey("anthropic")
	case llm.ProviderOpenRouter:
		llmCfg.APIKey = cfg.GetEffectiveAPIKey("openrouter")
	case llm.ProviderGemini:
		llmCfg.APIKey = cfg.GetEffectiveAPIKey("gemini")
	case llm.ProviderBedrock:
		llmCfg.AWSAccessKeyID = cfg.GetEffectiveAWSAccessKeyID()
		llmCfg.AWSSecretAccessKey = cfg.GetEffectiveAWSSecretAccessKey()
		llmCfg.AWSRegion = cfg.GetEffectiveAWSRegion()
	case llm.ProviderOllama:
		if url := cfg.GetEffectiveOllamaBaseURL(); url != "" {
			llmCfg.BaseURL = url
		}
		if model := cfg.GetEffectiveOllamaModel(); model != "" {
			llmCfg.Model = model
		}
	}
	return llm.NewClient(llmCfg)
}

func shouldSummarizeFile(f indexer.FileInfo) bool {
	if f.Content == "" || f.Language == "" {
		return false
	}
	if len(f.Content) < 100 {
		return false
	}
	skip := []string{".json", ".yaml", ".yml", ".toml", ".ini", ".env", ".md", ".txt"}
	for _, ext := range skip {
		if f.Extension == ext {
			return false
		}
	}
	return true
}

func joinMax(items []string, max int) string {
	if len(items) > max {
		items = items[:max]
	}
	result := ""
	for i, item := range items {
		if i > 0 {
			result += ", "
		}
		result += item
	}
	if len(items) < max {
		return result
	}
	return result + "..."
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func generateCommitSummary(client llm.Client, commitMessage string, fileChanges []*db.FileChange, projectContext string) (string, error) {
	var sb strings.Builder
	sb.WriteString("Commit message: ")
	sb.WriteString(commitMessage)
	sb.WriteString("\n\nFiles changed:\n")

	totalAdditions := 0
	totalDeletions := 0

	for _, fc := range fileChanges {
		sb.WriteString(fmt.Sprintf("- %s (%s): +%d/-%d\n", fc.FilePath, fc.ChangeType, fc.Additions, fc.Deletions))
		totalAdditions += fc.Additions
		totalDeletions += fc.Deletions

		// Include full patch/diff for context
		if fc.Patch != "" {
			sb.WriteString("  Diff:\n")
			lines := strings.Split(fc.Patch, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
					if !strings.HasPrefix(line, "+++") && !strings.HasPrefix(line, "---") {
						sb.WriteString(fmt.Sprintf("    %s\n", line))
					}
				}
			}
		}
	}

	sb.WriteString(fmt.Sprintf("\nTotal: +%d/-%d lines across %d files\n", totalAdditions, totalDeletions, len(fileChanges)))

	prompt := prompts.BuildCommitSummarizerPrompt(projectContext, sb.String())

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	return client.Complete(ctx, prompt)
}

func fillMissingSummaries(ctx context.Context, dbRepo *db.SQLRepository, repo *git.Repository, codebase *db.Codebase, llmClient llm.Client) (int, error) {
	dimColor := color.New(color.FgHiBlack)

	commits, err := dbRepo.GetUserCommitsMissingSummaries(ctx, codebase.ID)
	if err != nil {
		return 0, fmt.Errorf("failed to get commits missing summaries: %w", err)
	}

	if len(commits) == 0 {
		VerboseLog("No commits missing summaries")
		return 0, nil
	}
	dimColor.Printf("  Filling %d missing commit summaries...\n", len(commits))
	filled := 0
	for i, commit := range commits {
		fileChanges, err := dbRepo.GetFileChangesByCommit(ctx, commit.ID)
		if err != nil {
			return 0, fmt.Errorf("failed to get file changes for commit %s: %w", commit.Hash[:8], err)
		}
		if len(fileChanges) == 0 {
			VerboseLog("Skipping commit %s: no file changes", commit.Hash[:8])
			continue
		}
		var fcPtrs []*db.FileChange
		for j := range fileChanges {
			fcPtrs = append(fcPtrs, &fileChanges[j])
		}
		projectCtx := ""
		if codebase != nil {
			projectCtx = codebase.Summary
		}
		summary, err := generateCommitSummary(llmClient, commit.Message, fcPtrs, projectCtx)
		if err != nil {
			return 0, fmt.Errorf("failed to generate summary for commit %s: %w", commit.Hash[:8], err)
		}
		if err := dbRepo.UpdateCommitSummary(ctx, commit.ID, summary); err != nil {
			return 0, fmt.Errorf("failed to update summary for commit %s: %w", commit.Hash[:8], err)
		}
		filled++
		if (i+1)%10 == 0 || i+1 == len(commits) {
			fmt.Printf("\r  Processed %d/%d commits", i+1, len(commits))
		}
	}

	if len(commits) > 0 {
		fmt.Println()
	}

	return filled, nil
}
