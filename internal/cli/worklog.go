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
	"github.com/ishaan812/devlog/internal/config"
	"github.com/ishaan812/devlog/internal/db"
	"github.com/ishaan812/devlog/internal/llm"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
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
  devlog worklog --days 7                    # Last 7 days to stdout
  devlog worklog --days 30 --output log.md   # Last 30 days to file
  devlog worklog --days 14 --no-llm          # Without LLM summaries
  devlog worklog --group-by branch           # Group by branch
  devlog worklog --branch feature/auth       # Single branch worklog
  devlog worklog --all                       # Include all commits (not just yours)`,
	RunE: runWorklog,
}

func init() {
	rootCmd.AddCommand(worklogCmd)

	worklogCmd.Flags().IntVar(&worklogDays, "days", 7, "Number of days to include")
	worklogCmd.Flags().StringVarP(&worklogOutput, "output", "o", "", "Output file path (default: stdout)")
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

func queryCommitsForWorklog(database *gorm.DB, codebase *db.Codebase, startDate, endDate time.Time, cfg *config.Config) ([]commitData, error) {
	tx := database.Model(&db.Commit{}).
		Where("committed_at >= ? AND committed_at <= ?", startDate, endDate)

	// Filter by codebase if available
	if codebase != nil {
		tx = tx.Where("codebase_id = ?", codebase.ID)
	}

	// Filter by user commits unless --all
	if !worklogAll {
		tx = tx.Where("is_user_commit = ?", true)
	}

	// Filter by branch if specified
	if worklogBranch != "" && codebase != nil {
		branch, err := db.GetBranch(database, codebase.ID, worklogBranch)
		if err != nil || branch == nil {
			return nil, fmt.Errorf("branch '%s' not found", worklogBranch)
		}
		tx = tx.Where("branch_id = ?", branch.ID)
	}

	var dbCommits []db.Commit
	err := tx.Order("committed_at DESC").Find(&dbCommits).Error
	if err != nil {
		return nil, err
	}

	// Build commit data with branch info
	branchCache := make(map[string]*db.Branch)
	var commits []commitData
	for _, c := range dbCommits {
		cd := commitData{
			Hash:        c.Hash,
			Message:     c.Message,
			AuthorEmail: c.AuthorEmail,
			CommittedAt: c.CommittedAt,
			BranchID:    c.BranchID,
		}

		// Get branch name
		if c.BranchID != "" {
			if branch, ok := branchCache[c.BranchID]; ok {
				cd.BranchName = branch.Name
			} else {
				branch, _ := db.GetBranchByID(database, c.BranchID)
				if branch != nil {
					branchCache[c.BranchID] = branch
					cd.BranchName = branch.Name
				}
			}
		}

		// Get file changes for this commit
		var fileChanges []db.FileChange
		database.Where("commit_id = ?", c.ID).Find(&fileChanges)

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

func groupByBranch(database *gorm.DB, commits []commitData) []branchGroup {
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
			if c.BranchName != "" {
				sb.WriteString(fmt.Sprintf(" [%s]", c.BranchName))
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

func generateBranchWorklogMarkdown(groups []branchGroup, client llm.LLMClient, cfg *config.Config) (string, error) {
	var sb strings.Builder

	// Header
	userName := cfg.UserName
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
