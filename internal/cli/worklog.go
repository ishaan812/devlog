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

	// Daily breakdown - Date first, then branches within each date
	for _, group := range groups {
		dayName := group.Date.Format("Monday, January 2, 2006")
		sb.WriteString(fmt.Sprintf("# %s\n\n", dayName))

		// Group commits by branch
		branchCommits := make(map[string][]commitData)
		branchOrder := []string{}
		for _, c := range group.Commits {
			branchName := c.BranchName
			if branchName == "" {
				branchName = "unknown"
			}
			if _, exists := branchCommits[branchName]; !exists {
				branchOrder = append(branchOrder, branchName)
			}
			branchCommits[branchName] = append(branchCommits[branchName], c)
		}

		// For each branch on this day
		for _, branchName := range branchOrder {
			commits := branchCommits[branchName]

			sb.WriteString(fmt.Sprintf("## Branch: %s\n\n", branchName))

			// Updates section - LLM-summarized bullet points
			sb.WriteString("### Updates\n\n")

			// Generate summarized updates using LLM
			if client != nil {
				updatesSummary, err := generateDayBranchUpdates(commits, client)
				if err == nil && updatesSummary != "" {
					sb.WriteString(updatesSummary)
					sb.WriteString("\n")
				} else {
					// Fallback to commit messages
					writeFallbackUpdates(&sb, commits)
				}
			} else {
				// No LLM - use commit messages
				writeFallbackUpdates(&sb, commits)
			}
			sb.WriteString("\n")

			// Commits section
			sb.WriteString("### Commits\n\n")

			// Sort commits by time (newest first)
			sort.Slice(commits, func(i, j int) bool {
				return commits[i].CommittedAt.After(commits[j].CommittedAt)
			})

			for _, c := range commits {
				commitTime := c.CommittedAt.Format("15:04")
				message := strings.Split(strings.TrimSpace(c.Message), "\n")[0]
				if len(message) > 70 {
					message = message[:67] + "..."
				}
				sb.WriteString(fmt.Sprintf("- **%s** `%s` %s", commitTime, c.Hash[:7], message))
				if c.Additions > 0 || c.Deletions > 0 {
					sb.WriteString(fmt.Sprintf(" (+%d/-%d)", c.Additions, c.Deletions))
				}
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}
		sb.WriteString("---\n\n")
	}

	// Footer
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

	for _, group := range groups {
		branchName := "unknown"
		if group.Branch != nil {
			branchName = group.Branch.Name
		}

		// Branch heading
		sb.WriteString(fmt.Sprintf("# Branch: %s\n\n", branchName))

		// Stats
		totalAdditions := 0
		totalDeletions := 0
		for _, c := range group.Commits {
			totalAdditions += c.Additions
			totalDeletions += c.Deletions
		}

		// Summary section - LLM-generated summary of all commits in this branch
		sb.WriteString("## Summary\n\n")

		// Generate a consolidated summary using LLM if available
		if client != nil {
			branchSummary, err := generateBranchSummary(group, client)
			if err == nil && branchSummary != "" {
				sb.WriteString(branchSummary)
				sb.WriteString("\n\n")
			} else {
				// Fallback to bullet points if LLM fails
				writeBranchSummaryBullets(&sb, group)
			}
		} else {
			// No LLM - use bullet points from commit summaries
			writeBranchSummaryBullets(&sb, group)
		}

		// Daily Activity section within this branch
		sb.WriteString("## Daily Activity\n\n")
		sb.WriteString(fmt.Sprintf("**%d commits** | +%d / -%d lines\n\n", len(group.Commits), totalAdditions, totalDeletions))

		// Group commits by date
		commitsByDate := make(map[string][]commitData)
		for _, c := range group.Commits {
			dateKey := c.CommittedAt.Format("2006-01-02")
			commitsByDate[dateKey] = append(commitsByDate[dateKey], c)
		}

		// Sort dates (newest first)
		var dates []string
		for d := range commitsByDate {
			dates = append(dates, d)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(dates)))

		for _, dateStr := range dates {
			date, _ := time.Parse("2006-01-02", dateStr)
			dayCommits := commitsByDate[dateStr]

			// Sort commits within day by time (newest first)
			sort.Slice(dayCommits, func(i, j int) bool {
				return dayCommits[i].CommittedAt.After(dayCommits[j].CommittedAt)
			})

			sb.WriteString(fmt.Sprintf("### %s\n\n", date.Format("Monday, January 2, 2006")))

			for _, c := range dayCommits {
				commitTime := c.CommittedAt.Format("15:04")
				message := strings.Split(strings.TrimSpace(c.Message), "\n")[0]
				if len(message) > 70 {
					message = message[:67] + "..."
				}
				sb.WriteString(fmt.Sprintf("- **%s** `%s` %s", commitTime, c.Hash[:7], message))
				if c.Additions > 0 || c.Deletions > 0 {
					sb.WriteString(fmt.Sprintf(" (+%d/-%d)", c.Additions, c.Deletions))
				}
				sb.WriteString("\n")

				// Add commit summary as nested bullet if available
				if c.Summary != "" {
					sb.WriteString(fmt.Sprintf("  - %s\n", c.Summary))
				}
			}
			sb.WriteString("\n")
		}
		sb.WriteString("---\n\n")
	}

	// Footer
	sb.WriteString("*Generated by [DevLog](https://github.com/ishaan812/devlog)*\n")

	return sb.String(), nil
}

func writeBranchSummaryBullets(sb *strings.Builder, group branchGroup) {
	// Collect summaries from commits as bullet points
	summaryBullets := []string{}
	for _, c := range group.Commits {
		if c.Summary != "" {
			summaryBullets = append(summaryBullets, c.Summary)
		}
	}

	if len(summaryBullets) > 0 {
		for _, summary := range summaryBullets {
			sb.WriteString(fmt.Sprintf("- %s\n", summary))
		}
	} else {
		// Fall back to commit messages if no summaries
		for _, c := range group.Commits {
			message := strings.Split(strings.TrimSpace(c.Message), "\n")[0]
			sb.WriteString(fmt.Sprintf("- %s\n", message))
		}
	}
	sb.WriteString("\n")
}

func generateBranchSummary(group branchGroup, client llm.LLMClient) (string, error) {
	// Collect all summaries and messages
	var summaries []string
	for _, c := range group.Commits {
		if c.Summary != "" {
			summaries = append(summaries, c.Summary)
		} else {
			summaries = append(summaries, strings.TrimSpace(c.Message))
		}
	}

	if len(summaries) == 0 {
		return "", nil
	}

	prompt := fmt.Sprintf(`Summarize the following commit descriptions into a concise overview of the work done on this branch.
Write 2-4 sentences as a cohesive paragraph. Focus on the key accomplishments and changes.
Do NOT start sentences with "This commit" or similar phrases. Write in a direct, professional style.

Commit descriptions:
%s`, strings.Join(summaries, "\n\n"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return client.Complete(ctx, prompt)
}

// generateDayBranchUpdates creates a concise, interesting summary of commits for a branch on a specific day
func generateDayBranchUpdates(commits []commitData, client llm.LLMClient) (string, error) {
	// Collect summaries or messages
	var descriptions []string
	for _, c := range commits {
		if c.Summary != "" {
			descriptions = append(descriptions, c.Summary)
		} else {
			descriptions = append(descriptions, strings.TrimSpace(c.Message))
		}
	}

	if len(descriptions) == 0 {
		return "", nil
	}

	// For a single commit, make a concise bullet point
	if len(descriptions) == 1 {
		prompt := fmt.Sprintf(`Rewrite this commit description as a concise, engaging bullet point (1-2 sentences max).
Start with an action verb. Do NOT start with "This commit" or "The commit".
Output ONLY the plain text (no bullet prefix like "- " or "• ").

Description: %s`, descriptions[0])

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		result, err := client.Complete(ctx, prompt)
		if err != nil {
			return "", err
		}
		// Clean up any bullet prefixes the LLM might have added
		result = strings.TrimSpace(result)
		result = strings.TrimPrefix(result, "- ")
		result = strings.TrimPrefix(result, "• ")
		result = strings.TrimPrefix(result, "* ")
		return fmt.Sprintf("- %s", result), nil
	}

	// For multiple commits, create summarized bullet points
	prompt := fmt.Sprintf(`Summarize these commit descriptions into 2-4 concise bullet points highlighting the key updates.
Each bullet should be 1 sentence, starting with an action verb (Added, Improved, Fixed, Updated, etc.).
Do NOT start with "This commit" or use passive voice. Be specific and technical.
Output ONLY the bullet points, each on its own line starting with "- ".

Commit descriptions:
%s`, strings.Join(descriptions, "\n\n"))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	return client.Complete(ctx, prompt)
}

// writeFallbackUpdates writes commit messages as bullet points when LLM is not available
func writeFallbackUpdates(sb *strings.Builder, commits []commitData) {
	for _, c := range commits {
		if c.Summary != "" {
			// Truncate long summaries
			summary := c.Summary
			if len(summary) > 150 {
				summary = summary[:147] + "..."
			}
			sb.WriteString(fmt.Sprintf("- %s\n", summary))
		} else {
			message := strings.Split(strings.TrimSpace(c.Message), "\n")[0]
			sb.WriteString(fmt.Sprintf("- %s\n", message))
		}
	}
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
