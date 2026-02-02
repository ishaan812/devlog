package cli

import (
	"context"
	"fmt"
	"path/filepath"
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
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

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
	ingestGitOnly   bool
	ingestIndexOnly bool
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

	// Get user email for marking user commits
	userEmail := cfg.UserEmail
	if userEmail == "" {
		userEmail, _ = repo.GetUserEmail()
	}

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

		commits, files, err := ingestBranch(database, repo, codebase, branchInfo, "", sinceDate, userEmail)
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

		commits, files, err := ingestBranch(database, repo, codebase, branchInfo, selection.MainBranch, sinceDate, userEmail)
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
	titleColor := color.New(color.FgHiCyan)
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

	// Interactive selection
	fmt.Println()
	titleColor.Println("  Branch Selection")
	dimColor.Println("  " + strings.Repeat("─", 40))
	fmt.Println()

	// Build branch name list
	var branchNames []string
	for _, b := range branches {
		branchNames = append(branchNames, b.Name)
	}

	// Step 1: Select main/default branch
	dimColor.Println("  Select the main branch (commits from other branches will be")
	dimColor.Println("  compared against this to avoid duplicates):")
	fmt.Println()

	// Find default index
	defaultIndex := 0
	for i, name := range branchNames {
		if name == detectedDefault {
			defaultIndex = i
			break
		}
	}

	mainPrompt := promptui.Select{
		Label: "Main Branch",
		Items: branchNames,
		Size:  10,
		Templates: &promptui.SelectTemplates{
			Label:    "{{ . }}",
			Active:   "▸ {{ . | cyan }}",
			Inactive: "  {{ . }}",
			Selected: "  ✓ Main branch: {{ . | green }}",
		},
		CursorPos: defaultIndex,
	}

	_, mainBranch, err := mainPrompt.Run()
	if err != nil {
		return nil, fmt.Errorf("main branch selection cancelled: %w", err)
	}

	// Step 2: Select additional branches to ingest
	fmt.Println()
	dimColor.Println("  Select additional branches to ingest (space to select, enter to confirm):")
	dimColor.Println("  Only commits unique to each branch will be ingested.")
	fmt.Println()

	// For multi-select, we'll use a different approach
	// Create a list of checkable items
	selectedBranches := []string{mainBranch}

	// Filter out main branch from selection
	var otherBranches []string
	for _, name := range branchNames {
		if name != mainBranch {
			otherBranches = append(otherBranches, name)
		}
	}

	if len(otherBranches) > 0 {
		// Ask if user wants to select additional branches
		confirmPrompt := promptui.Select{
			Label: "Ingest additional branches?",
			Items: []string{"Yes, let me select", "No, only main branch", "Yes, all branches"},
			Templates: &promptui.SelectTemplates{
				Label:    "{{ . }}",
				Active:   "▸ {{ . | cyan }}",
				Inactive: "  {{ . }}",
				Selected: "  {{ . | green }}",
			},
		}

		idx, _, err := confirmPrompt.Run()
		if err != nil {
			return nil, fmt.Errorf("selection cancelled: %w", err)
		}

		switch idx {
		case 0: // Yes, let me select
			selectedBranches = append(selectedBranches, selectMultipleBranches(otherBranches)...)
		case 1: // No, only main branch
			// Keep only main branch
		case 2: // Yes, all branches
			selectedBranches = append(selectedBranches, otherBranches...)
		}
	}

	fmt.Println()
	dimColor.Printf("  Selected %d branch(es): %s\n", len(selectedBranches), strings.Join(selectedBranches, ", "))
	fmt.Println()

	return &BranchSelection{
		MainBranch:       mainBranch,
		SelectedBranches: selectedBranches,
	}, nil
}

func selectMultipleBranches(branches []string) []string {
	if len(branches) == 0 {
		return nil
	}

	var selected []string

	// Use individual prompts for each branch
	for _, branch := range branches {
		prompt := promptui.Select{
			Label: fmt.Sprintf("Include '%s'?", branch),
			Items: []string{"Yes", "No"},
			Templates: &promptui.SelectTemplates{
				Label:    "{{ . }}",
				Active:   "▸ {{ . | cyan }}",
				Inactive: "  {{ . }}",
				Selected: "",
			},
			HideSelected: true,
		}

		idx, _, err := prompt.Run()
		if err != nil {
			continue
		}

		if idx == 0 { // Yes
			selected = append(selected, branch)
			fmt.Printf("  ✓ %s\n", branch)
		}
	}

	return selected
}

func ingestBranch(database *gorm.DB, repo *git.Repository, codebase *db.Codebase, branchInfo git.BranchInfo, baseBranch string, sinceDate time.Time, userEmail string) (int, int, error) {
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
		return 0, 0, err
	}

	// Track counts
	var commitCount, fileCount int
	var firstHash, latestHash string

	for _, hash := range commitHashes {
		// Stop at last processed hash
		if hash == lastHash {
			break
		}

		// Get commit details
		gitCommit, err := repo.GetCommit(hash)
		if err != nil {
			continue
		}

		// Check date filter
		if !sinceDate.IsZero() && gitCommit.Author.When.Before(sinceDate) {
			continue
		}

		// Track first/latest
		if latestHash == "" {
			latestHash = hash
		}
		firstHash = hash

		// Check if commit already exists
		exists, _ := db.CommitExists(database, codebase.ID, hash)
		if exists {
			continue
		}

		// Insert developer
		author := gitCommit.Author
		dev := &db.Developer{
			ID:    author.Email,
			Name:  author.Name,
			Email: author.Email,
		}
		db.UpsertDeveloper(database, dev)

		// Determine if this is a user commit
		isUserCommit := userEmail != "" && strings.EqualFold(author.Email, userEmail)

		// Get commit stats
		stats, fileChanges := getCommitStats(repo, gitCommit)

		// Create commit record
		commit := &db.Commit{
			ID:                uuid.New().String(),
			Hash:              hash,
			CodebaseID:        codebase.ID,
			BranchID:          branch.ID,
			AuthorEmail:       author.Email,
			Message:           strings.TrimSpace(gitCommit.Message),
			CommittedAt:       author.When,
			Stats:             stats,
			IsUserCommit:      isUserCommit,
			IsOnDefaultBranch: isDefault,
		}

		if err := db.UpsertCommit(database, commit); err != nil {
			VerboseLog("Warning: failed to insert commit %s: %v", hash[:8], err)
			continue
		}

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

	// Generate codebase summary
	if !ingestSkipSummaries && summarizer != nil {
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

	// Index folders
	fmt.Println()
	dimColor.Printf("  Indexing folders...")
	folderCount := 0
	folderIDMap := make(map[string]string)

	for folderPath, folderInfo := range scanResult.Folders {
		folderID := uuid.New().String()
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

		// Generate folder summary for shallow folders
		if !ingestSkipSummaries && summarizer != nil && folderInfo.Depth <= 2 && len(folderInfo.Files) > 0 {
			ctx := context.Background()
			summary, err := summarizer.SummarizeFolder(ctx, folderInfo)
			if err == nil {
				folder.Summary = summary.Summary
				folder.Purpose = summary.Purpose
			}
		}

		if err := db.UpsertFolder(database, folder); err != nil {
			VerboseLog("Warning: failed to save folder %s: %v", folderPath, err)
		}

		folderCount++
		fmt.Printf("\r  Processed %d/%d folders", folderCount, len(scanResult.Folders))
	}
	fmt.Println()

	// Index files
	dimColor.Printf("  Indexing files...")
	fileCount := 0
	summarizedCount := 0

	for _, fileInfo := range scanResult.Files {
		fileID := uuid.New().String()

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

		// Generate file summary
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
		if fileCount%10 == 0 || fileCount == len(scanResult.Files) {
			fmt.Printf("\r  Processed %d/%d files", fileCount, len(scanResult.Files))
		}
	}
	fmt.Println()

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
		infoColor.Printf("%d files\n", summarizedCount)
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
