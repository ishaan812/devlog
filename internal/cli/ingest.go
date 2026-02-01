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
	"github.com/spf13/cobra"
)

var (
	// Git history flags
	ingestDays  int
	ingestAll   bool
	ingestSince string

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

By default, only commits from the last 30 days are ingested. Use --all to
ingest the entire history, or --days to specify a custom time range.

Examples:
  devlog ingest                       # Full ingest (git + index)
  devlog ingest ~/projects/myapp      # Ingest specific path
  devlog ingest --days 90             # Last 90 days of git history
  devlog ingest --all                 # Full git history
  devlog ingest --git-only            # Only git history, skip indexing
  devlog ingest --index-only          # Only indexing, skip git history
  devlog ingest --skip-summaries      # Skip LLM summaries (faster)
  devlog ingest --max-files 100       # Limit files to index`,
	Args: cobra.MaximumNArgs(1),
	RunE: runIngest,
}

func init() {
	rootCmd.AddCommand(ingestCmd)

	// Git history flags
	ingestCmd.Flags().IntVar(&ingestDays, "days", 30, "Number of days of history to ingest")
	ingestCmd.Flags().BoolVar(&ingestAll, "all", false, "Ingest full git history (ignores --days)")
	ingestCmd.Flags().StringVar(&ingestSince, "since", "", "Ingest commits since date (YYYY-MM-DD)")

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
		if err := ingestGitHistory(absPath); err != nil {
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

func ingestGitHistory(absPath string) error {
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

	// Get last scanned hash for incremental updates
	repoPath := repo.Path()
	lastHash, err := db.GetLastScannedHash(database, repoPath)
	if err != nil {
		return fmt.Errorf("failed to get last scanned hash: %w", err)
	}

	// Get current HEAD
	headHash, err := repo.HeadHash()
	if err != nil {
		return fmt.Errorf("failed to get HEAD hash: %w", err)
	}

	if lastHash != "" {
		VerboseLog("Last ingested commit: %s", lastHash[:8])
		VerboseLog("Current HEAD: %s", headHash[:8])
		if headHash == lastHash {
			dimColor.Println("  No new commits to ingest")
			totalCommits, _ := db.GetCommitCount(database, repoPath)
			totalFiles, _ := db.GetFileChangeCount(database, repoPath)
			dimColor.Printf("  Total: %d commits, %d file changes\n\n", totalCommits, totalFiles)
			return nil
		}
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

	// Track counts
	var commitCount, fileCount int

	// Walk commits
	opts := git.WalkOptions{
		StopAtHash: lastHash,
		Since:      sinceDate,
		Verbose:    IsVerbose(),
		OnProgress: func(processed, total int) {
			if processed%10 == 0 || processed == total {
				fmt.Printf("\r  Processed %d commits...", processed)
			}
		},
		OnCommit: func(info git.CommitInfo) error {
			// Insert developer
			err := db.InsertDeveloper(database, db.Developer{
				ID:    info.AuthorEmail,
				Name:  info.AuthorName,
				Email: info.AuthorEmail,
			})
			if err != nil {
				VerboseLog("Warning: failed to insert developer: %v", err)
			}

			// Insert commit
			err = db.InsertCommit(database, db.Commit{
				Hash:        info.Hash,
				RepoPath:    repoPath,
				Message:     strings.TrimSpace(info.Message),
				AuthorEmail: info.AuthorEmail,
				CommittedAt: info.CommittedAt,
				Stats: map[string]interface{}{
					"additions":     info.Stats.TotalAdditions,
					"deletions":     info.Stats.TotalDeletions,
					"files_changed": info.Stats.FilesChanged,
				},
			})
			if err != nil {
				VerboseLog("Warning: failed to insert commit %s: %v", info.Hash[:8], err)
			}

			// Insert file changes
			for _, fc := range info.FileChanges {
				err := db.InsertFileChange(database, db.FileChange{
					ID:         fc.ID,
					CommitHash: info.Hash,
					FilePath:   fc.FilePath,
					ChangeType: fc.ChangeType,
					Additions:  fc.Additions,
					Deletions:  fc.Deletions,
					Patch:      fc.Patch,
				})
				if err != nil {
					VerboseLog("Warning: failed to insert file change: %v", err)
				}
				fileCount++
			}

			commitCount++
			return nil
		},
	}

	_, err = git.WalkCommits(repo, opts)
	if err != nil {
		return fmt.Errorf("failed to walk commits: %w", err)
	}

	fmt.Println() // Clear progress line

	// Update cursor
	if commitCount > 0 {
		if err := db.UpdateCursor(database, repoPath, headHash); err != nil {
			VerboseLog("Warning: failed to update cursor: %v", err)
		}
	}

	// Print summary
	if commitCount == 0 {
		dimColor.Println("  No new commits in time range")
	} else {
		successColor.Printf("  Ingested %d commits, %d file changes\n", commitCount, fileCount)
	}

	// Show totals
	totalCommits, _ := db.GetCommitCount(database, repoPath)
	totalFiles, _ := db.GetFileChangeCount(database, repoPath)
	infoColor.Printf("  Total: %d commits, %d file changes\n\n", totalCommits, totalFiles)

	return nil
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

	// Create codebase record
	codebaseID := uuid.New().String()
	codebase := db.Codebase{
		ID:        codebaseID,
		Path:      absPath,
		Name:      scanResult.Name,
		IndexedAt: time.Now(),
		TechStack: techStack,
	}

	// Setup LLM client for summaries
	var llmClient llm.LLMClient
	var summarizer *indexer.Summarizer
	var embedder llm.EmbeddingClient

	if !ingestSkipSummaries {
		llmClient, err = createLLMClient(cfg)
		if err != nil {
			warnColor.Printf("  LLM not available, skipping summaries: %v\n", err)
			ingestSkipSummaries = true
		} else {
			summarizer = indexer.NewSummarizer(llmClient, IsVerbose())
		}
	}

	// Setup embedder
	if !ingestSkipEmbeddings {
		embedder, err = createEmbedderForIndex(cfg)
		if err != nil {
			warnColor.Printf("  Embedder not available, skipping embeddings: %v\n", err)
			ingestSkipEmbeddings = true
		} else {
			dimColor.Println("  Embeddings enabled")
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
	if err := db.InsertCodebase(database, codebase); err != nil {
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

		folder := db.Folder{
			ID:         folderID,
			CodebaseID: codebaseID,
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

				// Generate embedding
				if !ingestSkipEmbeddings && embedder != nil && folder.Summary != "" {
					embeddingText := folder.Summary
					if folder.Purpose != "" {
						embeddingText = folder.Purpose + ": " + embeddingText
					}
					embedding, err := embedder.Embed(ctx, embeddingText)
					if err == nil {
						folder.Embedding = embedding
					}
				}
			}
		}

		if err := db.InsertFolder(database, folder); err != nil {
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

		file := db.FileIndex{
			ID:          fileID,
			CodebaseID:  codebaseID,
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

				// Generate embedding
				if !ingestSkipEmbeddings && embedder != nil && file.Summary != "" {
					embeddingText := file.Summary
					if file.Purpose != "" {
						embeddingText = file.Purpose + ": " + embeddingText
					}
					embedding, err := embedder.Embed(ctx, embeddingText)
					if err == nil {
						file.Embedding = embedding
					}
				}
			}
		}

		if err := db.InsertFileIndex(database, file); err != nil {
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
	stats, _ := db.GetCodebaseStats(database, codebaseID)
	dimColor.Printf("  Folders:    ")
	infoColor.Printf("%d\n", stats["folder_count"])
	dimColor.Printf("  Files:      ")
	infoColor.Printf("%d\n", stats["file_count"])
	dimColor.Printf("  Total size: ")
	infoColor.Printf("%s\n", formatBytes(stats["total_size_bytes"].(int64)))
	dimColor.Printf("  Lines:      ")
	infoColor.Printf("%d\n", stats["total_lines"])

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

func createEmbedderForIndex(cfg *config.Config) (llm.EmbeddingClient, error) {
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
	case llm.ProviderOllama:
		if cfg.OllamaBaseURL != "" {
			llmCfg.BaseURL = cfg.OllamaBaseURL
		}
	}

	return llm.NewEmbedder(llmCfg)
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
