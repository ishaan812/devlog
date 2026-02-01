package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/db"
	"github.com/ishaan812/devlog/internal/llm"
	"github.com/spf13/cobra"
)

var (
	searchCodebase  string
	searchLimit     int
	searchKeyword   bool
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search the indexed codebase using semantic search",
	Long: `Search through your indexed codebase using natural language queries.

By default, uses semantic search with vector embeddings to find relevant
code based on meaning, not just keywords. Falls back to keyword search
if embeddings aren't available.

For semantic search to work, ingest your codebase without --skip-embeddings:
  devlog ingest

Examples:
  devlog search "authentication logic"
  devlog search "database connection handling"
  devlog search "API request validation"
  devlog search "error handling patterns"
  devlog search --keyword "db"     # Force keyword-only search`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSearch,
}

func init() {
	rootCmd.AddCommand(searchCmd)

	searchCmd.Flags().StringVar(&searchCodebase, "codebase", "", "Codebase path (default: current directory)")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 10, "Maximum results to show")
	searchCmd.Flags().BoolVar(&searchKeyword, "keyword", false, "Use keyword search instead of semantic search")
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
		// Keyword search for files
		files, err = db.SearchFilesBySummary(database, codebase.ID, query)
		if err != nil {
			return err
		}

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

	// Display results
	fmt.Println()
	titleColor.Printf("  Search Results for: %s\n", query)
	if useSemanticSearch {
		semanticColor.Println("  🔍 Using semantic search (vector similarity)")
	} else {
		dimColor.Println("  🔤 Using keyword search")
	}
	dimColor.Println("  " + strings.Repeat("─", 50))
	fmt.Println()

	if len(matchingFolders) > 0 {
		titleColor.Println("  📁 Folders")
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
		titleColor.Println("  📄 Files")
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
				dimColor.Printf("    %s • %d lines\n", f.Language, f.LineCount)
			}
			fmt.Println()
		}
	}

	if len(files) == 0 && len(matchingFolders) == 0 {
		dimColor.Println("  No results found.")
		fmt.Println()
		dimColor.Println("  Tips:")
		dimColor.Println("  - Try different keywords")
		dimColor.Println("  - Make sure the codebase is indexed with 'devlog ingest'")
		dimColor.Println("  - Use broader terms")
	}

	fmt.Println()
	return nil
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
