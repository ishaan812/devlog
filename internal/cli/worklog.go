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

	"github.com/fatih/color"
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
	worklogBranch   string
	worklogAll      bool
	worklogGroupBy  string
)

var worklogCmd = &cobra.Command{
	Use:   "worklog",
	Short: "Generate a work log from your commit history",
	Long: `Generate a formatted markdown work log summarizing your development activity.

The work log groups commits by date or branch and uses an LLM to generate
human-readable summaries of your work. By default, only shows commits
made by the current user.

Grouping Options:
  date    - Group commits by date (default)
  branch  - Group commits by branch with stories

Examples:
  devlog worklog                              # Writes worklog_<start>_<end>.md
  devlog worklog --days 30                    # Last 30 days
  devlog worklog --days 14 --output log.md    # Custom output filename
  devlog worklog --no-llm                     # Without LLM summaries
  devlog worklog --group-by branch            # Group by branch
  devlog worklog --branch feature/auth        # Single branch worklog
  devlog worklog --all                        # Include all commits (not just yours)`,
	RunE: runWorklog,
}

func init() {
	rootCmd.AddCommand(worklogCmd)

	worklogCmd.Flags().IntVar(&worklogDays, "days", 7, "Number of days to include")
	worklogCmd.Flags().StringVarP(&worklogOutput, "output", "o", "", "Output file path (default: worklog_<start>_<end>.md)")
	worklogCmd.Flags().StringVar(&worklogProvider, "provider", "", "LLM provider for summaries")
	worklogCmd.Flags().StringVar(&worklogModel, "model", "", "LLM model to use")
	worklogCmd.Flags().BoolVar(&worklogNoLLM, "no-llm", false, "Skip LLM summaries")
	worklogCmd.Flags().StringVar(&worklogBranch, "branch", "", "Filter by specific branch")
	worklogCmd.Flags().BoolVar(&worklogAll, "all", false, "Include all commits (not just your own)")
	worklogCmd.Flags().StringVar(&worklogGroupBy, "group-by", "date", "Group commits by: date, branch")
}

type commitData struct {
	Hash        string
	Message     string
	Summary     string // LLM-generated summary from ingest
	AuthorEmail string
	CommittedAt time.Time
	Additions   int
	Deletions   int
	Files       []string
	BranchID    string
	BranchName  string
}

type dayGroup struct {
	Date    time.Time
	Commits []commitData
}

type branchGroup struct {
	Branch  *db.Branch
	Commits []commitData
}

func runWorklog(cmd *cobra.Command, args []string) error {
	// Colors for terminal output
	titleColor := color.New(color.FgHiCyan, color.Bold)
	dimColor := color.New(color.FgHiBlack)

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

	// Get codebase
	codebasePath, _ := filepath.Abs(".")
	codebase, err := db.GetCodebaseByPath(database, codebasePath)
	if err != nil || codebase == nil {
		// If no codebase, query all commits across profiles
		VerboseLog("No codebase found at current path, querying all commits")
	}

	// Calculate date range
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -worklogDays)

	// Query commits
	commits, err := queryCommitsForWorklog(database, codebase, startDate, endDate, cfg)
	if err != nil {
		return fmt.Errorf("failed to query commits: %w", err)
	}

	if len(commits) == 0 {
		titleColor.Println("\n  Work Log")
		dimColor.Println("  No commits found in the specified time range.")
		if !worklogAll {
			dimColor.Println("  (Showing only your commits. Use --all to include everyone's)")
		}
		fmt.Println()
		return nil
	}

	// Setup LLM client
	var client llm.LLMClient
	if !worklogNoLLM {
		client, err = createWorklogClient(cfg)
		if err != nil {
			VerboseLog("LLM not available, generating without summaries: %v", err)
		}
	}

	// Generate markdown based on grouping
	var markdown string
	switch worklogGroupBy {
	case "branch":
		groups := groupByBranch(database, commits)
		markdown, err = generateBranchWorklogMarkdown(groups, client, cfg)
	default:
		groups := groupByDate(commits)
		markdown, err = generateWorklogMarkdown(groups, client, cfg)
	}

	if err != nil {
		return fmt.Errorf("failed to generate markdown: %w", err)
	}

	// Output - default to file in current directory
	outputPath := worklogOutput
	if outputPath == "" {
		// Generate default filename with date range
		endDate := time.Now()
		startDate := endDate.AddDate(0, 0, -worklogDays)
		outputPath = fmt.Sprintf("worklog_%s_%s.md", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	}

	dir := filepath.Dir(outputPath)
	if dir != "." && dir != "" {
		os.MkdirAll(dir, 0755)
	}
	if err := os.WriteFile(outputPath, []byte(markdown), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	fmt.Printf("Work log written to %s\n", outputPath)

	return nil
}

func queryCommitsForWorklog(database *sql.DB, codebase *db.Codebase, startDate, endDate time.Time, cfg *config.Config) ([]commitData, error) {
	// Build query with JOIN to get branch name in a single query
	queryStr := `
		SELECT c.id, c.hash, c.codebase_id, c.branch_id, c.author_email, c.message, c.summary, c.committed_at,
			b.name as branch_name
		FROM commits c
		LEFT JOIN branches b ON c.branch_id = b.id
		WHERE c.committed_at >= $1 AND c.committed_at <= $2
	`
	args := []interface{}{startDate, endDate}
	argIdx := 3

	// Filter by codebase if available
	if codebase != nil {
		queryStr += fmt.Sprintf(" AND c.codebase_id = $%d", argIdx)
		args = append(args, codebase.ID)
		argIdx++
	}

	// Filter by user commits unless --all
	if !worklogAll {
		queryStr += " AND c.is_user_commit = TRUE"
	}

	// Filter by branch if specified
	if worklogBranch != "" && codebase != nil {
		branch, err := db.GetBranch(database, codebase.ID, worklogBranch)
		if err != nil || branch == nil {
			return nil, fmt.Errorf("branch '%s' not found", worklogBranch)
		}
		queryStr += fmt.Sprintf(" AND c.branch_id = $%d", argIdx)
		args = append(args, branch.ID)
		argIdx++
	}

	queryStr += " ORDER BY c.committed_at DESC"

	rows, err := database.Query(queryStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// First, collect all commits without nested queries (DuckDB doesn't support concurrent queries)
	type rawCommit struct {
		ID          string
		Hash        string
		CodebaseID  string
		BranchID    sql.NullString
		AuthorEmail string
		Message     string
		Summary     sql.NullString
		CommittedAt time.Time
		BranchName  sql.NullString
	}
	var rawCommits []rawCommit

	for rows.Next() {
		var c rawCommit
		if err := rows.Scan(&c.ID, &c.Hash, &c.CodebaseID, &c.BranchID, &c.AuthorEmail, &c.Message, &c.Summary, &c.CommittedAt, &c.BranchName); err != nil {
			return nil, err
		}
		rawCommits = append(rawCommits, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Close rows before doing file change lookups
	rows.Close()

	// Now build commit data with file changes
	var commits []commitData
	for _, c := range rawCommits {
		cd := commitData{
			Hash:        c.Hash,
			Message:     c.Message,
			Summary:     c.Summary.String,
			AuthorEmail: c.AuthorEmail,
			CommittedAt: c.CommittedAt,
			BranchID:    c.BranchID.String,
			BranchName:  c.BranchName.String,
		}

		// Get file changes for this commit
		fileChanges, _ := db.GetFileChangesByCommit(database, c.ID)
		for _, fc := range fileChanges {
			cd.Additions += fc.Additions
			cd.Deletions += fc.Deletions
			cd.Files = append(cd.Files, fc.FilePath)
		}

		commits = append(commits, cd)
	}

	return commits, nil
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

func groupByBranch(database *sql.DB, commits []commitData) []branchGroup {
	branchMap := make(map[string]*branchGroup)
	branchOrder := []string{}

	for _, c := range commits {
		branchID := c.BranchID
		if branchID == "" {
			branchID = "unknown"
		}

		if _, exists := branchMap[branchID]; !exists {
			branch, _ := db.GetBranchByID(database, branchID)
			branchMap[branchID] = &branchGroup{
				Branch:  branch,
				Commits: []commitData{},
			}
			branchOrder = append(branchOrder, branchID)
		}
		branchMap[branchID].Commits = append(branchMap[branchID].Commits, c)
	}

	var groups []branchGroup
	for _, id := range branchOrder {
		groups = append(groups, *branchMap[id])
	}

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
		userName = cfg.GitHubUsername
	}
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

		// For multiple commits, show day summary
		// For single commit, just show the commit with its summary
		showDaySummary := len(group.Commits) > 1
		if showDaySummary {
			daySummary := getDaySummaryFromCommits(group.Commits)
			if daySummary == "" && client != nil {
				var err error
				daySummary, err = generateDaySummary(group, client)
				if err != nil {
					daySummary = ""
				}
			}
			if daySummary != "" {
				sb.WriteString(fmt.Sprintf("> %s\n\n", daySummary))
			}
		}

		// List commits with their summaries
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
			if c.BranchName != "" {
				sb.WriteString(fmt.Sprintf(" [%s]", c.BranchName))
			}
			sb.WriteString("\n")

			// Add stored summary if available
			if c.Summary != "" {
				sb.WriteString(fmt.Sprintf("  > %s\n", c.Summary))
			}
		}
		sb.WriteString("\n")
	}

	// Footer
	sb.WriteString("---\n\n")
	sb.WriteString("*Generated by [DevLog](https://github.com/ishaan812/devlog)*\n")

	return sb.String(), nil
}

func generateBranchWorklogMarkdown(groups []branchGroup, client llm.LLMClient, cfg *config.Config) (string, error) {
	var sb strings.Builder

	// Header
	userName := cfg.UserName
	if userName == "" {
		userName = cfg.GitHubUsername
	}
	if userName == "" {
		userName = "Developer"
	}

	sb.WriteString(fmt.Sprintf("# Work Log - %s\n\n", userName))
	sb.WriteString(fmt.Sprintf("*Generated on %s*\n\n", time.Now().Format("January 2, 2006")))
	sb.WriteString(fmt.Sprintf("**Period:** Last %d days\n\n", worklogDays))
	sb.WriteString("---\n\n")

	// Branch breakdown
	sb.WriteString("## Work by Branch\n\n")

	for _, group := range groups {
		branchName := "Unknown Branch"
		if group.Branch != nil {
			branchName = group.Branch.Name
		}

		sb.WriteString(fmt.Sprintf("### %s\n\n", branchName))

		// Branch story/description
		if group.Branch != nil && group.Branch.Story != "" {
			sb.WriteString(fmt.Sprintf("*%s*\n\n", group.Branch.Story))
		}

		// Stats
		totalAdditions := 0
		totalDeletions := 0
		for _, c := range group.Commits {
			totalAdditions += c.Additions
			totalDeletions += c.Deletions
		}

		sb.WriteString(fmt.Sprintf("**%d commits** | +%d / -%d lines\n\n", len(group.Commits), totalAdditions, totalDeletions))

		// List commits by date
		commitsByDate := make(map[string][]commitData)
		for _, c := range group.Commits {
			dateKey := c.CommittedAt.Format("2006-01-02")
			commitsByDate[dateKey] = append(commitsByDate[dateKey], c)
		}

		// Sort dates
		var dates []string
		for d := range commitsByDate {
			dates = append(dates, d)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(dates)))

		for _, dateStr := range dates {
			date, _ := time.Parse("2006-01-02", dateStr)
			sb.WriteString(fmt.Sprintf("**%s**\n", date.Format("Jan 2")))

			for _, c := range commitsByDate[dateStr] {
				message := strings.Split(strings.TrimSpace(c.Message), "\n")[0]
				if len(message) > 70 {
					message = message[:67] + "..."
				}
				sb.WriteString(fmt.Sprintf("- `%s` %s", c.Hash[:7], message))
				if c.Additions > 0 || c.Deletions > 0 {
					sb.WriteString(fmt.Sprintf(" (+%d/-%d)", c.Additions, c.Deletions))
				}
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}
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

// getDaySummaryFromCommits combines stored commit summaries into a day summary
func getDaySummaryFromCommits(commits []commitData) string {
	var summaries []string
	for _, c := range commits {
		if c.Summary != "" {
			summaries = append(summaries, c.Summary)
		}
	}

	if len(summaries) == 0 {
		return ""
	}

	if len(summaries) == 1 {
		return summaries[0]
	}

	// Combine multiple summaries
	return strings.Join(summaries, " ")
}
