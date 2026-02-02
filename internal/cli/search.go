package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/db"
	"github.com/ishaan812/devlog/internal/llm"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

var (
	searchCodebase  string
	searchLimit     int
	searchKeyword   bool
	searchLanguage  string
	searchExtension string
	searchPath      string
	searchType      string
	searchDays      int
	searchBranch    string
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search the indexed codebase and commit history",
	Long: `Search through your indexed codebase and commit history.

By default, searches file summaries and content. Use --type to search
different data types.

Search Types:
  files    - Search indexed files (default)
  commits  - Search commit messages
  all      - Search both files and commits

Filtering Options:
  --lang       Filter files by programming language (e.g., go, python, javascript)
  --ext        Filter files by extension (e.g., .go, .py, .ts)
  --path       Filter by path pattern (e.g., "internal/", "src/")
  --branch     Filter commits by branch name
  --days       Filter commits to last N days

Examples:
  devlog search "authentication logic"
  devlog search "database connection" --lang go
  devlog search "API endpoint" --path "internal/api/"
  devlog search --type commits "fix bug"
  devlog search --type commits "refactor" --days 7
  devlog search --type all "login"`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSearch,
}

func init() {
	rootCmd.AddCommand(searchCmd)

	searchCmd.Flags().StringVar(&searchCodebase, "codebase", "", "Codebase path (default: current directory)")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 10, "Maximum results to show")
	searchCmd.Flags().BoolVar(&searchKeyword, "keyword", false, "Use keyword search instead of semantic search")
	searchCmd.Flags().StringVar(&searchLanguage, "lang", "", "Filter by programming language")
	searchCmd.Flags().StringVar(&searchExtension, "ext", "", "Filter by file extension")
	searchCmd.Flags().StringVar(&searchPath, "path", "", "Filter by path pattern")
	searchCmd.Flags().StringVarP(&searchType, "type", "t", "files", "Search type: files, commits, all")
	searchCmd.Flags().IntVar(&searchDays, "days", 0, "Filter commits to last N days")
	searchCmd.Flags().StringVar(&searchBranch, "branch", "", "Filter commits by branch name")
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")

	// Colors
	titleColor := color.New(color.FgHiCyan, color.Bold)
	pathColor := color.New(color.FgHiYellow)
	summaryColor := color.New(color.FgHiWhite)
	dimColor := color.New(color.FgHiBlack)
	purposeColor := color.New(color.FgHiMagenta)
	semanticColor := color.New(color.FgHiGreen)
	commitColor := color.New(color.FgHiBlue)

	database, err := db.GetDB()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Get codebase
	codebasePath := searchCodebase
	if codebasePath == "" {
		codebasePath, _ = filepath.Abs(".")
	}

	codebase, err := db.GetCodebaseByPath(database, codebasePath)
	if err != nil {
		return err
	}
	if codebase == nil {
		return fmt.Errorf("codebase not indexed. Run 'devlog ingest' first")
	}

	// Display header
	fmt.Println()
	titleColor.Printf("  Search Results for: %s\n", query)

	// Show active filters
	var filters []string
	if searchLanguage != "" {
		filters = append(filters, fmt.Sprintf("lang:%s", searchLanguage))
	}
	if searchExtension != "" {
		filters = append(filters, fmt.Sprintf("ext:%s", searchExtension))
	}
	if searchPath != "" {
		filters = append(filters, fmt.Sprintf("path:%s", searchPath))
	}
	if searchBranch != "" {
		filters = append(filters, fmt.Sprintf("branch:%s", searchBranch))
	}
	if searchDays > 0 {
		filters = append(filters, fmt.Sprintf("days:%d", searchDays))
	}
	if len(filters) > 0 {
		dimColor.Printf("  Filters: %s\n", strings.Join(filters, " "))
	}
	dimColor.Println("  " + strings.Repeat("─", 50))
	fmt.Println()

	// Search based on type
	switch searchType {
	case "files":
		return searchFiles(database, codebase, query, titleColor, pathColor, summaryColor, dimColor, purposeColor, semanticColor)
	case "commits":
		return searchCommits(database, codebase, query, titleColor, commitColor, dimColor)
	case "all":
		err := searchFiles(database, codebase, query, titleColor, pathColor, summaryColor, dimColor, purposeColor, semanticColor)
		if err != nil {
			VerboseLog("File search error: %v", err)
		}
		return searchCommits(database, codebase, query, titleColor, commitColor, dimColor)
	default:
		return fmt.Errorf("unknown search type: %s (use files, commits, or all)", searchType)
	}
}

func searchFiles(database *gorm.DB, codebase *db.Codebase, query string,
	titleColor, pathColor, summaryColor, dimColor, purposeColor, semanticColor *color.Color) error {

	var files []db.FileIndex
	var matchingFolders []db.Folder
	useSemanticSearch := !searchKeyword && db.HasEmbeddings(database, codebase.ID)

	if useSemanticSearch {
		// Try semantic search with embeddings
		cfg, _ := config.Load()
		embedder, err := createEmbedder(cfg)
		if err != nil {
			VerboseLog("Embedder not available, falling back to keyword search: %v", err)
			useSemanticSearch = false
		} else {
			ctx := context.Background()
			queryEmbedding, err := embedder.Embed(ctx, query)
			if err != nil {
				VerboseLog("Failed to embed query, falling back to keyword search: %v", err)
				useSemanticSearch = false
			} else {
				// Semantic search for files
				files, err = db.SemanticSearchFiles(database, codebase.ID, queryEmbedding, searchLimit)
				if err != nil {
					VerboseLog("Semantic file search failed: %v", err)
				}

				// Semantic search for folders
				matchingFolders, err = db.SemanticSearchFolders(database, codebase.ID, queryEmbedding, 5)
				if err != nil {
					VerboseLog("Semantic folder search failed: %v", err)
				}
			}
		}
	}

	// Fall back to keyword search if semantic search didn't work or wasn't used
	if !useSemanticSearch {
		// Keyword search for files with filters
		files, _ = searchFilesWithFilters(database, codebase.ID, query)

		// Keyword search for folders
		folders, _ := db.GetFoldersByCodebase(database, codebase.ID)
		queryLower := strings.ToLower(query)
		for _, f := range folders {
			if strings.Contains(strings.ToLower(f.Summary), queryLower) ||
				strings.Contains(strings.ToLower(f.Purpose), queryLower) ||
				strings.Contains(strings.ToLower(f.Name), queryLower) {
				matchingFolders = append(matchingFolders, f)
			}
		}
	}

	// Apply post-filters for semantic search results
	if useSemanticSearch {
		files = applyFileFilters(files)
	}

	// Display search mode
	if useSemanticSearch {
		semanticColor.Println("  Using semantic search (vector similarity)")
	} else {
		dimColor.Println("  Using keyword search")
	}
	fmt.Println()

	if len(matchingFolders) > 0 {
		titleColor.Println("  Folders")
		fmt.Println()
		for i, f := range matchingFolders {
			if i >= 5 {
				dimColor.Printf("  ... and %d more folders\n", len(matchingFolders)-5)
				break
			}
			pathColor.Printf("  %s\n", f.Path)
			if f.Purpose != "" {
				purposeColor.Printf("    [%s] ", f.Purpose)
			}
			if f.Summary != "" {
				summaryColor.Printf("%s\n", truncate(f.Summary, 60))
			} else {
				fmt.Println()
			}
		}
		fmt.Println()
	}

	if len(files) > 0 {
		titleColor.Println("  Files")
		fmt.Println()
		for i, f := range files {
			if i >= searchLimit {
				dimColor.Printf("  ... and %d more files\n", len(files)-searchLimit)
				break
			}

			icon := getFileIcon(f.Extension, f.Language)
			pathColor.Printf("  %s %s\n", icon, f.Path)

			if f.Purpose != "" {
				purposeColor.Printf("    [%s] ", f.Purpose)
			}
			if f.Summary != "" {
				summaryColor.Printf("%s\n", truncate(f.Summary, 60))
			} else {
				fmt.Println()
			}

			if f.Language != "" {
				dimColor.Printf("    %s | %d lines\n", f.Language, f.LineCount)
			}
			fmt.Println()
		}
	}

	if len(files) == 0 && len(matchingFolders) == 0 {
		dimColor.Println("  No files found.")
		fmt.Println()
	}

	return nil
}

func searchCommits(database *gorm.DB, codebase *db.Codebase, query string,
	titleColor, commitColor, dimColor *color.Color) error {

	titleColor.Println("  Commits")
	fmt.Println()

	// Build query conditions
	var commits []db.Commit
	tx := database.Model(&db.Commit{}).Where("codebase_id = ?", codebase.ID)

	// Apply query filter
	if query != "" {
		queryPattern := "%" + query + "%"
		tx = tx.Where("message LIKE ? OR summary LIKE ?", queryPattern, queryPattern)
	}

	// Apply branch filter
	if searchBranch != "" {
		branch, err := db.GetBranch(database, codebase.ID, searchBranch)
		if err != nil || branch == nil {
			dimColor.Printf("  Branch '%s' not found\n", searchBranch)
			return nil
		}
		tx = tx.Where("branch_id = ?", branch.ID)
	}

	// Apply date filter
	if searchDays > 0 {
		since := time.Now().AddDate(0, 0, -searchDays)
		tx = tx.Where("committed_at >= ?", since)
	}

	// Execute query
	err := tx.Order("committed_at DESC").Limit(searchLimit).Find(&commits).Error
	if err != nil {
		return fmt.Errorf("failed to search commits: %w", err)
	}

	if len(commits) == 0 {
		dimColor.Println("  No commits found matching the query.")
		fmt.Println()
		return nil
	}

	for _, c := range commits {
		// Get first line of commit message
		msg := strings.Split(c.Message, "\n")[0]
		if len(msg) > 70 {
			msg = msg[:67] + "..."
		}

		// Commit date and hash
		dateStr := c.CommittedAt.Format("Jan 2")
		commitColor.Printf("  %s ", c.Hash[:7])
		dimColor.Printf("%s ", dateStr)
		fmt.Printf("%s\n", msg)

		// Show summary if available
		if c.Summary != "" && c.Summary != msg {
			dimColor.Printf("    %s\n", truncate(c.Summary, 60))
		}
	}

	fmt.Println()
	return nil
}

func searchFilesWithFilters(database *gorm.DB, codebaseID, query string) ([]db.FileIndex, error) {
	var files []db.FileIndex
	tx := database.Model(&db.FileIndex{}).Where("codebase_id = ?", codebaseID)

	// Apply query filter
	if query != "" {
		searchPattern := "%" + query + "%"
		tx = tx.Where("(summary LIKE ? OR purpose LIKE ? OR name LIKE ?)",
			searchPattern, searchPattern, searchPattern)
	}

	// Apply language filter
	if searchLanguage != "" {
		tx = tx.Where("LOWER(language) = LOWER(?)", searchLanguage)
	}

	// Apply extension filter
	if searchExtension != "" {
		ext := searchExtension
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		tx = tx.Where("extension = ?", ext)
	}

	// Apply path filter
	if searchPath != "" {
		pathPattern := "%" + searchPath + "%"
		tx = tx.Where("path LIKE ?", pathPattern)
	}

	err := tx.Order("path").Limit(searchLimit * 2).Find(&files).Error
	return files, err
}

func applyFileFilters(files []db.FileIndex) []db.FileIndex {
	if searchLanguage == "" && searchExtension == "" && searchPath == "" {
		return files
	}

	var filtered []db.FileIndex
	for _, f := range files {
		// Language filter
		if searchLanguage != "" && !strings.EqualFold(f.Language, searchLanguage) {
			continue
		}

		// Extension filter
		if searchExtension != "" {
			ext := searchExtension
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			if f.Extension != ext {
				continue
			}
		}

		// Path filter
		if searchPath != "" && !strings.Contains(f.Path, searchPath) {
			continue
		}

		filtered = append(filtered, f)
	}

	return filtered
}

func createEmbedder(cfg *config.Config) (llm.EmbeddingClient, error) {
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
