package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/db"
	"github.com/ishaan812/devlog/internal/llm"
	"github.com/spf13/cobra"
)

var (
	worklogDays     int
	worklogOutput   string
	worklogProvider string
	worklogModel    string
	worklogNoLLM    bool
)

var worklogCmd = &cobra.Command{
	Use:   "worklog",
	Short: "Generate a work log from your commit history",
	Long: `Generate a formatted markdown work log summarizing your development activity.

The work log groups commits by date and uses an LLM to generate
human-readable summaries of your work.

Examples:
  devlog worklog --days 7                    # Last 7 days to stdout
  devlog worklog --days 30 --output log.md   # Last 30 days to file
  devlog worklog --days 14 --no-llm          # Without LLM summaries`,
	RunE: runWorklog,
}

func init() {
	rootCmd.AddCommand(worklogCmd)

	worklogCmd.Flags().IntVar(&worklogDays, "days", 7, "Number of days to include")
	worklogCmd.Flags().StringVarP(&worklogOutput, "output", "o", "", "Output file path (default: stdout)")
	worklogCmd.Flags().StringVar(&worklogProvider, "provider", "", "LLM provider for summaries")
	worklogCmd.Flags().StringVar(&worklogModel, "model", "", "LLM model to use")
	worklogCmd.Flags().BoolVar(&worklogNoLLM, "no-llm", false, "Skip LLM summaries")
}

type commitData struct {
	Hash        string
	Message     string
	AuthorEmail string
	CommittedAt time.Time
	Additions   int
	Deletions   int
	Files       []string
}

type dayGroup struct {
	Date    time.Time
	Commits []commitData
}

func runWorklog(cmd *cobra.Command, args []string) error {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		VerboseLog("Warning: failed to load config: %v", err)
		cfg = &config.Config{DefaultProvider: "ollama"}
	}

	// Initialize database
	database, err := db.GetDB()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Calculate date range
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -worklogDays)

	// Query commits
	commits, err := queryCommits(database, startDate, endDate)
	if err != nil {
		return fmt.Errorf("failed to query commits: %w", err)
	}

	if len(commits) == 0 {
		fmt.Println("No commits found in the specified time range.")
		return nil
	}

	// Group by date
	groups := groupByDate(commits)

	// Generate markdown
	var client llm.LLMClient
	if !worklogNoLLM {
		client, err = createWorklogClient(cfg)
		if err != nil {
			VerboseLog("LLM not available, generating without summaries: %v", err)
		}
	}

	markdown, err := generateWorklogMarkdown(groups, client, cfg)
	if err != nil {
		return fmt.Errorf("failed to generate markdown: %w", err)
	}

	// Output
	if worklogOutput != "" {
		dir := filepath.Dir(worklogOutput)
		if dir != "." {
			os.MkdirAll(dir, 0755)
		}
		if err := os.WriteFile(worklogOutput, []byte(markdown), 0644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
		fmt.Printf("Work log written to %s\n", worklogOutput)
	} else {
		fmt.Println(markdown)
	}

	return nil
}

func queryCommits(database *sql.DB, startDate, endDate time.Time) ([]commitData, error) {
	query := `
		SELECT
			c.hash,
			c.message,
			c.author_email,
			c.committed_at,
			COALESCE(SUM(fc.additions), 0) as additions,
			COALESCE(SUM(fc.deletions), 0) as deletions,
			GROUP_CONCAT(DISTINCT fc.file_path) as files
		FROM commit c
		LEFT JOIN file_change fc ON c.hash = fc.commit_hash
		WHERE c.committed_at >= ? AND c.committed_at <= ?
		GROUP BY c.hash, c.message, c.author_email, c.committed_at
		ORDER BY c.committed_at DESC
	`

	rows, err := database.Query(query, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var commits []commitData
	for rows.Next() {
		var c commitData
		var filesStr sql.NullString
		if err := rows.Scan(&c.Hash, &c.Message, &c.AuthorEmail, &c.CommittedAt, &c.Additions, &c.Deletions, &filesStr); err != nil {
			return nil, err
		}
		if filesStr.Valid && filesStr.String != "" {
			c.Files = strings.Split(filesStr.String, ",")
		}
		commits = append(commits, c)
	}

	return commits, rows.Err()
}

func groupByDate(commits []commitData) []dayGroup {
	dateMap := make(map[string][]commitData)

	for _, c := range commits {
		dateKey := c.CommittedAt.Format("2006-01-02")
		dateMap[dateKey] = append(dateMap[dateKey], c)
	}

	var groups []dayGroup
	for dateStr, dayCommits := range dateMap {
		date, _ := time.Parse("2006-01-02", dateStr)
		groups = append(groups, dayGroup{
			Date:    date,
			Commits: dayCommits,
		})
	}

	// Sort by date descending
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Date.After(groups[j].Date)
	})

	return groups
}

func createWorklogClient(cfg *config.Config) (llm.LLMClient, error) {
	selectedProvider := worklogProvider
	if selectedProvider == "" {
		selectedProvider = cfg.DefaultProvider
	}
	if selectedProvider == "" {
		selectedProvider = "ollama"
	}

	llmCfg := llm.Config{
		Provider: llm.Provider(selectedProvider),
		Model:    worklogModel,
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
		if worklogModel == "" && cfg.OllamaModel != "" {
			llmCfg.Model = cfg.OllamaModel
		}
	}

	return llm.NewClient(llmCfg)
}

func generateWorklogMarkdown(groups []dayGroup, client llm.LLMClient, cfg *config.Config) (string, error) {
	var sb strings.Builder

	// Header
	userName := cfg.UserName
	if userName == "" {
		userName = "Developer"
	}

	sb.WriteString(fmt.Sprintf("# Work Log - %s\n\n", userName))
	sb.WriteString(fmt.Sprintf("*Generated on %s*\n\n", time.Now().Format("January 2, 2006")))

	if len(groups) > 0 {
		startDate := groups[len(groups)-1].Date
		endDate := groups[0].Date
		sb.WriteString(fmt.Sprintf("**Period:** %s - %s\n\n", startDate.Format("Jan 2"), endDate.Format("Jan 2, 2006")))
	}

	sb.WriteString("---\n\n")

	// Summary section (with LLM if available)
	if client != nil {
		summary, err := generateOverallSummary(groups, client)
		if err == nil && summary != "" {
			sb.WriteString("## Summary\n\n")
			sb.WriteString(summary)
			sb.WriteString("\n\n---\n\n")
		}
	}

	// Daily breakdown
	sb.WriteString("## Daily Activity\n\n")

	for _, group := range groups {
		dayName := group.Date.Format("Monday, January 2, 2006")
		sb.WriteString(fmt.Sprintf("### %s\n\n", dayName))

		// Stats for the day
		totalAdditions := 0
		totalDeletions := 0
		for _, c := range group.Commits {
			totalAdditions += c.Additions
			totalDeletions += c.Deletions
		}

		sb.WriteString(fmt.Sprintf("**%d commits** | +%d / -%d lines\n\n", len(group.Commits), totalAdditions, totalDeletions))

		// Generate day summary with LLM
		if client != nil {
			daySummary, err := generateDaySummary(group, client)
			if err == nil && daySummary != "" {
				sb.WriteString(fmt.Sprintf("> %s\n\n", daySummary))
			}
		}

		// List commits
		for _, c := range group.Commits {
			commitTime := c.CommittedAt.Format("15:04")
			message := strings.Split(strings.TrimSpace(c.Message), "\n")[0] // First line only
			if len(message) > 80 {
				message = message[:77] + "..."
			}
			sb.WriteString(fmt.Sprintf("- **%s** `%s` %s", commitTime, c.Hash[:7], message))
			if c.Additions > 0 || c.Deletions > 0 {
				sb.WriteString(fmt.Sprintf(" (+%d/-%d)", c.Additions, c.Deletions))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Footer
	sb.WriteString("---\n\n")
	sb.WriteString("*Generated by [DevLog](https://github.com/ishaan812/devlog)*\n")

	return sb.String(), nil
}

func generateOverallSummary(groups []dayGroup, client llm.LLMClient) (string, error) {
	// Collect all commit messages
	var messages []string
	for _, g := range groups {
		for _, c := range g.Commits {
			messages = append(messages, strings.TrimSpace(c.Message))
		}
	}

	if len(messages) == 0 {
		return "", nil
	}

	// Limit to avoid context overflow
	if len(messages) > 50 {
		messages = messages[:50]
	}

	prompt := fmt.Sprintf(`Summarize the following git commit messages into a brief, professional summary of work accomplished. Write 2-3 sentences highlighting the main themes and accomplishments. Be concise.

Commit messages:
%s`, strings.Join(messages, "\n"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return client.Complete(ctx, prompt)
}

func generateDaySummary(group dayGroup, client llm.LLMClient) (string, error) {
	var messages []string
	for _, c := range group.Commits {
		messages = append(messages, strings.TrimSpace(c.Message))
	}

	if len(messages) == 0 {
		return "", nil
	}

	prompt := fmt.Sprintf(`Summarize these git commits from one day in one sentence. Be brief and professional.

Commits:
%s`, strings.Join(messages, "\n"))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	return client.Complete(ctx, prompt)
}
