package cli

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/db"
	"github.com/ishaan812/devlog/internal/git"
	"github.com/ishaan812/devlog/internal/indexer"
	"github.com/ishaan812/devlog/internal/llm"
	"github.com/ishaan812/devlog/internal/tui"
	"github.com/spf13/cobra"
)

// Regex to extract GitHub username from noreply emails
// Matches: username@users.noreply.github.com or 12345+username@users.noreply.github.com
var githubNoReplyRegex = regexp.MustCompile(`^(?:\d+\+)?([^@]+)@users\.noreply\.github\.com$`)

// extractGitHubUsername extracts the GitHub username from an email if it's a GitHub noreply email
func extractGitHubUsername(email string) string {
	matches := githubNoReplyRegex.FindStringSubmatch(strings.ToLower(email))
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// isUserCommitByGitHub checks if a commit author matches the configured GitHub username
func isUserCommitByGitHub(authorEmail string, githubUsername string) bool {
	if githubUsername == "" {
		return false
	}
	extractedUsername := extractGitHubUsername(authorEmail)
	return strings.EqualFold(extractedUsername, githubUsername)
}

var (
	// Git history flags
	ingestDays  int
	ingestAll   bool
	ingestSince string

	// Branch selection flags
	ingestBranches    []string
	ingestAllBranches bool

	// Indexing flags
	ingestSkipSummaries  bool
	ingestSkipEmbeddings bool
	ingestMaxFiles       int

	// Mode flags
	ingestGitOnly        bool
	ingestIndexOnly      bool
	ingestSkipCommitSums bool
	ingestFillSummaries  bool
	ingestForceReindex   bool
)

var ingestCmd = &cobra.Command{
	Use:   "ingest [path]",
	Short: "Ingest git history and index codebase",
	Long: `Ingest git commit history and index codebase for semantic search.

This unified command performs two phases:
  1. Git History Ingestion - Scan commits and store in the database
  2. Codebase Indexing - Generate summaries and embeddings for search

The repository is automatically added to the active profile.

By default, you'll be prompted to select which branches to ingest.
The main/default branch is ingested fully, while feature branches only
ingest commits unique to that branch (not on the main branch).

Examples:
  devlog ingest                       # Interactive branch selection
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

	// Git history flags
	ingestCmd.Flags().IntVar(&ingestDays, "days", 30, "Number of days of history to ingest")
	ingestCmd.Flags().BoolVar(&ingestAll, "all", false, "Ingest full git history (ignores --days)")
	ingestCmd.Flags().StringVar(&ingestSince, "since", "", "Ingest commits since date (YYYY-MM-DD)")

	// Branch selection flags
	ingestCmd.Flags().StringSliceVar(&ingestBranches, "branches", nil, "Specific branches to ingest (comma-separated)")
	ingestCmd.Flags().BoolVar(&ingestAllBranches, "all-branches", false, "Ingest all branches without prompting")

	// Indexing flags
	ingestCmd.Flags().BoolVar(&ingestSkipSummaries, "skip-summaries", false, "Skip LLM-generated summaries")
	ingestCmd.Flags().BoolVar(&ingestSkipEmbeddings, "skip-embeddings", false, "Skip embedding generation")
	ingestCmd.Flags().IntVar(&ingestMaxFiles, "max-files", 500, "Maximum files to index")

	// Mode flags
	ingestCmd.Flags().BoolVar(&ingestGitOnly, "git-only", false, "Only ingest git history")
	ingestCmd.Flags().BoolVar(&ingestIndexOnly, "index-only", false, "Only index codebase")
	ingestCmd.Flags().BoolVar(&ingestSkipCommitSums, "skip-commit-summaries", false, "Skip LLM-generated commit summaries")
	ingestCmd.Flags().BoolVar(&ingestFillSummaries, "fill-summaries", false, "Generate summaries for existing commits that are missing them")
	ingestCmd.Flags().BoolVar(&ingestForceReindex, "force-reindex", false, "Force re-indexing all files, ignoring content hashes")
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

	// Colors
	titleColor := color.New(color.FgHiCyan, color.Bold)
	successColor := color.New(color.FgHiGreen)
	dimColor := color.New(color.FgHiBlack)

	// Load config and add repo to profile
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Ensure default profile exists
	if err := cfg.EnsureDefaultProfile(); err != nil {
		return fmt.Errorf("failed to ensure default profile: %w", err)
	}

	// Set active profile for DB operations
	db.SetActiveProfile(cfg.GetActiveProfileName())

	// Add repo to active profile
	profileName := cfg.GetActiveProfileName()
	if err := cfg.AddRepoToProfile(profileName, absPath); err != nil {
		VerboseLog("Warning: failed to add repo to profile: %v", err)
	} else {
		if err := cfg.Save(); err != nil {
			VerboseLog("Warning: failed to save config: %v", err)
		}
	}

	// Header
	fmt.Println()
	titleColor.Printf("  Ingesting Repository\n")
	dimColor.Printf("  %s\n", absPath)
	dimColor.Printf("  Profile: %s\n\n", profileName)

	// Phase 1: Git History (unless --index-only)
	if !ingestIndexOnly {
		if err := ingestGitHistory(absPath, cfg); err != nil {
			// If git fails, still try indexing (repo might not be git initialized)
			VerboseLog("Git ingest warning: %v", err)
			dimColor.Printf("  Note: Git ingestion skipped (%v)\n\n", err)
		}
	}

	// Phase 2: Codebase Indexing (unless --git-only)
	if !ingestGitOnly {
		if err := indexCodebase(absPath, cfg); err != nil {
			return fmt.Errorf("indexing failed: %w", err)
		}
	}

	// Final success message
	fmt.Println()
	successColor.Printf("  Ingestion Complete!\n\n")
	dimColor.Println("  Use 'devlog ask <question>' to query git history")
	dimColor.Println("  Use 'devlog search <query>' to search the codebase")
	fmt.Println()

	return nil
}

// BranchSelection holds the user's branch selection
type BranchSelection struct {
	MainBranch       string
	SelectedBranches []string
}

func ingestGitHistory(absPath string, cfg *config.Config) error {
	titleColor := color.New(color.FgHiCyan, color.Bold)
	successColor := color.New(color.FgHiGreen)
	dimColor := color.New(color.FgHiBlack)
	infoColor := color.New(color.FgHiWhite)
	titleColor.Printf("  Git History\n")
	// Open repository
	VerboseLog("Opening repository at %s", absPath)
	repo, err := git.OpenRepo(absPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}
	// Initialize database
	VerboseLog("Initializing database")
	database, err := db.GetDB()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	// Get or create codebase record
	codebase, err := db.GetCodebaseByPath(database, absPath)
	if err != nil {
		return fmt.Errorf("failed to get codebase: %w", err)
	}

	if codebase == nil {
		// Create new codebase
		defaultBranch, _ := repo.GetDefaultBranch()
		codebase = &db.Codebase{
			ID:            uuid.New().String(),
			Path:          absPath,
			Name:          filepath.Base(absPath),
			DefaultBranch: defaultBranch,
			IndexedAt:     time.Now(),
		}
		if err := db.UpsertCodebase(database, codebase); err != nil {
			return fmt.Errorf("failed to create codebase: %w", err)
		}
	}

	// Get user identifiers for marking user commits
	userEmail := cfg.UserEmail
	if userEmail == "" {
		userEmail, _ = repo.GetUserEmail()
	}
	githubUsername := cfg.GitHubUsername

	// Create/update developer record for current user
	if userEmail != "" {
		userName := cfg.UserName
		if userName == "" {
			userName, _ = repo.GetUserName()
		}
		dev := &db.Developer{
			ID:            userEmail,
			Name:          userName,
			Email:         userEmail,
			IsCurrentUser: true,
		}
		if err := db.UpsertDeveloper(database, dev); err != nil {
			VerboseLog("Warning: failed to upsert developer: %v", err)
		}
		db.SetCurrentUser(database, userEmail)
	}

	// List all branches
	allBranches, err := repo.ListBranches()
	if err != nil {
		return fmt.Errorf("failed to list branches: %w", err)
	}

	if len(allBranches) == 0 {
		dimColor.Println("  No branches found in repository")
		return nil
	}

	// Branch selection
	selection, err := selectBranches(allBranches, codebase.DefaultBranch)
	if err != nil {
		return fmt.Errorf("branch selection failed: %w", err)
	}

	// Update codebase default branch if changed
	if selection.MainBranch != codebase.DefaultBranch {
		codebase.DefaultBranch = selection.MainBranch
		db.UpsertCodebase(database, codebase)
	}

	// Determine the since date
	var sinceDate time.Time
	if ingestAll {
		dimColor.Println("  Ingesting full history...")
	} else if ingestSince != "" {
		sinceDate, err = time.Parse("2006-01-02", ingestSince)
		if err != nil {
			return fmt.Errorf("invalid date format (use YYYY-MM-DD): %w", err)
		}
		dimColor.Printf("  Since %s...\n", sinceDate.Format("Jan 2, 2006"))
	} else {
		sinceDate = time.Now().AddDate(0, 0, -ingestDays)
		dimColor.Printf("  Last %d days...\n", ingestDays)
	}

	// Track totals
	var totalCommits, totalFiles int

	// Create LLM client for commit summaries (if not skipped)
	var llmClient llm.LLMClient
	if !ingestSkipCommitSums && !ingestSkipSummaries {
		var err error
		llmClient, err = createLLMClient(cfg)
		if err != nil {
			dimColor.Printf("  Note: LLM not available, skipping commit summaries\n")
			VerboseLog("LLM error: %v", err)
		}
	}

	// Batch-fetch existing commit hashes for efficient deduplication
	existingHashes, err := db.GetExistingCommitHashes(database, codebase.ID)
	if err != nil {
		VerboseLog("Warning: failed to get existing hashes, will check individually: %v", err)
		existingHashes = make(map[string]bool)
	}
	VerboseLog("Found %d existing commits in database", len(existingHashes))

	// Create a map of selected branches for quick lookup
	selectedMap := make(map[string]bool)
	for _, b := range selection.SelectedBranches {
		selectedMap[b] = true
	}

	// Ingest main/default branch first
	fmt.Println()
	dimColor.Printf("  Scanning branches...\n")

	for _, branchInfo := range allBranches {
		if branchInfo.Name != selection.MainBranch {
			continue
		}

		// Mark as default
		branchInfo.IsDefault = true

		commits, files, err := ingestBranch(database, repo, codebase, branchInfo, "", sinceDate, userEmail, githubUsername, llmClient, existingHashes)
		if err != nil {
			VerboseLog("Warning: failed to ingest branch %s: %v", branchInfo.Name, err)
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

	// Ingest selected feature branches (only unique commits)
	for _, branchInfo := range allBranches {
		// Skip main branch (already processed) and non-selected branches
		if branchInfo.Name == selection.MainBranch {
			continue
		}
		if !selectedMap[branchInfo.Name] {
			continue
		}

		commits, files, err := ingestBranch(database, repo, codebase, branchInfo, selection.MainBranch, sinceDate, userEmail, githubUsername, llmClient, existingHashes)
		if err != nil {
			VerboseLog("Warning: failed to ingest branch %s: %v", branchInfo.Name, err)
			continue
		}
		totalCommits += commits
		totalFiles += files

		if commits > 0 {
			infoColor.Printf("    %s: %d commits\n", branchInfo.Name, commits)
		}
	}

	// Fill missing summaries if requested
	if ingestFillSummaries && llmClient != nil {
		fillCount, err := fillMissingSummaries(database, repo, codebase, llmClient)
		if err != nil {
			VerboseLog("Warning: failed to fill missing summaries: %v", err)
		} else if fillCount > 0 {
			successColor.Printf("  Generated %d missing commit summaries\n", fillCount)
		}
	}

	// Print summary
	fmt.Println()
	if totalCommits == 0 {
		dimColor.Println("  No new commits in time range")
	} else {
		successColor.Printf("  Ingested %d commits, %d file changes\n", totalCommits, totalFiles)
	}

	// Show totals
	commitCount, _ := db.GetCommitCount(database, codebase.ID)
	fileCount, _ := db.GetFileChangeCount(database, codebase.ID)
	infoColor.Printf("  Total: %d commits, %d file changes\n\n", commitCount, fileCount)

	return nil
}

func selectBranches(branches []git.BranchInfo, detectedDefault string) (*BranchSelection, error) {
	dimColor := color.New(color.FgHiBlack)

	// If branches specified via flag, use them
	if len(ingestBranches) > 0 {
		mainBranch := ingestBranches[0]
		return &BranchSelection{
			MainBranch:       mainBranch,
			SelectedBranches: ingestBranches,
		}, nil
	}

	// If --all-branches flag, select all
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

	// Interactive selection using Bubbletea TUI
	fmt.Println()
	selection, err := tui.RunBranchSelection(branches, detectedDefault)
	if err != nil {
		return nil, err
	}

	fmt.Println()
	dimColor.Printf("  Selected %d branch(es): %s\n", len(selection.SelectedBranches), strings.Join(selection.SelectedBranches, ", "))
	fmt.Println()

	return &BranchSelection{
		MainBranch:       selection.MainBranch,
		SelectedBranches: selection.SelectedBranches,
	}, nil
}

func ingestBranch(database *sql.DB, repo *git.Repository, codebase *db.Codebase, branchInfo git.BranchInfo, baseBranch string, sinceDate time.Time, userEmail string, githubUsername string, llmClient llm.LLMClient, existingHashes map[string]bool) (int, int, error) {
	// Get or create branch record
	branch, err := db.GetBranch(database, codebase.ID, branchInfo.Name)
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
		// Save branch first so commits can reference it
		if err := db.UpsertBranch(database, branch); err != nil {
			VerboseLog("Warning: failed to create branch %s: %v", branchInfo.Name, err)
		}
	}

	// Get cursor for incremental updates
	lastHash, _ := db.GetBranchCursor(database, codebase.ID, branchInfo.Name)

	// Get commits to process
	var commitHashes []string
	if isDefault || baseBranch == "" {
		// For default branch, get all commits
		commitHashes, err = repo.GetCommitsOnBranch(branchInfo.Name, "")
	} else {
		// For feature branches, only get unique commits
		commitHashes, err = repo.GetCommitsOnBranch(branchInfo.Name, baseBranch)
	}
	if err != nil {
		VerboseLog("Error getting commits for branch %s: %v", branchInfo.Name, err)
		return 0, 0, err
	}

	// Filter to only new commits (not in existingHashes and before cursor)
	var newCommitHashes []string
	for _, hash := range commitHashes {
		// Stop at last processed hash (cursor)
		if hash == lastHash {
			VerboseLog("Stopping at cursor hash: %s", lastHash)
			break
		}
		// Skip if already exists in database (batch check)
		if existingHashes[hash] {
			continue
		}
		newCommitHashes = append(newCommitHashes, hash)
	}

	VerboseLog("Branch %s: %d total commits, %d new to process", branchInfo.Name, len(commitHashes), len(newCommitHashes))

	// Track counts
	var commitCount, fileCount int
	var firstHash, latestHash string

	for _, hash := range newCommitHashes {
		// Get commit details
		gitCommit, err := repo.GetCommit(hash)
		if err != nil {
			VerboseLog("Error getting commit %s: %v", hash, err)
			continue
		}

		// Check date filter
		if !sinceDate.IsZero() && gitCommit.Author.When.Before(sinceDate) {
			VerboseLog("Skipping commit %s: before date filter (commit: %v, filter: %v)", hash[:8], gitCommit.Author.When, sinceDate)
			continue
		}

		// Track first/latest
		if latestHash == "" {
			latestHash = hash
		}
		firstHash = hash

		// Insert developer
		author := gitCommit.Author
		dev := &db.Developer{
			ID:    author.Email,
			Name:  author.Name,
			Email: author.Email,
		}
		db.UpsertDeveloper(database, dev)

		// Determine if this is a user commit
		// Match by email or by GitHub username extracted from noreply email
		isUserCommit := false
		if userEmail != "" && strings.EqualFold(author.Email, userEmail) {
			isUserCommit = true
		} else if isUserCommitByGitHub(author.Email, githubUsername) {
			isUserCommit = true
		}

		// Get commit stats
		stats, fileChanges := getCommitStats(repo, gitCommit)

		// Generate commit summary for user commits
		var commitSummary string
		if isUserCommit && llmClient != nil && len(fileChanges) > 0 {
			summary, err := generateCommitSummary(llmClient, gitCommit.Message, fileChanges)
			if err != nil {
				VerboseLog("Warning: failed to generate commit summary: %v", err)
			} else {
				commitSummary = summary
			}
		}

		// Create commit record
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

		if err := db.UpsertCommit(database, commit); err != nil {
			VerboseLog("Warning: failed to insert commit %s: %v", hash[:8], err)
			continue
		}

		// Mark as existing for subsequent branches
		existingHashes[hash] = true

		// Insert file changes
		for _, fc := range fileChanges {
			fc.CommitID = commit.ID
			if err := db.CreateFileChange(database, fc); err != nil {
				VerboseLog("Warning: failed to insert file change: %v", err)
			}
			fileCount++
		}

		commitCount++
	}

	// Update branch record
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
		db.UpsertBranch(database, branch)

		// Update cursor
		if latestHash != "" {
			db.UpdateBranchCursor(database, codebase.ID, branchInfo.Name, latestHash)
		}
	}

	return commitCount, fileCount, nil
}

func getCommitStats(repo *git.Repository, commit *git.Commit) (db.JSON, []*db.FileChange) {
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
		return stats, fileChanges
	}

	// Get trees
	parentTree, err := parent.Tree()
	if err != nil {
		return stats, fileChanges
	}

	commitTree, err := commit.Tree()
	if err != nil {
		return stats, fileChanges
	}

	// Calculate diff
	changes, err := parentTree.Diff(commitTree)
	if err != nil {
		return stats, fileChanges
	}

	var totalAdditions, totalDeletions int

	for _, change := range changes {
		fc := &db.FileChange{
			ID: uuid.New().String(),
		}

		// Determine change type and path
		action, err := change.Action()
		if err != nil {
			continue
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

		// Get patch for stats
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
			// Store patch if small enough
			if len(patchStr) < 10000 {
				fc.Patch = patchStr
			}
		}

		fileChanges = append(fileChanges, fc)
	}

	stats["additions"] = totalAdditions
	stats["deletions"] = totalDeletions
	stats["files_changed"] = len(fileChanges)

	return stats, fileChanges
}

func indexCodebase(absPath string, cfg *config.Config) error {
	titleColor := color.New(color.FgHiCyan, color.Bold)
	successColor := color.New(color.FgHiGreen)
	infoColor := color.New(color.FgHiWhite)
	dimColor := color.New(color.FgHiBlack)
	warnColor := color.New(color.FgHiYellow)

	titleColor.Printf("  Codebase Indexing\n")

	// Initialize database
	database, err := db.GetDB()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Scan codebase
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

	// Limit files if needed
	if ingestMaxFiles > 0 && len(scanResult.Files) > ingestMaxFiles {
		scanResult.Files = scanResult.Files[:ingestMaxFiles]
		warnColor.Printf("  Limited to %d files\n", ingestMaxFiles)
	}

	successColor.Printf("  Found %d files in %d folders\n", len(scanResult.Files), len(scanResult.Folders))

	// Detect tech stack
	techStack := indexer.DetectTechStack(scanResult.Files)
	if len(techStack) > 0 {
		var techs []string
		for tech := range techStack {
			techs = append(techs, tech)
		}
		dimColor.Printf("  Tech: %s\n", joinMax(techs, 5))
	}

	// Get or create codebase record
	codebase, _ := db.GetCodebaseByPath(database, absPath)
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

	// Setup LLM client for summaries
	var llmClient llm.LLMClient
	var summarizer *indexer.Summarizer

	if !ingestSkipSummaries {
		llmClient, err = createLLMClient(cfg)
		if err != nil {
			warnColor.Printf("  LLM not available, skipping summaries: %v\n", err)
			ingestSkipSummaries = true
		} else {
			summarizer = indexer.NewSummarizer(llmClient, IsVerbose())
		}
	}

	// Generate codebase summary (only on first index or force reindex)
	if !ingestSkipSummaries && summarizer != nil && (isFirstIndex || ingestForceReindex || codebase.Summary == "") {
		s.Suffix = " Generating codebase summary..."
		s.Start()
		ctx := context.Background()
		summary, err := summarizer.SummarizeCodebase(ctx, scanResult)
		if err == nil {
			codebase.Summary = summary
		}
		s.Stop()
		if codebase.Summary != "" {
			infoColor.Printf("  Summary: %s\n", truncate(codebase.Summary, 80))
		}
	}

	// Save codebase
	if err := db.UpsertCodebase(database, codebase); err != nil {
		return fmt.Errorf("failed to save codebase: %w", err)
	}

	// === INCREMENTAL INDEXING: Fetch existing data for comparison ===
	existingFiles, err := db.GetExistingFileHashes(database, codebase.ID)
	if err != nil {
		VerboseLog("Warning: couldn't fetch existing files, will re-index all: %v", err)
		existingFiles = make(map[string]db.ExistingFileInfo)
	}

	existingFolders, err := db.GetExistingFolderPaths(database, codebase.ID)
	if err != nil {
		VerboseLog("Warning: couldn't fetch existing folders: %v", err)
		existingFolders = make(map[string]string)
	}

	// Build set of current file/folder paths for deletion detection
	currentFilePaths := make(map[string]bool)
	for _, f := range scanResult.Files {
		currentFilePaths[f.Path] = true
	}
	currentFolderPaths := make(map[string]bool)
	for path := range scanResult.Folders {
		currentFolderPaths[path] = true
	}

	// Categorize files: new, changed, unchanged
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

	// Find deleted files
	var deletedFilePaths []string
	for path := range existingFiles {
		if !currentFilePaths[path] {
			deletedFilePaths = append(deletedFilePaths, path)
		}
	}

	// Find deleted folders
	var deletedFolderPaths []string
	for path := range existingFolders {
		if !currentFolderPaths[path] {
			deletedFolderPaths = append(deletedFolderPaths, path)
		}
	}

	// Report incremental stats
	if !isFirstIndex && !ingestForceReindex {
		dimColor.Printf("  Incremental: %d new, %d changed, %d unchanged, %d deleted\n",
			len(newFiles), len(changedFiles), len(unchangedFiles), len(deletedFilePaths))
	}

	// Delete removed files and folders
	if len(deletedFilePaths) > 0 {
		if err := db.DeleteFileIndexesByPaths(database, codebase.ID, deletedFilePaths); err != nil {
			VerboseLog("Warning: failed to delete removed files: %v", err)
		}
		VerboseLog("Deleted %d removed file indexes", len(deletedFilePaths))
	}
	if len(deletedFolderPaths) > 0 {
		if err := db.DeleteFoldersByPaths(database, codebase.ID, deletedFolderPaths); err != nil {
			VerboseLog("Warning: failed to delete removed folders: %v", err)
		}
		VerboseLog("Deleted %d removed folder indexes", len(deletedFolderPaths))
	}

	// Index folders (always update metadata, but only generate summaries for new/changed)
	fmt.Println()
	dimColor.Printf("  Indexing folders...")
	folderCount := 0
	folderIDMap := make(map[string]string)

	for folderPath, folderInfo := range scanResult.Folders {
		// Reuse existing folder ID if available
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

		// Only generate folder summary for new folders or on force reindex
		isNewFolder := existingFolders[folderPath] == ""
		if !ingestSkipSummaries && summarizer != nil && folderInfo.Depth <= 2 && len(folderInfo.Files) > 0 {
			if isNewFolder || ingestForceReindex {
				ctx := context.Background()
				summary, err := summarizer.SummarizeFolder(ctx, folderInfo)
				if err == nil {
					folder.Summary = summary.Summary
					folder.Purpose = summary.Purpose
				}
			}
		}

		if err := db.UpsertFolder(database, folder); err != nil {
			VerboseLog("Warning: failed to save folder %s: %v", folderPath, err)
		}

		folderCount++
		fmt.Printf("\r  Processed %d/%d folders", folderCount, len(scanResult.Folders))
	}
	fmt.Println()

	// Index files (only new and changed files need summaries)
	filesToProcess := append(newFiles, changedFiles...)
	dimColor.Printf("  Indexing files...")
	fileCount := 0
	summarizedCount := 0
	totalFiles := len(filesToProcess) + len(unchangedFiles)

	// Process new and changed files (need summaries)
	for _, fileInfo := range filesToProcess {
		// Check if we had an existing ID to reuse
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

		// Generate file summary for new/changed files
		if !ingestSkipSummaries && summarizer != nil && shouldSummarizeFile(fileInfo) {
			ctx := context.Background()
			summary, err := summarizer.SummarizeFile(ctx, fileInfo)
			if err == nil {
				file.Summary = summary.Summary
				file.Purpose = summary.Purpose
				file.KeyExports = summary.KeyExports
				summarizedCount++
			}
		}

		if err := db.UpsertFileIndex(database, file); err != nil {
			VerboseLog("Warning: failed to save file %s: %v", fileInfo.Path, err)
		}

		fileCount++
		if fileCount%10 == 0 || fileCount == len(filesToProcess) {
			fmt.Printf("\r  Processed %d/%d files (summarizing)", fileCount, len(filesToProcess))
		}
	}

	// Update unchanged files (just update metadata, keep existing summary)
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

		if err := db.UpsertFileIndex(database, file); err != nil {
			VerboseLog("Warning: failed to save file %s: %v", fileInfo.Path, err)
		}
	}

	fmt.Printf("\r  Processed %d/%d files                    \n", totalFiles, totalFiles)

	// Show stats
	fmt.Println()
	stats, _ := db.GetCodebaseStats(database, codebase.ID)
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

func createLLMClient(cfg *config.Config) (llm.LLMClient, error) {
	provider := cfg.DefaultProvider
	if provider == "" {
		provider = "ollama"
	}

	llmCfg := llm.Config{
		Provider: llm.Provider(provider),
	}

	switch llmCfg.Provider {
	case llm.ProviderOpenAI:
		llmCfg.APIKey = cfg.GetAPIKey("openai")
	case llm.ProviderAnthropic:
		llmCfg.APIKey = cfg.GetAPIKey("anthropic")
	case llm.ProviderBedrock:
		llmCfg.AWSAccessKeyID = cfg.AWSAccessKeyID
		llmCfg.AWSSecretAccessKey = cfg.AWSSecretAccessKey
		llmCfg.AWSRegion = cfg.AWSRegion
	case llm.ProviderOllama:
		if cfg.OllamaBaseURL != "" {
			llmCfg.BaseURL = cfg.OllamaBaseURL
		}
		if cfg.OllamaModel != "" {
			llmCfg.Model = cfg.OllamaModel
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

// generateCommitSummary generates a meaningful summary for a commit using file changes
func generateCommitSummary(client llm.LLMClient, commitMessage string, fileChanges []*db.FileChange) (string, error) {
	// Build context from file changes
	var sb strings.Builder
	sb.WriteString("Commit message: ")
	sb.WriteString(commitMessage)
	sb.WriteString("\n\nFiles changed:\n")

	totalAdditions := 0
	totalDeletions := 0

	for i, fc := range fileChanges {
		if i >= 20 {
			sb.WriteString(fmt.Sprintf("... and %d more files\n", len(fileChanges)-20))
			break
		}

		sb.WriteString(fmt.Sprintf("- %s (%s): +%d/-%d\n", fc.FilePath, fc.ChangeType, fc.Additions, fc.Deletions))
		totalAdditions += fc.Additions
		totalDeletions += fc.Deletions

		// Include patch snippet for context (first 500 chars)
		if fc.Patch != "" && len(fc.Patch) > 0 {
			patchPreview := fc.Patch
			if len(patchPreview) > 500 {
				patchPreview = patchPreview[:500] + "..."
			}
			// Only include actual code changes, not headers
			lines := strings.Split(patchPreview, "\n")
			var codeLines []string
			for _, line := range lines {
				if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
					if !strings.HasPrefix(line, "+++") && !strings.HasPrefix(line, "---") {
						codeLines = append(codeLines, line)
					}
				}
			}
			if len(codeLines) > 0 {
				sb.WriteString("  Changes:\n")
				for j, line := range codeLines {
					if j >= 10 {
						break
					}
					sb.WriteString(fmt.Sprintf("    %s\n", line))
				}
			}
		}
	}

	sb.WriteString(fmt.Sprintf("\nTotal: +%d/-%d lines across %d files\n", totalAdditions, totalDeletions, len(fileChanges)))

	prompt := fmt.Sprintf(`Analyze this git commit and write a clear, technical summary of what was accomplished.
Focus on the WHAT and WHY, not just listing files. Be specific about functionality added/changed.
Keep it to 1-2 sentences, max 100 words. Be professional and technical.

IMPORTANT STYLE RULES:
- Do NOT start with "This commit" or "The commit" - start directly with the action (e.g., "Added...", "Improved...", "Fixed...")
- Do NOT include any preamble like "Here is a summary"
- Write in active voice, past tense (e.g., "Added error handling" not "This commit adds error handling")
- Be concise and direct

%s`, sb.String())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return client.Complete(ctx, prompt)
}

// fillMissingSummaries generates summaries for user commits that don't have them
func fillMissingSummaries(database *sql.DB, repo *git.Repository, codebase *db.Codebase, llmClient llm.LLMClient) (int, error) {
	dimColor := color.New(color.FgHiBlack)

	// Get user commits missing summaries
	commits, err := db.GetUserCommitsMissingSummaries(database, codebase.ID)
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
		// Get file changes for this commit
		fileChanges, err := db.GetFileChangesByCommit(database, commit.ID)
		if err != nil {
			VerboseLog("Warning: failed to get file changes for commit %s: %v", commit.Hash[:8], err)
			continue
		}

		if len(fileChanges) == 0 {
			VerboseLog("Skipping commit %s: no file changes", commit.Hash[:8])
			continue
		}

		// Convert to pointer slice for generateCommitSummary
		var fcPtrs []*db.FileChange
		for j := range fileChanges {
			fcPtrs = append(fcPtrs, &fileChanges[j])
		}

		// Generate summary
		summary, err := generateCommitSummary(llmClient, commit.Message, fcPtrs)
		if err != nil {
			VerboseLog("Warning: failed to generate summary for commit %s: %v", commit.Hash[:8], err)
			continue
		}

		// Update commit with summary
		if err := db.UpdateCommitSummary(database, commit.ID, summary); err != nil {
			VerboseLog("Warning: failed to update summary for commit %s: %v", commit.Hash[:8], err)
			continue
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
