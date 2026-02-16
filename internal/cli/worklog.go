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
	"github.com/google/uuid"
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
	worklogNoCache  bool
	worklogStyle    string
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

Style Options:
  non-technical - Focus on high-level goals and accomplishments (default)
  technical     - Include file paths, code changes, and technical details

Note: Days are processed oldest-to-newest so that branch context builds
chronologically. If you extend your date range (e.g. from 7 to 14 days),
previously cached summaries for newer days will be regenerated to include
the correct context from the newly included older days.

Examples:
  devlog worklog                              # Writes worklog_<start>_<end>.md
  devlog worklog --days 30                    # Last 30 days
  devlog worklog --days 14 --output log.md    # Custom output filename
  devlog worklog --no-llm                     # Without LLM summaries
  devlog worklog --group-by branch            # Group by branch
  devlog worklog --branch feature/auth        # Single branch worklog
  devlog worklog --all                        # Include all commits (not just yours)
  devlog worklog --no-cache                   # Force regeneration of all summaries
  devlog worklog --style technical            # Use technical style for this worklog`,
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
	worklogCmd.Flags().BoolVar(&worklogNoCache, "no-cache", false, "Skip cache and regenerate all LLM summaries")
	worklogCmd.Flags().StringVar(&worklogStyle, "style", "", "Worklog style: 'technical' or 'non-technical' (default: profile setting or 'non-technical')")
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
	successColor := color.New(color.FgGreen)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w\n\nRun 'devlog onboard' to set up your configuration", err)
	}

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
	codebaseContext := getCodebaseContext(codebase)

	style := worklogStyle
	if style == "" {
		style = cfg.GetWorklogStyle() // defaults to "non-technical"
	}
	if style != "technical" && style != "non-technical" {
		return fmt.Errorf("invalid worklog style: %s (must be 'technical' or 'non-technical')", style)
	}

	var cache *worklogCacheContext
	if codebase != nil {
		cache = &worklogCacheContext{
			dbRepo:                dbRepo,
			codebaseID:            codebase.ID,
			profileName:           cfg.GetActiveProfileName(),
			loc:                   loc,
			noCache:               worklogNoCache,
			ChangedDailySummaries: make(map[time.Time]bool),
		}
	}

	var markdown string
	var dayGroups []dayGroup // For weekly summary generation

	switch worklogGroupBy {
	case "branch":
		groups, groupErr := groupByBranch(ctx, dbRepo, commits)
		if groupErr != nil {
			return groupErr
		}
		markdown, err = generateBranchWorklogMarkdown(groups, client, cfg, loc, projectContext, codebaseContext, cache, style)
	default:
		dayGroups = groupByDate(commits, loc)
		markdown, err = generateWorklogMarkdown(dayGroups, client, cfg, loc, projectContext, codebaseContext, cache, style)

		// Generate weekly summaries if the date range spans more than one week
		if worklogDays > 7 && cache != nil && !worklogNoLLM {
			successColor.Println("\n  Generating weekly summaries...")
			if err := generateWeeklySummaries(ctx, cache, dayGroups, client, projectContext, codebaseContext, loc, style); err != nil {
				fmt.Printf("Warning: failed to generate weekly summaries: %v\n", err)
			} else {
				successColor.Println("  ✓ Weekly summaries generated")
			}
		}

		// Generate monthly summaries if the date range is long enough
		if worklogDays > 28 && cache != nil && !worklogNoLLM {
			successColor.Println("\n  Generating monthly summaries...")
			if err := generateMonthlySummaries(ctx, cache, client, projectContext, codebaseContext, loc, style, startDate, endDate); err != nil {
				fmt.Printf("Warning: failed to generate monthly summaries: %v\n", err)
			} else {
				successColor.Println("  ✓ Monthly summaries generated")
			}
		}
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

func getProjectContext(codebase *db.Codebase) string {
	if codebase != nil && codebase.Summary != "" {
		return codebase.Summary
	}
	return "(No project context available)"
}

func getCodebaseContext(codebase *db.Codebase) string {
	if codebase == nil {
		return "(No codebase context available)"
	}
	var parts []string
	if codebase.ProjectContext != "" {
		parts = append(parts, fmt.Sprintf("Current project focus:\n%s", codebase.ProjectContext))
	}
	if codebase.LongtermContext != "" {
		parts = append(parts, fmt.Sprintf("Long-term goals:\n%s", codebase.LongtermContext))
	}
	if len(parts) == 0 {
		return "(No codebase context available)"
	}
	return strings.Join(parts, "\n\n")
}

func extractContextLine(section string) string {
	lines := strings.Split(section, "\n")
	var featureBullets []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "- **") ||
			strings.HasPrefix(trimmed, "- 🐛") || strings.HasPrefix(trimmed, "- 🔧") ||
			trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			featureBullets = append(featureBullets, strings.TrimPrefix(trimmed, "- "))
		}
	}
	if len(featureBullets) == 0 {
		return ""
	}
	result := featureBullets[0]
	if len(result) > 150 {
		result = result[:147] + "..."
	}
	return result
}

func buildCommitContext(c commitData, style string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Commit %s: %s\n", c.Hash[:7], strings.TrimSpace(c.Message)))
	if c.Summary != "" {
		sb.WriteString(fmt.Sprintf("Summary: %s\n", c.Summary))
	}

	if style == "technical" {
		sb.WriteString(fmt.Sprintf("Stats: +%d/-%d lines\n", c.Additions, c.Deletions))
		if len(c.Files) > 0 {
			sb.WriteString(fmt.Sprintf("Files: %s\n", strings.Join(c.Files, ", ")))
		}
	}
	return sb.String()
}

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

type worklogCacheContext struct {
	dbRepo                *db.SQLRepository
	codebaseID            string
	profileName           string
	loc                   *time.Location
	noCache               bool
	ChangedDailySummaries map[time.Time]bool
}

func computeCommitHashes(commits []commitData) string {
	hashes := make([]string, len(commits))
	for i, c := range commits {
		hashes[i] = c.Hash
	}
	sort.Strings(hashes)
	return strings.Join(hashes, ",")
}

func computeCommitStats(commits []commitData) (int, int) {
	adds, dels := 0, 0
	for _, c := range commits {
		adds += c.Additions
		dels += c.Deletions
	}
	return adds, dels
}

func getCachedOrGenerate(
	ctx context.Context,
	cache *worklogCacheContext,
	date time.Time,
	branchID, branchName string,
	entryType, groupBy string,
	commits []commitData,
	generator func() (string, error),
) (string, bool, error) {
	if cache == nil || cache.dbRepo == nil || cache.codebaseID == "" {
		content, err := generator()
		return content, false, err
	}

	today := time.Now().In(cache.loc).Truncate(24 * time.Hour)
	entryDay := date.In(cache.loc).Truncate(24 * time.Hour)
	isToday := entryDay.Equal(today)
	currentHashes := computeCommitHashes(commits)

	if !cache.noCache && !isToday {
		cached, err := cache.dbRepo.GetWorklogEntry(ctx, cache.codebaseID, cache.profileName, date, branchID, entryType, groupBy)
		if err == nil && cached != nil && cached.CommitHashes == currentHashes {
			return cached.Content, true, nil
		}
	}

	content, err := generator()
	if err != nil {
		return "", false, err
	}

	storeCacheEntry(ctx, cache, date, branchID, branchName, entryType, groupBy, commits, content)

	return content, false, nil
}

func storeCacheEntry(ctx context.Context, cache *worklogCacheContext, date time.Time, branchID, branchName, entryType, groupBy string, commits []commitData, content string) {
	if cache == nil || cache.dbRepo == nil || cache.codebaseID == "" {
		return
	}
	adds, dels := computeCommitStats(commits)
	entry := &db.WorklogEntry{
		ID:           uuid.New().String(),
		CodebaseID:   cache.codebaseID,
		ProfileName:  cache.profileName,
		EntryDate:    date,
		BranchID:     branchID,
		BranchName:   branchName,
		EntryType:    entryType,
		GroupBy:      groupBy,
		Content:      content,
		CommitCount:  len(commits),
		Additions:    adds,
		Deletions:    dels,
		CommitHashes: computeCommitHashes(commits),
		CreatedAt:    time.Now(),
	}
	if storeErr := cache.dbRepo.UpsertWorklogEntry(ctx, entry); storeErr != nil {
		VerboseLog("Warning: failed to cache worklog entry: %v", storeErr)
	}
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

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Date.Before(groups[j].Date)
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
		selectedProvider = cfg.GetEffectiveProvider()
	}
	if selectedProvider == "" {
		return nil, fmt.Errorf("no provider configured; run 'devlog onboard' first")
	}
	selectedModel := worklogModel // from CLI flag
	if selectedModel == "" {
		selectedModel = cfg.GetEffectiveModel()
	}
	llmCfg := llm.Config{Provider: llm.Provider(selectedProvider), Model: selectedModel}
	switch llmCfg.Provider {
	case llm.ProviderOpenAI:
		llmCfg.APIKey = cfg.GetEffectiveAPIKey("openai")
	case llm.ProviderChatGPT:
		llmCfg.APIKey = cfg.GetEffectiveAPIKey("chatgpt")
	case llm.ProviderAnthropic:
		llmCfg.APIKey = cfg.GetEffectiveAPIKey("anthropic")
	case llm.ProviderOpenRouter:
		llmCfg.APIKey = cfg.GetEffectiveAPIKey("openrouter")
	case llm.ProviderGemini:
		llmCfg.APIKey = cfg.GetEffectiveAPIKey("gemini")
	case llm.ProviderBedrock:
		llmCfg.AWSAccessKeyID = cfg.GetEffectiveAWSAccessKeyID()
		llmCfg.AWSSecretAccessKey = cfg.GetEffectiveAWSSecretAccessKey()
		llmCfg.AWSRegion = cfg.GetEffectiveAWSRegion()
	case llm.ProviderOllama:
		if url := cfg.GetEffectiveOllamaBaseURL(); url != "" {
			llmCfg.BaseURL = url
		}
		// Ollama uses its own model field as override
		if selectedModel == "" {
			if model := cfg.GetEffectiveOllamaModel(); model != "" {
				llmCfg.Model = model
			}
		}
	}
	return llm.NewClient(llmCfg)
}

type dayOutputSection struct {
	dayName  string
	branches []branchOutputSection
}

type branchOutputSection struct {
	branchName string
	content    string
}

func generateWorklogMarkdown(groups []dayGroup, client llm.Client, cfg *config.Config, loc *time.Location, projectContext string, codebaseContext string, cache *worklogCacheContext, style string) (string, error) {
	ctx := context.Background()
	dimColor := color.New(color.FgHiBlack)
	cacheColor := color.New(color.FgHiGreen)
	warnColor := color.New(color.FgYellow)

	branchContextMap := make(map[string]string)
	branchCacheBusted := make(map[string]bool)
	cacheInvalidatedCount := 0
	daySections := make([]dayOutputSection, 0, len(groups))

	for _, group := range groups {
		dayName := group.Date.In(loc).Format("Monday, January 2, 2006")
		ds := dayOutputSection{dayName: dayName}

		branchCommits := make(map[string][]commitData)
		branchIDs := make(map[string]string)
		branchOrder := []string{}
		for _, c := range group.Commits {
			bName := c.BranchName
			if bName == "" {
				bName = "unknown"
			}
			if _, exists := branchCommits[bName]; !exists {
				branchOrder = append(branchOrder, bName)
			}
			branchCommits[bName] = append(branchCommits[bName], c)
			if c.BranchID != "" {
				branchIDs[bName] = c.BranchID
			}
		}

		for _, bName := range branchOrder {
			commits := branchCommits[bName]
			branchID := branchIDs[bName]

			branchCtx := branchContextMap[branchID]
			if branchCtx == "" {
				branchCtx = "(No previous context for this branch)"
			}

			forceRegen := branchCacheBusted[branchID]

			var content string
			var cached bool
			var err error

			if forceRegen {
				content, err = buildDayBranchSection(commits, client, projectContext, branchCtx, loc, style)
				if err != nil {
					return "", fmt.Errorf("failed to generate day/branch updates: %w", err)
				}
				storeCacheEntry(ctx, cache, group.Date, branchID, bName, "day_updates", "date", commits, content)
			} else {
									content, cached, err = getCachedOrGenerate(
										ctx, cache, group.Date, branchID, bName,
										"day_updates", "date", commits,
										func() (string, error) {
											return buildDayBranchSection(commits, client, projectContext, branchCtx, loc, style)
										},
									)
									if err != nil {
										return "", fmt.Errorf("failed to generate day/branch updates: %w", err)
									}
									if !cached && cache != nil {
										if cache.ChangedDailySummaries == nil {
											cache.ChangedDailySummaries = make(map[time.Time]bool)
										}
										cache.ChangedDailySummaries[group.Date.In(loc).Truncate(24*time.Hour)] = true
									}
			}
			if cached {
				cacheColor.Printf("  %s [%s]: cached\n", group.Date.In(loc).Format("Jan 2"), bName)
			} else {
				if forceRegen {
					warnColor.Printf("  %s [%s]: regenerated (context updated)\n", group.Date.In(loc).Format("Jan 2"), bName)
					cacheInvalidatedCount++
				} else {
					dimColor.Printf("  %s [%s]: generated\n", group.Date.In(loc).Format("Jan 2"), bName)
				}
				branchCacheBusted[branchID] = true
			}

			contextLine := extractContextLine(content)
			if contextLine != "" {
				dateStr := group.Date.In(loc).Format("Jan 2")
				entry := fmt.Sprintf("- %s: %s", dateStr, contextLine)
				if prev := branchContextMap[branchID]; prev != "" {
					branchContextMap[branchID] = prev + "\n" + entry
				} else {
					branchContextMap[branchID] = entry
				}
				lines := strings.Split(branchContextMap[branchID], "\n")
				if len(lines) > 10 {
					branchContextMap[branchID] = strings.Join(lines[len(lines)-10:], "\n")
				}
			}

			ds.branches = append(ds.branches, branchOutputSection{branchName: bName, content: content})
		}

		daySections = append(daySections, ds)
	}

	if cache != nil && cache.dbRepo != nil {
		for branchID, ctxSummary := range branchContextMap {
			if err := cache.dbRepo.UpdateBranchContext(ctx, branchID, ctxSummary); err != nil {
				VerboseLog("Warning: failed to save branch context: %v", err)
			}
		}
	}

	if cacheInvalidatedCount > 0 {
		warnColor.Printf("\n  Note: %d cached entries were regenerated because the date range was extended.\n", cacheInvalidatedCount)
		warnColor.Println("  Branch context flows chronologically, so newer days were re-summarized with updated context.")
	}

	var sb strings.Builder

	userName := cfg.GetEffectiveUserName()
	if userName == "" {
		userName = cfg.GetEffectiveGitHubUsername()
	}
	if userName == "" {
		userName = "Developer"
	}

	sb.WriteString(fmt.Sprintf("# Work Log - %s\n\n", userName))
	sb.WriteString(fmt.Sprintf("*Generated on %s*\n\n", time.Now().In(loc).Format("January 2, 2006")))

	if len(groups) > 0 {
		startDate := groups[0].Date.In(loc)
		endDate := groups[len(groups)-1].Date.In(loc)
		sb.WriteString(fmt.Sprintf("**Period:** %s - %s\n\n", startDate.Format("Jan 2"), endDate.Format("Jan 2, 2006")))
	}

	sb.WriteString("---\n\n")

	if client != nil {
		summary, err := generateOverallSummary(groups, client, projectContext, codebaseContext, style)
		if err != nil {
			return "", fmt.Errorf("failed to generate overall summary: %w", err)
		}
		if summary != "" {
			sb.WriteString("## Summary\n\n")
			sb.WriteString(summary)
			sb.WriteString("\n\n---\n\n")
		}
	}

	for i := len(daySections) - 1; i >= 0; i-- {
		ds := daySections[i]
		sb.WriteString(fmt.Sprintf("# %s\n\n", ds.dayName))
		for _, bs := range ds.branches {
			sb.WriteString(fmt.Sprintf("## Branch: %s\n\n", bs.branchName))
			sb.WriteString(bs.content)
			sb.WriteString("\n")
		}
		sb.WriteString("---\n\n")
	}

	sb.WriteString("*Generated by [DevLog](https://github.com/ishaan812/devlog)*\n")

	return sb.String(), nil
}

func generateBranchWorklogMarkdown(groups []branchGroup, client llm.Client, cfg *config.Config, loc *time.Location, projectContext string, codebaseContext string, cache *worklogCacheContext, style string) (string, error) {
	ctx := context.Background()
	dimColor := color.New(color.FgHiBlack)
	cacheColor := color.New(color.FgHiGreen)

	var sb strings.Builder

	userName := cfg.GetEffectiveUserName()
	if userName == "" {
		userName = cfg.GetEffectiveGitHubUsername()
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
		branchID := ""
		if group.Branch != nil {
			branchName = group.Branch.Name
			branchID = group.Branch.ID
		}

		sb.WriteString(fmt.Sprintf("# Branch: %s\n\n", branchName))

		totalAdditions := 0
		totalDeletions := 0
		for _, c := range group.Commits {
			totalAdditions += c.Additions
			totalDeletions += c.Deletions
		}

		sb.WriteString("## Summary\n\n")

		var dbRepoRef *db.SQLRepository
		if cache != nil {
			dbRepoRef = cache.dbRepo
		}
		branchCtx := "(No previous context for this branch)"
		if dbRepoRef != nil && branchID != "" {
			if branch, err := dbRepoRef.GetBranchByID(ctx, branchID); err == nil && branch != nil && branch.ContextSummary != "" {
				branchCtx = branch.ContextSummary
			}
		}

		if client != nil {
			var entryDate time.Time
			if len(group.Commits) > 0 {
				entryDate = group.Commits[0].CommittedAt.In(loc).Truncate(24 * time.Hour)
			}

			branchSummary, cached, err := getCachedOrGenerate(
				ctx, cache, entryDate, branchID, branchName,
				"branch_summary", "branch", group.Commits,
				func() (string, error) {
					return generateBranchSummary(group, client, projectContext, branchCtx, style)
				},
			)
			if err != nil {
				return "", fmt.Errorf("failed to generate branch summary: %w", err)
			}
			if cached {
				cacheColor.Printf("  Branch %s: cached\n", branchName)
			} else {
				dimColor.Printf("  Branch %s: generated\n", branchName)
			}
			if branchSummary != "" {
				sb.WriteString(branchSummary)
				sb.WriteString("\n\n")
			}
		}

		sb.WriteString("## Daily Activity\n\n")
		sb.WriteString(fmt.Sprintf("**%d commits** | +%d / -%d lines\n\n", len(group.Commits), totalAdditions, totalDeletions))

		commitsByDate := make(map[string][]commitData)
		for _, c := range group.Commits {
			localTime := c.CommittedAt.In(loc)
			dateKey := localTime.Format("2006-01-02")
			commitsByDate[dateKey] = append(commitsByDate[dateKey], c)
		}

		var dates []string
		for d := range commitsByDate {
			dates = append(dates, d)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(dates)))

		for _, dateStr := range dates {
			date, _ := time.Parse("2006-01-02", dateStr)
			dayCommits := commitsByDate[dateStr]

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

				if c.Summary != "" {
					sb.WriteString(fmt.Sprintf("  - %s\n", c.Summary))
				}
			}
			sb.WriteString("\n")
		}
		sb.WriteString("---\n\n")
	}

	sb.WriteString("*Generated by [DevLog](https://github.com/ishaan812/devlog)*\n")

	return sb.String(), nil
}

func buildDayBranchSection(commits []commitData, client llm.Client, projectContext string, branchContext string, loc *time.Location, style string) (string, error) {
	var section strings.Builder

	if client != nil {
		updatesSummary, err := generateDayBranchUpdates(commits, client, projectContext, branchContext, style)
		if err != nil {
			return "", err
		}
		if updatesSummary != "" {
			section.WriteString(updatesSummary)
			section.WriteString("\n")
		}
	} else {
		section.WriteString("### Updates\n\n")
	}
	section.WriteString("\n")

	section.WriteString("### Commits\n\n")

	sorted := make([]commitData, len(commits))
	copy(sorted, commits)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CommittedAt.After(sorted[j].CommittedAt)
	})

	for _, c := range sorted {
		commitTime := c.CommittedAt.In(loc).Format("15:04")
		message := strings.Split(strings.TrimSpace(c.Message), "\n")[0]
		section.WriteString(fmt.Sprintf("- **%s** `%s` %s", commitTime, c.Hash[:7], message))
		if c.Additions > 0 || c.Deletions > 0 {
			section.WriteString(fmt.Sprintf(" (+%d/-%d)", c.Additions, c.Deletions))
		}
		section.WriteString("\n")
	}

	return section.String(), nil
}

func generateBranchSummary(group branchGroup, client llm.Client, projectContext string, branchContext string, style string) (string, error) {
	var commitBlocks []string
	for _, c := range group.Commits {
		commitBlocks = append(commitBlocks, buildCommitContext(c, style))
	}

	if len(commitBlocks) == 0 {
		return "", nil
	}

	stats := buildAggregateStats(group.Commits)

	var prompt string
	if style == "technical" {
		prompt = prompts.BuildWorklogBranchSummaryPrompt(projectContext, branchContext, strings.Join(commitBlocks, "\n---\n"), stats)
	} else {
		prompt = prompts.BuildWorklogBranchSummaryPromptNonTechnical(projectContext, branchContext, strings.Join(commitBlocks, "\n---\n"), stats)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	result, err := client.Complete(ctx, prompt)
	if err != nil {
		return "", err
	}
	return result, nil
}

func generateDayBranchUpdates(commits []commitData, client llm.Client, projectContext string, branchContext string, style string) (string, error) {
	var commitBlocks []string
	for _, c := range commits {
		commitBlocks = append(commitBlocks, buildCommitContext(c, style))
	}

	if len(commitBlocks) == 0 {
		return "", nil
	}

	var prompt string
	if style == "technical" {
		prompt = prompts.BuildWorklogDayUpdatesPrompt(projectContext, branchContext, strings.Join(commitBlocks, "\n---\n"))
	} else {
		prompt = prompts.BuildWorklogDayUpdatesPromptNonTechnical(projectContext, branchContext, strings.Join(commitBlocks, "\n---\n"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	result, err := client.Complete(ctx, prompt)
	if err != nil {
		return "", err
	}
	return result, nil
}

func generateOverallSummary(groups []dayGroup, client llm.Client, projectContext string, codebaseContext string, style string) (string, error) {
	var allCommits []commitData
	var commitBlocks []string
	for _, g := range groups {
		for _, c := range g.Commits {
			allCommits = append(allCommits, c)
			commitBlocks = append(commitBlocks, buildCommitContext(c, style))
		}
	}

	if len(commitBlocks) == 0 {
		return "", nil
	}

	stats := buildAggregateStats(allCommits)

	var prompt string
	if style == "technical" {
		prompt = prompts.BuildWorklogOverallSummaryPrompt(projectContext, codebaseContext, strings.Join(commitBlocks, "\n---\n"), stats)
	} else {
		prompt = prompts.BuildWorklogOverallSummaryPromptNonTechnical(projectContext, codebaseContext, strings.Join(commitBlocks, "\n---\n"), stats)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := client.Complete(ctx, prompt)
	if err != nil {
		return "", err
	}
	return result, nil
}

// getWeekStart returns the Sunday (start of week) for a given date
func getWeekStart(t time.Time, loc *time.Location) time.Time {
	localTime := t.In(loc)
	// Go's time.Weekday() returns 0 for Sunday, 1 for Monday, etc.
	daysSinceSunday := int(localTime.Weekday())
	weekStart := localTime.AddDate(0, 0, -daysSinceSunday)
	return time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, loc)
}

// generateWeeklySummaries generates and caches weekly summary entries
func generateWeeklySummaries(ctx context.Context, cache *worklogCacheContext, groups []dayGroup, client llm.Client, projectContext, codebaseContext string, loc *time.Location, style string) error {
	if cache == nil || cache.dbRepo == nil {
		return nil
	}

	// Group days by week
	weekGroups := make(map[time.Time][]dayGroup)
	for _, group := range groups {
		weekStart := getWeekStart(group.Date, loc)
		weekGroups[weekStart] = append(weekGroups[weekStart], group)
	}

	// Generate summary for each week
	for weekStart, weekDays := range weekGroups {
		// Skip if only one day in the week
		if len(weekDays) <= 1 {
			continue
		}

		// Collect all commits for the week
		var weekCommits []commitData
		var dailySummaries []string
		for _, day := range weekDays {
			weekCommits = append(weekCommits, day.Commits...)
			// Get cached daily summaries to include in weekly context
			entries, err := cache.dbRepo.ListWorklogEntriesByDate(ctx, cache.codebaseID, cache.profileName, day.Date)
			if err == nil {
				for _, entry := range entries {
					if entry.EntryType == "day_updates" {
						dateStr := day.Date.In(loc).Format("Monday, January 2")
						dailySummaries = append(dailySummaries, fmt.Sprintf("### %s\n\n%s", dateStr, entry.Content))
					}
				}
			}
		}

		if len(weekCommits) == 0 {
			continue
		}

		// Check if any daily summaries within this week have changed
		dailySummariesChanged := false
		if cache.ChangedDailySummaries != nil {
			for _, dayGroup := range weekDays {
				if cache.ChangedDailySummaries[dayGroup.Date.In(loc).Truncate(24*time.Hour)] {
					dailySummariesChanged = true
					break
				}
			}
		}

		// Check if we already have a cached weekly summary
		existing, err := cache.dbRepo.GetWeeklySummary(ctx, cache.codebaseID, cache.profileName, weekStart)
		currentHashes := computeCommitHashes(weekCommits)

		// Skip if cache is valid and no daily summaries changed
		if err == nil && existing != nil && existing.CommitHashes == currentHashes && !cache.noCache && !dailySummariesChanged {
			continue
		}

		// If the cache is being busted, clear the old entry first
		if existing != nil && (cache.noCache || dailySummariesChanged) {
			if err := cache.dbRepo.DeleteWorklogEntry(ctx, existing.ID); err != nil {
				VerboseLog("Warning: failed to delete old weekly summary: %v", err)
			}
		}

		// Generate weekly summary
		stats := buildAggregateStats(weekCommits)
		dailySummaryText := strings.Join(dailySummaries, "\n\n")

		var prompt string
		if style == "technical" {
			prompt = prompts.BuildWorklogWeekSummaryPrompt(projectContext, codebaseContext, dailySummaryText, stats)
		} else {
			prompt = prompts.BuildWorklogWeekSummaryPromptNonTechnical(projectContext, codebaseContext, dailySummaryText, stats)
		}

		timeoutCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
		content, err := client.Complete(timeoutCtx, prompt)
		cancel()

		if err != nil {
			return fmt.Errorf("failed to generate weekly summary for %s: %w", weekStart.Format("Jan 2"), err)
		}

		// Store the weekly summary
		adds, dels := computeCommitStats(weekCommits)
		weekEntry := &db.WorklogEntry{
			ID:           fmt.Sprintf("week-%s-%s", cache.codebaseID, weekStart.Format("2006-01-02")),
			CodebaseID:   cache.codebaseID,
			ProfileName:  cache.profileName,
			EntryDate:    weekStart,
			BranchID:     "",
			BranchName:   "",
			EntryType:    "week_summary",
			GroupBy:      "date",
			Content:      content,
			CommitCount:  len(weekCommits),
			Additions:    adds,
			Deletions:    dels,
			CommitHashes: currentHashes,
			CreatedAt:    time.Now(),
		}

		if err := cache.dbRepo.UpsertWorklogEntry(ctx, weekEntry); err != nil {
			return fmt.Errorf("failed to cache weekly summary: %w", err)
		}
	}

	return nil
}

// getMonthStart returns the first day of the month for a given date
func getMonthStart(t time.Time, loc *time.Location) time.Time {
	localTime := t.In(loc)
	return time.Date(localTime.Year(), localTime.Month(), 1, 0, 0, 0, 0, loc)
}

// generateMonthlySummaries generates and caches monthly summary entries
func generateMonthlySummaries(ctx context.Context, cache *worklogCacheContext, client llm.Client, projectContext, codebaseContext string, loc *time.Location, style string, startDate, endDate time.Time) error {
	if cache == nil || cache.dbRepo == nil {
		return nil
	}

	currentMonth := getMonthStart(startDate, loc)
	endMonth := getMonthStart(endDate, loc)

	for !currentMonth.After(endMonth) {
		monthStart := currentMonth
		monthEnd := monthStart.AddDate(0, 1, -1)

		weeklySummaries, err := cache.dbRepo.GetWeeklySummariesInRange(ctx, cache.codebaseID, cache.profileName, monthStart, monthEnd)
		if err != nil {
			return fmt.Errorf("failed to get weekly summaries for monthly generation: %w", err)
		}
		if len(weeklySummaries) == 0 {
			currentMonth = currentMonth.AddDate(0, 1, 0)
			continue
		}

		// Check if any daily summaries within this month have changed.
		dailySummariesChanged := false
		if cache.ChangedDailySummaries != nil {
			for date := monthStart; !date.After(monthEnd); date = date.AddDate(0, 0, 1) {
				if cache.ChangedDailySummaries[date.In(loc).Truncate(24*time.Hour)] {
					dailySummariesChanged = true
					break
				}
			}
		}

		// Check if we already have a cached monthly summary.
		existing, err := cache.dbRepo.GetMonthlySummary(ctx, cache.codebaseID, cache.profileName, monthStart)

		// For monthly summaries, recompute the overall commit hashes from weekly summaries.
		var allWeeklyCommitHashes []string
		for _, ws := range weeklySummaries {
			allWeeklyCommitHashes = append(allWeeklyCommitHashes, strings.Split(ws.CommitHashes, ",")...)
		}
		sort.Strings(allWeeklyCommitHashes)
		currentHashes := strings.Join(allWeeklyCommitHashes, ",")

		// Skip if cache is valid and no daily summaries changed.
		if err == nil && existing != nil && existing.CommitHashes == currentHashes && !cache.noCache && !dailySummariesChanged {
			currentMonth = currentMonth.AddDate(0, 1, 0)
			continue
		}

		// If the cache is being busted, clear the old entry first.
		if existing != nil && (cache.noCache || dailySummariesChanged) {
			if err := cache.dbRepo.DeleteWorklogEntry(ctx, existing.ID); err != nil {
				VerboseLog("Warning: failed to delete old monthly summary: %v", err)
			}
		}

		var summaryTexts []string
		// Re-fetch weekly summaries to ensure we have the most up-to-date content.
		// This is crucial because generateWeeklySummaries might have just updated them.
		updatedWeeklySummaries, err := cache.dbRepo.GetWeeklySummariesInRange(ctx, cache.codebaseID, cache.profileName, monthStart, monthEnd)
		if err != nil {
			return fmt.Errorf("failed to re-fetch weekly summaries for monthly generation: %w", err)
		}
		for _, summary := range updatedWeeklySummaries {
			summaryTexts = append(summaryTexts, summary.Content)
		}

		// Aggregate all commits for accurate stats calculation for the month.
		monthCommits, err := cache.dbRepo.GetCommitsBetweenDates(ctx, cache.codebaseID, monthStart, monthEnd)
		if err != nil {
			return fmt.Errorf("failed to get commits for monthly summary stats: %w", err)
		}
		monthCommitData := make([]commitData, 0, len(monthCommits))
		for _, c := range monthCommits {
			cd := commitData{
				Hash:        c.Hash,
				Message:     c.Message,
				Summary:     c.Summary,
				AuthorEmail: c.AuthorEmail,
				CommittedAt: c.CommittedAt,
			}
			switch v := c.Stats["additions"].(type) {
			case int:
				cd.Additions = v
			case int32:
				cd.Additions = int(v)
			case int64:
				cd.Additions = int(v)
			case float64:
				cd.Additions = int(v)
			}
			switch v := c.Stats["deletions"].(type) {
			case int:
				cd.Deletions = v
			case int32:
				cd.Deletions = int(v)
			case int64:
				cd.Deletions = int(v)
			case float64:
				cd.Deletions = int(v)
			}
			monthCommitData = append(monthCommitData, cd)
		}
		monthStats := buildAggregateStats(monthCommitData)

		// Generate monthly summary.
		var prompt string
		if style == "technical" {
			prompt = prompts.BuildWorklogMonthSummaryPrompt(projectContext, codebaseContext, strings.Join(summaryTexts, "\n\n"), monthStats)
		} else {
			prompt = prompts.BuildWorklogMonthSummaryPromptNonTechnical(projectContext, codebaseContext, strings.Join(summaryTexts, "\n\n"), monthStats)
		}

		timeoutCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
		content, err := client.Complete(timeoutCtx, prompt)
		cancel()

		if err != nil {
			return fmt.Errorf("failed to generate monthly summary for %s: %w", monthStart.Format("January 2006"), err)
		}

		// Store the monthly summary.
		adds, dels := computeCommitStats(monthCommitData)
		monthEntry := &db.WorklogEntry{
			ID:           fmt.Sprintf("month-%s-%s", cache.codebaseID, monthStart.Format("2006-01")),
			CodebaseID:   cache.codebaseID,
			ProfileName:  cache.profileName,
			EntryDate:    monthStart,
			EntryType:    "month_summary",
			GroupBy:      "date",
			Content:      content,
			CommitCount:  len(monthCommits),
			Additions:    adds,
			Deletions:    dels,
			CommitHashes: currentHashes,
			CreatedAt:    time.Now(),
		}

		if err := cache.dbRepo.UpsertWorklogEntry(ctx, monthEntry); err != nil {
			return fmt.Errorf("failed to cache monthly summary: %w", err)
		}

		currentMonth = currentMonth.AddDate(0, 1, 0)
	}

	return nil
}
