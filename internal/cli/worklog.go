package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/db"
	"github.com/ishaan812/devlog/internal/llm"
	"github.com/ishaan812/devlog/internal/prompts"
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

// getProfileTimezone returns the timezone location for the active profile
func getProfileTimezone(cfg *config.Config) *time.Location {
	loc := time.UTC // Default to UTC
	if cfg.Profiles != nil && cfg.ActiveProfile != "" {
		if profile := cfg.Profiles[cfg.ActiveProfile]; profile != nil && profile.Timezone != "" {
			if tzLoc, err := time.LoadLocation(profile.Timezone); err == nil {
				loc = tzLoc
			}
		}
	}
	return loc
}

func runWorklog(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	titleColor := color.New(color.FgHiCyan, color.Bold)
	dimColor := color.New(color.FgHiBlack)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w\n\nRun 'devlog onboard' to set up your configuration", err)
	}

	// Get timezone for the active profile
	loc := getProfileTimezone(cfg)

	dbRepo, err := db.GetRepository()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	codebasePath, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("failed to resolve current directory: %w", err)
	}
	codebase, err := dbRepo.GetCodebaseByPath(ctx, codebasePath)
	if err != nil || codebase == nil {
		VerboseLog("No codebase found at current path, querying all commits")
	}

	endDate := time.Now().In(loc)
	startDate := endDate.AddDate(0, 0, -worklogDays)

	commits, err := queryCommitsForWorklog(ctx, dbRepo, codebase, startDate, endDate, cfg)
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

	var client llm.Client
	if !worklogNoLLM {
		client, err = createWorklogClient(cfg)
		if err != nil {
			return fmt.Errorf("failed to create LLM client: %w\n\nTo skip LLM summaries, use: --no-llm", err)
		}
	}

	projectContext := getProjectContext(codebase)

	var markdown string
	switch worklogGroupBy {
	case "branch":
		groups, groupErr := groupByBranch(ctx, dbRepo, commits)
		if groupErr != nil {
			return groupErr
		}
		markdown, err = generateBranchWorklogMarkdown(groups, client, cfg, loc, projectContext)
	default:
		groups := groupByDate(commits, loc)
		markdown, err = generateWorklogMarkdown(groups, client, cfg, loc, projectContext)
	}

	if err != nil {
		return fmt.Errorf("failed to generate markdown: %w", err)
	}

	outputPath := worklogOutput
	if outputPath == "" {
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

func queryCommitsForWorklog(ctx context.Context, dbRepo *db.SQLRepository, codebase *db.Codebase, startDate, endDate time.Time, cfg *config.Config) ([]commitData, error) {
	queryStr := `
		SELECT c.id, c.hash, c.codebase_id, c.branch_id, c.author_email, c.message, c.summary, c.committed_at,
			b.name as branch_name
		FROM commits c
		LEFT JOIN branches b ON c.branch_id = b.id
		WHERE c.committed_at >= $1 AND c.committed_at <= $2
	`
	args := []any{startDate, endDate}
	argIdx := 3

	if codebase != nil {
		queryStr += fmt.Sprintf(" AND c.codebase_id = $%d", argIdx)
		args = append(args, codebase.ID)
		argIdx++
	}

	if !worklogAll {
		queryStr += " AND c.is_user_commit = TRUE"
	}

	if worklogBranch != "" && codebase != nil {
		branch, err := dbRepo.GetBranch(ctx, codebase.ID, worklogBranch)
		if err != nil || branch == nil {
			return nil, fmt.Errorf("branch '%s' not found", worklogBranch)
		}
		queryStr += fmt.Sprintf(" AND c.branch_id = $%d", argIdx)
		args = append(args, branch.ID)
	}

	queryStr += " ORDER BY c.committed_at DESC"

	results, err := dbRepo.ExecuteQueryWithArgs(ctx, queryStr, args...)
	if err != nil {
		return nil, err
	}

	var commits []commitData
	for _, row := range results {
		cd := commitData{
			Hash:        getString(row, "hash"),
			Message:     getString(row, "message"),
			Summary:     getString(row, "summary"),
			AuthorEmail: getString(row, "author_email"),
			BranchID:    getString(row, "branch_id"),
			BranchName:  getString(row, "branch_name"),
		}
		if t, ok := row["committed_at"].(time.Time); ok {
			cd.CommittedAt = t
		}

		if id := getString(row, "id"); id != "" {
			fileChanges, err := dbRepo.GetFileChangesByCommit(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("failed to get file changes for commit %s: %w", id, err)
			}
			for _, fc := range fileChanges {
				cd.Additions += fc.Additions
				cd.Deletions += fc.Deletions
				cd.Files = append(cd.Files, fc.FilePath)
			}
		}

		commits = append(commits, cd)
	}

	return commits, nil
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key]; ok && v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// getProjectContext returns the codebase summary for use as LLM context
func getProjectContext(codebase *db.Codebase) string {
	if codebase != nil && codebase.Summary != "" {
		return codebase.Summary
	}
	return "(No project context available)"
}

// buildCommitContext builds a rich text block describing a single commit for LLM consumption
func buildCommitContext(c commitData) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Commit %s: %s\n", c.Hash[:7], strings.TrimSpace(c.Message)))
	if c.Summary != "" {
		sb.WriteString(fmt.Sprintf("Summary: %s\n", c.Summary))
	}
	sb.WriteString(fmt.Sprintf("Stats: +%d/-%d lines\n", c.Additions, c.Deletions))
	if len(c.Files) > 0 {
		sb.WriteString(fmt.Sprintf("Files: %s\n", strings.Join(c.Files, ", ")))
	}
	return sb.String()
}

// buildAggregateStats builds a stats summary string for a set of commits
func buildAggregateStats(commits []commitData) string {
	totalAdditions := 0
	totalDeletions := 0
	fileSet := make(map[string]bool)
	for _, c := range commits {
		totalAdditions += c.Additions
		totalDeletions += c.Deletions
		for _, f := range c.Files {
			fileSet[f] = true
		}
	}
	return fmt.Sprintf("%d commits | +%d/-%d lines | %d unique files changed", len(commits), totalAdditions, totalDeletions, len(fileSet))
}

func groupByDate(commits []commitData, loc *time.Location) []dayGroup {
	dateMap := make(map[string][]commitData)

	for _, c := range commits {
		// Convert commit time to user's timezone
		localTime := c.CommittedAt.In(loc)
		dateKey := localTime.Format("2006-01-02")
		dateMap[dateKey] = append(dateMap[dateKey], c)
	}

	var groups []dayGroup
	for dateStr, dayCommits := range dateMap {
		date, _ := time.ParseInLocation("2006-01-02", dateStr, loc)
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

func groupByBranch(ctx context.Context, dbRepo *db.SQLRepository, commits []commitData) ([]branchGroup, error) {
	branchMap := make(map[string]*branchGroup)
	branchOrder := []string{}

	for _, c := range commits {
		branchID := c.BranchID
		if branchID == "" {
			branchID = "unknown"
		}

		if _, exists := branchMap[branchID]; !exists {
			branch, err := dbRepo.GetBranchByID(ctx, branchID)
			if err != nil {
				return nil, fmt.Errorf("failed to get branch %s: %w", branchID, err)
			}
			branchMap[branchID] = &branchGroup{Branch: branch, Commits: []commitData{}}
			branchOrder = append(branchOrder, branchID)
		}
		branchMap[branchID].Commits = append(branchMap[branchID].Commits, c)
	}

	var groups []branchGroup
	for _, id := range branchOrder {
		groups = append(groups, *branchMap[id])
	}

	return groups, nil
}

func createWorklogClient(cfg *config.Config) (llm.Client, error) {
	selectedProvider := worklogProvider
	if selectedProvider == "" {
		selectedProvider = cfg.DefaultProvider
	}
	if selectedProvider == "" {
		return nil, fmt.Errorf("no provider configured; run 'devlog onboard' first")
	}
	selectedModel := worklogModel // from CLI flag
	if selectedModel == "" {
		selectedModel = cfg.DefaultModel
	}
	llmCfg := llm.Config{Provider: llm.Provider(selectedProvider), Model: selectedModel}
	switch llmCfg.Provider {
	case llm.ProviderOpenAI:
		llmCfg.APIKey = cfg.GetAPIKey("openai")
	case llm.ProviderAnthropic:
		llmCfg.APIKey = cfg.GetAPIKey("anthropic")
	case llm.ProviderOpenRouter:
		llmCfg.APIKey = cfg.GetAPIKey("openrouter")
	case llm.ProviderGemini:
		llmCfg.APIKey = cfg.GetAPIKey("gemini")
	case llm.ProviderBedrock:
		llmCfg.AWSAccessKeyID = cfg.AWSAccessKeyID
		llmCfg.AWSSecretAccessKey = cfg.AWSSecretAccessKey
		llmCfg.AWSRegion = cfg.AWSRegion
	case llm.ProviderOllama:
		if cfg.OllamaBaseURL != "" {
			llmCfg.BaseURL = cfg.OllamaBaseURL
		}
		// Ollama uses its own model field as override
		if selectedModel == "" && cfg.OllamaModel != "" {
			llmCfg.Model = cfg.OllamaModel
		}
	}
	return llm.NewClient(llmCfg)
}

func generateWorklogMarkdown(groups []dayGroup, client llm.Client, cfg *config.Config, loc *time.Location, projectContext string) (string, error) {
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
	sb.WriteString(fmt.Sprintf("*Generated on %s*\n\n", time.Now().In(loc).Format("January 2, 2006")))

	if len(groups) > 0 {
		startDate := groups[len(groups)-1].Date.In(loc)
		endDate := groups[0].Date.In(loc)
		sb.WriteString(fmt.Sprintf("**Period:** %s - %s\n\n", startDate.Format("Jan 2"), endDate.Format("Jan 2, 2006")))
	}

	sb.WriteString("---\n\n")

	// Summary section (with LLM if available)
	if client != nil {
		summary, err := generateOverallSummary(groups, client, projectContext)
		if err != nil {
			return "", fmt.Errorf("failed to generate overall summary: %w", err)
		}
		if summary != "" {
			sb.WriteString("## Summary\n\n")
			sb.WriteString(summary)
			sb.WriteString("\n\n---\n\n")
		}
	}

	// Daily breakdown - Date first, then branches within each date
	for _, group := range groups {
		dayName := group.Date.In(loc).Format("Monday, January 2, 2006")
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
				updatesSummary, err := generateDayBranchUpdates(commits, client, projectContext)
				if err != nil {
					return "", fmt.Errorf("failed to generate day/branch updates: %w", err)
				}
				if updatesSummary != "" {
					sb.WriteString(updatesSummary)
					sb.WriteString("\n")
				}
			}
			sb.WriteString("\n")

			// Commits section
			sb.WriteString("### Commits\n\n")

			// Sort commits by time (newest first)
			sort.Slice(commits, func(i, j int) bool {
				return commits[i].CommittedAt.After(commits[j].CommittedAt)
			})

			for _, c := range commits {
				commitTime := c.CommittedAt.In(loc).Format("15:04")
				message := strings.Split(strings.TrimSpace(c.Message), "\n")[0]
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

func generateBranchWorklogMarkdown(groups []branchGroup, client llm.Client, cfg *config.Config, loc *time.Location, projectContext string) (string, error) {
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
	sb.WriteString(fmt.Sprintf("*Generated on %s*\n\n", time.Now().In(loc).Format("January 2, 2006")))
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
			branchSummary, err := generateBranchSummary(group, client, projectContext)
			if err != nil {
				return "", fmt.Errorf("failed to generate branch summary: %w", err)
			}
			if branchSummary != "" {
				sb.WriteString(branchSummary)
				sb.WriteString("\n\n")
			}
		}

		// Daily Activity section within this branch
		sb.WriteString("## Daily Activity\n\n")
		sb.WriteString(fmt.Sprintf("**%d commits** | +%d / -%d lines\n\n", len(group.Commits), totalAdditions, totalDeletions))

		// Group commits by date (in user's timezone)
		commitsByDate := make(map[string][]commitData)
		for _, c := range group.Commits {
			localTime := c.CommittedAt.In(loc)
			dateKey := localTime.Format("2006-01-02")
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

			sb.WriteString(fmt.Sprintf("### %s\n\n", date.In(loc).Format("Monday, January 2, 2006")))

			for _, c := range dayCommits {
				commitTime := c.CommittedAt.In(loc).Format("15:04")
				message := strings.Split(strings.TrimSpace(c.Message), "\n")[0]
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

func generateBranchSummary(group branchGroup, client llm.Client, projectContext string) (string, error) {
	// Build rich context for each commit
	var commitBlocks []string
	for _, c := range group.Commits {
		commitBlocks = append(commitBlocks, buildCommitContext(c))
	}

	if len(commitBlocks) == 0 {
		return "", nil
	}

	stats := buildAggregateStats(group.Commits)
	prompt := prompts.BuildWorklogBranchSummaryPrompt(projectContext, strings.Join(commitBlocks, "\n---\n"), stats)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	result, err := client.Complete(ctx, prompt)
	if err != nil {
		return "", err
	}
	return result, nil
}

func generateDayBranchUpdates(commits []commitData, client llm.Client, projectContext string) (string, error) {
	// Build rich context for each commit
	var commitBlocks []string
	for _, c := range commits {
		commitBlocks = append(commitBlocks, buildCommitContext(c))
	}

	if len(commitBlocks) == 0 {
		return "", nil
	}

	prompt := prompts.BuildWorklogDayUpdatesPrompt(projectContext, strings.Join(commitBlocks, "\n---\n"))

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	result, err := client.Complete(ctx, prompt)
	if err != nil {
		return "", err
	}
	return result, nil
}

func generateOverallSummary(groups []dayGroup, client llm.Client, projectContext string) (string, error) {
	// Build rich context for all commits
	var allCommits []commitData
	var commitBlocks []string
	for _, g := range groups {
		for _, c := range g.Commits {
			allCommits = append(allCommits, c)
			commitBlocks = append(commitBlocks, buildCommitContext(c))
		}
	}

	if len(commitBlocks) == 0 {
		return "", nil
	}

	stats := buildAggregateStats(allCommits)
	prompt := prompts.BuildWorklogOverallSummaryPrompt(projectContext, strings.Join(commitBlocks, "\n---\n"), stats)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := client.Complete(ctx, prompt)
	if err != nil {
		return "", err
	}
	return result, nil
}
