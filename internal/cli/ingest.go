package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

var (
	ingestDays           int
	ingestAll            bool
	ingestSince          string
	ingestBranches       []string
	ingestAllBranches    bool
	ingestReselectBranch bool
	ingestSkipSummaries  bool
	ingestMaxFiles       int
	ingestGitOnly        bool
	ingestIndexOnly      bool
	ingestSkipCommitSums bool
	ingestFillSummaries  bool
	ingestForceReindex   bool
	ingestSkipWorklog    bool
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
  devlog ingest --index-only          # Only indexing, skip git history`,
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
	ingestCmd.Flags().IntVar(&ingestMaxFiles, "max-files", 500, "Maximum files to index")
	ingestCmd.Flags().BoolVar(&ingestGitOnly, "git-only", false, "Only ingest git history")
	ingestCmd.Flags().BoolVar(&ingestIndexOnly, "index-only", false, "Only index codebase")
	ingestCmd.Flags().BoolVar(&ingestSkipCommitSums, "skip-commit-summaries", false, "Skip LLM-generated commit summaries")
	ingestCmd.Flags().BoolVar(&ingestFillSummaries, "fill-summaries", false, "Generate summaries for existing commits that are missing them")
	ingestCmd.Flags().BoolVar(&ingestForceReindex, "force-reindex", false, "Force re-indexing all files, ignoring content hashes")
	ingestCmd.Flags().BoolVar(&ingestSkipWorklog, "skip-worklog", false, "Skip worklog generation prompt after ingestion")
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
	if !ingestIndexOnly {
		if err := ingestGitHistory(absPath, cfg); err != nil {
			VerboseLog("Git ingest warning: %v", err)
			dimColor.Printf("  Note: Git ingestion skipped (%v)\n\n", err)
		} else {
			gitHistoryIngested = true
		}
	}

	if !ingestGitOnly {
		if err := indexCodebase(absPath, cfg); err != nil {
			return fmt.Errorf("indexing failed: %w", err)
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

	style := cfg.GetWorklogStyle()
	if style == "" {
		style = "non-technical"
	}

	cache := &worklogCacheContext{
		dbRepo:      dbRepo,
		codebaseID:  codebase.ID,
		profileName: cfg.GetActiveProfileName(),
		loc:         loc,
		noCache:     false,
		ChangedDailySummaries: make(map[time.Time]bool),
	}

	groups := groupByDate(commits, loc)
	markdown, err := generateWorklogMarkdown(groups, client, cfg, loc, projectContext, codebaseContext, cache, style)
	if err != nil {
		return fmt.Errorf("failed to generate markdown: %w", err)
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

	selection, err := selectBranches(allBranches, codebase.DefaultBranch, cfg, absPath)
	if err != nil {
		return fmt.Errorf("branch selection failed: %w", err)
	}

	if selection.MainBranch != codebase.DefaultBranch {
		codebase.DefaultBranch = selection.MainBranch
		dbRepo.UpsertCodebase(ctx, codebase)
	}

	var sinceDate time.Time
	if ingestAll {
		dimColor.Println("  Ingesting full history...")
	} else if ingestSince != "" {
		sinceDate, err = time.Parse("2006-01-02", ingestSince)
		if err != nil {
			return fmt.Errorf("invalid date format (use YYYY-MM-DD): %w", err)
		}
		// Load timezone for display
		loc := time.UTC
		if tzName := cfg.GetTimezone(); tzName != "" {
			if tzLoc, err := time.LoadLocation(tzName); err == nil {
				loc = tzLoc
			}
		}
		dimColor.Printf("  Since %s...\n", sinceDate.In(loc).Format("Jan 2, 2006"))
	} else {
		sinceDate = time.Now().AddDate(0, 0, -ingestDays)
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
		branchInfo.IsDefault = true
		commits, files, err := ingestBranch(ctx, dbRepo, repo, codebase, branchInfo, "", sinceDate, userEmail, githubUsername, llmClient, existingHashes)
		if err != nil {
			return fmt.Errorf("failed to ingest branch %s: %w", branchInfo.Name, err)
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
		commits, files, err := ingestBranch(ctx, dbRepo, repo, codebase, branchInfo, selection.MainBranch, sinceDate, userEmail, githubUsername, llmClient, existingHashes)
		if err != nil {
			return fmt.Errorf("failed to ingest branch %s: %w", branchInfo.Name, err)
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

func selectBranches(branches []git.BranchInfo, detectedDefault string, cfg *config.Config, repoPath string) (*BranchSelection, error) {
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

			promptColor.Printf("  [Enter] Use current selection  [m] Modify  [r] Reselect all: ")

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
				return &BranchSelection{
					MainBranch:       saved.MainBranch,
					SelectedBranches: validBranches,
				}, nil
			}
		}
	}

	fmt.Println()
	selection, err := tui.RunBranchSelection(branches, detectedDefault)
	if err != nil {
		return nil, err
	}

	return saveBranchSelection(cfg, profileName, repoPath, selection, dimColor)
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
		commitHashes, err = repo.GetCommitsOnBranch(branchInfo.Name, "")
	} else {
		commitHashes, err = repo.GetCommitsOnBranch(branchInfo.Name, baseBranch)
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

		stats, fileChanges, err := getCommitStats(repo, gitCommit)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to get stats for commit %s: %w", hash[:8], err)
		}

		var commitSummary string
		if isUserCommit && llmClient != nil && len(fileChanges) > 0 {
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
	}

	return commitCount, fileCount, nil
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

	titleColor.Printf("  Codebase Indexing\n")

	dbRepo, err := db.GetRepository()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Scanning files..."
	s.Color("cyan")
	s.Start()

	scanResult, err := indexer.ScanCodebase(absPath, 500*1024)
	if err != nil {
		s.Stop()
		return fmt.Errorf("failed to scan codebase: %w", err)
	}
	s.Stop()

	if ingestMaxFiles > 0 && len(scanResult.Files) > ingestMaxFiles {
		scanResult.Files = scanResult.Files[:ingestMaxFiles]
		warnColor.Printf("  Limited to %d files\n", ingestMaxFiles)
	}

	successColor.Printf("  Found %d files in %d folders\n", len(scanResult.Files), len(scanResult.Folders))

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

	if !ingestSkipSummaries {
		if llmClient, err = createLLMClient(cfg); err != nil {
			return fmt.Errorf("failed to initialize LLM client: %w\n\nTo skip file/folder summaries, use: --skip-summaries", err)
		}
		summarizer = indexer.NewSummarizer(llmClient, IsVerbose())
	}
	if !ingestSkipSummaries && summarizer != nil {
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
			return fmt.Errorf("failed to generate codebase summary: %w\n\nTo skip summaries, use: --skip-summaries", err)
		}
		codebase.Summary = summary
		if codebase.Summary != "" {
			infoColor.Printf("  Summary: %s\n", truncate(codebase.Summary, 80))
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

		isNewFolder := existingFolders[folderPath] == ""
		if !ingestSkipSummaries && summarizer != nil && folderInfo.Depth <= 2 && len(folderInfo.Files) > 0 {
			if isNewFolder || ingestForceReindex {
				summary, err := summarizer.SummarizeFolder(ctx, folderInfo)
				if err != nil {
					return fmt.Errorf("failed to generate folder summary for %s: %w\n\nTo skip summaries, use: --skip-summaries", folderPath, err)
				}
				folder.Summary = summary.Summary
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
			IndexedAt:   time.Now(),
		}

		if !ingestSkipSummaries && summarizer != nil && shouldSummarizeFile(fileInfo) {
			summary, err := summarizer.SummarizeFile(ctx, fileInfo)
			if err != nil {
				return fmt.Errorf("failed to generate file summary for %s: %w\n\nTo skip summaries, use: --skip-summaries", fileInfo.Path, err)
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
