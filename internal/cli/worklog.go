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
	ParentCount int
	IsMergeSync bool
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
	nameOfUser := getWorklogUserName(cfg)

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
		markdown, err = generateBranchWorklogMarkdown(groups, client, cfg, loc, projectContext, codebaseContext, cache, style, nameOfUser)
	default:
		dayGroups = groupByDate(commits, loc)
		markdown, err = generateWorklogMarkdown(dayGroups, client, cfg, loc, projectContext, codebaseContext, cache, style, nameOfUser)

		// Generate weekly summaries if the date range spans more than one week
		if worklogDays > 7 && cache != nil && !worklogNoLLM {
			successColor.Println("\n  Generating weekly summaries...")
			if err := generateWeeklySummaries(ctx, cache, dayGroups, client, projectContext, codebaseContext, loc, style, nameOfUser); err != nil {
				fmt.Printf("Warning: failed to generate weekly summaries: %v\n", err)
			} else {
				successColor.Println("  ✓ Weekly summaries generated")
			}
		}

		// Generate monthly summaries if the date range is long enough
		if worklogDays > 28 && cache != nil && !worklogNoLLM {
			successColor.Println("\n  Generating monthly summaries...")
			if err := generateMonthlySummaries(ctx, cache, client, projectContext, codebaseContext, loc, style, startDate, endDate, nameOfUser); err != nil {
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
			b.name as branch_name, c.parent_count, c.is_merge_sync
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
			ParentCount: getInt(row, "parent_count"),
			IsMergeSync: getBool(row, "is_merge_sync"),
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

func getInt(m map[string]any, key string) int {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func getBool(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
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

func getWorklogUserName(cfg *config.Config) string {
	userName := cfg.GetEffectiveUserName()
	if userName == "" {
		userName = cfg.GetEffectiveGitHubUsername()
	}
	if userName == "" {
		userName = "User"
	}
	return userName
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

func splitAttributionCommits(commits []commitData) (attribution []commitData, mergeSync []commitData) {
	for _, c := range commits {
		if c.IsMergeSync {
			mergeSync = append(mergeSync, c)
			continue
		}
		attribution = append(attribution, c)
	}
	return attribution, mergeSync
}

func hasAttributionCommits(commits []commitData) bool {
	for _, c := range commits {
		if !c.IsMergeSync {
			return true
		}
	}
	return false
}

func mergeSyncUpdateLine(count int) string {
	if count <= 1 {
		return "- User resolved merge conflicts between the current branch and its parent branch."
	}
	return fmt.Sprintf("- User resolved merge conflicts while syncing the current branch with its parent branch (%d merge-sync commits).", count)
}

func buildCommitContext(c commitData, style string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Commit %s: %s\n", c.Hash[:7], strings.TrimSpace(c.Message)))
	if c.IsMergeSync {
		sb.WriteString("Classification: merge-sync (branch synchronization/conflict resolution)\n")
	}
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

func generateWorklogMarkdown(groups []dayGroup, client llm.Client, cfg *config.Config, loc *time.Location, projectContext string, codebaseContext string, cache *worklogCacheContext, style string, nameOfUser string) (string, error) {
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
				content, err = buildDayBranchSection(commits, client, projectContext, branchCtx, loc, style, nameOfUser)
				if err != nil {
					return "", fmt.Errorf("failed to generate day/branch updates: %w", err)
				}
				storeCacheEntry(ctx, cache, group.Date, branchID, bName, "day_updates", "date", commits, content)
			} else {
				content, cached, err = getCachedOrGenerate(
					ctx, cache, group.Date, branchID, bName,
					"day_updates", "date", commits,
					func() (string, error) {
						return buildDayBranchSection(commits, client, projectContext, branchCtx, loc, style, nameOfUser)
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
			if contextLine != "" && hasAttributionCommits(commits) {
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
		summary, err := generateOverallSummary(groups, client, projectContext, codebaseContext, style, nameOfUser)
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

func generateBranchWorklogMarkdown(groups []branchGroup, client llm.Client, cfg *config.Config, loc *time.Location, projectContext string, codebaseContext string, cache *worklogCacheContext, style string, nameOfUser string) (string, error) {
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
					return generateBranchSummary(group, client, projectContext, branchCtx, style, nameOfUser)
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
				if c.IsMergeSync {
					sb.WriteString(" [merge-sync]")
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

func buildDayBranchSection(commits []commitData, client llm.Client, projectContext string, branchContext string, loc *time.Location, style string, nameOfUser string) (string, error) {
	var section strings.Builder
	attributionCommits, mergeSyncCommits := splitAttributionCommits(commits)

	if client != nil && len(attributionCommits) > 0 {
		updatesSummary, err := generateDayBranchUpdates(attributionCommits, client, projectContext, branchContext, style, nameOfUser)
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
	if len(mergeSyncCommits) > 0 {
		section.WriteString(mergeSyncUpdateLine(len(mergeSyncCommits)))
		section.WriteString("\n")
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
		if c.IsMergeSync {
			section.WriteString(" [merge-sync]")
		}
		section.WriteString("\n")
	}

	return section.String(), nil
}

func generateBranchSummary(group branchGroup, client llm.Client, projectContext string, branchContext string, style string, nameOfUser string) (string, error) {
	attributionCommits, mergeSyncCommits := splitAttributionCommits(group.Commits)
	if len(attributionCommits) == 0 {
		if len(mergeSyncCommits) > 0 {
			return "### Updates\n\n" + mergeSyncUpdateLine(len(mergeSyncCommits)), nil
		}
		return "", nil
	}

	var commitBlocks []string
	for _, c := range attributionCommits {
		commitBlocks = append(commitBlocks, buildCommitContext(c, style))
	}

	if len(commitBlocks) == 0 {
		return "", nil
	}

	stats := buildAggregateStats(attributionCommits)

	var prompt string
	if style == "technical" {
		prompt = prompts.BuildWorklogBranchSummaryPrompt(nameOfUser, projectContext, branchContext, strings.Join(commitBlocks, "\n---\n"), stats)
	} else {
		prompt = prompts.BuildWorklogBranchSummaryPromptNonTechnical(nameOfUser, projectContext, branchContext, strings.Join(commitBlocks, "\n---\n"), stats)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	result, err := client.Complete(ctx, prompt)
	if err != nil {
		return "", err
	}
	return result, nil
}

func generateDayBranchUpdates(commits []commitData, client llm.Client, projectContext string, branchContext string, style string, nameOfUser string) (string, error) {
	var commitBlocks []string
	for _, c := range commits {
		commitBlocks = append(commitBlocks, buildCommitContext(c, style))
	}

	if len(commitBlocks) == 0 {
		return "", nil
	}

	var prompt string
	if style == "technical" {
		prompt = prompts.BuildWorklogDayUpdatesPrompt(nameOfUser, projectContext, branchContext, strings.Join(commitBlocks, "\n---\n"))
	} else {
		prompt = prompts.BuildWorklogDayUpdatesPromptNonTechnical(nameOfUser, projectContext, branchContext, strings.Join(commitBlocks, "\n---\n"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	result, err := client.Complete(ctx, prompt)
	if err != nil {
		return "", err
	}
	return result, nil
}

func generateOverallSummary(groups []dayGroup, client llm.Client, projectContext string, codebaseContext string, style string, nameOfUser string) (string, error) {
	var allCommits []commitData
	var commitBlocks []string
	for _, g := range groups {
		for _, c := range g.Commits {
			if c.IsMergeSync {
				continue
			}
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
		prompt = prompts.BuildWorklogOverallSummaryPrompt(nameOfUser, projectContext, codebaseContext, strings.Join(commitBlocks, "\n---\n"), stats)
	} else {
		prompt = prompts.BuildWorklogOverallSummaryPromptNonTechnical(nameOfUser, projectContext, codebaseContext, strings.Join(commitBlocks, "\n---\n"), stats)
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
func generateWeeklySummaries(ctx context.Context, cache *worklogCacheContext, groups []dayGroup, client llm.Client, projectContext, codebaseContext string, loc *time.Location, style string, nameOfUser string) error {
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
		attributionWeekCommits, _ := splitAttributionCommits(weekCommits)
		stats := buildAggregateStats(attributionWeekCommits)
		dailySummaryText := strings.Join(dailySummaries, "\n\n")

		periodContext := buildWeeklyPeriodContext(weekCommits, weekDays, loc)
		var prompt string
		if style == "technical" {
			prompt = prompts.BuildWorklogWeekSummaryPrompt(nameOfUser, projectContext, codebaseContext, periodContext, dailySummaryText, stats)
		} else {
			prompt = prompts.BuildWorklogWeekSummaryPromptNonTechnical(nameOfUser, projectContext, codebaseContext, periodContext, dailySummaryText, stats)
		}

		timeoutCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
		content, err := client.Complete(timeoutCtx, prompt)
		cancel()

		if err != nil {
			VerboseLog("Warning: LLM weekly summary failed for %s, using fallback: %v", weekStart.Format("Jan 2"), err)
			content = buildFallbackWeeklySummary(weekStart, weekDays, weekCommits, loc)
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

func weekLabelFromDate(t time.Time, loc *time.Location) string {
	return getWeekStart(t, loc).Format("Jan 2")
}

func buildWeeklyPeriodContext(weekCommits []commitData, weekDays []dayGroup, loc *time.Location) string {
	branchSet := make(map[string]bool)
	dayToBranches := make(map[string]map[string]bool)
	for _, c := range weekCommits {
		branch := strings.TrimSpace(c.BranchName)
		if branch == "" {
			branch = "unknown"
		}
		branchSet[branch] = true

		dayKey := c.CommittedAt.In(loc).Format("2006-01-02")
		if dayToBranches[dayKey] == nil {
			dayToBranches[dayKey] = make(map[string]bool)
		}
		dayToBranches[dayKey][branch] = true
	}

	branches := make([]string, 0, len(branchSet))
	for b := range branchSet {
		branches = append(branches, b)
	}
	sort.Strings(branches)

	var sb strings.Builder
	sb.WriteString("Active branches this week:\n")
	for _, b := range branches {
		sb.WriteString(fmt.Sprintf("- %s\n", b))
	}

	if len(weekDays) > 0 {
		sb.WriteString("\nDay-to-branch activity map:\n")
		for _, d := range weekDays {
			dayKey := d.Date.In(loc).Format("2006-01-02")
			branchMap := dayToBranches[dayKey]
			dayBranches := make([]string, 0, len(branchMap))
			for b := range branchMap {
				dayBranches = append(dayBranches, b)
			}
			sort.Strings(dayBranches)
			sb.WriteString(fmt.Sprintf("- %s: %s\n", d.Date.In(loc).Format("Mon Jan 2"), strings.Join(dayBranches, ", ")))
		}
	}

	return sb.String()
}

func buildMonthlyPeriodContext(ctx context.Context, cache *worklogCacheContext, monthStart, monthEnd time.Time, loc *time.Location) string {
	var sb strings.Builder

	// Explicit week labels for THIS month only - LLM must use these for (Weeks: ...) citations.
	weekSet := make(map[string]time.Time)
	for d := monthStart; !d.After(monthEnd); d = d.AddDate(0, 0, 1) {
		ws := getWeekStart(d, loc)
		label := ws.In(loc).Format("Jan 2")
		weekSet[label] = ws
	}
	validWeeks := make([]string, 0, len(weekSet))
	for w := range weekSet {
		validWeeks = append(validWeeks, w)
	}
	sort.Slice(validWeeks, func(i, j int) bool {
		return weekSet[validWeeks[i]].Before(weekSet[validWeeks[j]])
	})
	sb.WriteString(fmt.Sprintf("VALID week labels for this month (use ONLY these in (Weeks: ...) - never use weeks from other months): %s\n\n",
		strings.Join(validWeeks, ", ")))

	if cache == nil || cache.dbRepo == nil {
		return sb.String()
	}

	rows, err := cache.dbRepo.ExecuteQueryWithArgs(ctx, `
		SELECT entry_date, branch_name
		FROM worklog_entries
		WHERE codebase_id = $1 AND profile_name = $2 AND entry_type = 'day_updates'
			AND entry_date >= $3 AND entry_date <= $4
			AND branch_name IS NOT NULL AND branch_name != ''
		ORDER BY entry_date ASC, branch_name ASC`,
		cache.codebaseID, cache.profileName, monthStart, monthEnd)
	if err != nil {
		VerboseLog("Warning: failed to load monthly branch activity context: %v", err)
		return sb.String()
	}

	branchWeeks := make(map[string]map[string]bool)
	for _, row := range rows {
		branch, ok := row["branch_name"].(string)
		if !ok || strings.TrimSpace(branch) == "" {
			continue
		}

		entryDate, ok := row["entry_date"].(time.Time)
		if !ok {
			continue
		}

		week := weekLabelFromDate(entryDate, loc)
		if branchWeeks[branch] == nil {
			branchWeeks[branch] = make(map[string]bool)
		}
		branchWeeks[branch][week] = true
	}

	branches := make([]string, 0, len(branchWeeks))
	for b := range branchWeeks {
		branches = append(branches, b)
	}
	sort.Strings(branches)

	sb.WriteString("Active branches this month (with weeks they had updates):\n")
	for _, b := range branches {
		weeks := make([]string, 0, len(branchWeeks[b]))
		for w := range branchWeeks[b] {
			weeks = append(weeks, w)
		}
		sort.Strings(weeks)
		sb.WriteString(fmt.Sprintf("- %s: %s\n", b, strings.Join(weeks, ", ")))
	}

	return sb.String()
}

// generateMonthlySummaries generates and caches monthly summary entries
func generateMonthlySummaries(ctx context.Context, cache *worklogCacheContext, client llm.Client, projectContext, codebaseContext string, loc *time.Location, style string, startDate, endDate time.Time, nameOfUser string) error {
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
		// Monthly summaries should still be generated even when weekly summaries
		// are missing, using month commits as fallback context.

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
				ParentCount: c.ParentCount,
				IsMergeSync: c.IsMergeSync,
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
		attributionMonthCommits, _ := splitAttributionCommits(monthCommitData)
		monthStats := buildAggregateStats(attributionMonthCommits)

		// Generate monthly summary.
		periodContext := buildMonthlyPeriodContext(ctx, cache, monthStart, monthEnd, loc)
		var prompt string
		if style == "technical" {
			prompt = prompts.BuildWorklogMonthSummaryPrompt(nameOfUser, projectContext, codebaseContext, periodContext, strings.Join(summaryTexts, "\n\n"), monthStats)
		} else {
			prompt = prompts.BuildWorklogMonthSummaryPromptNonTechnical(nameOfUser, projectContext, codebaseContext, periodContext, strings.Join(summaryTexts, "\n\n"), monthStats)
		}

		timeoutCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
		content, err := client.Complete(timeoutCtx, prompt)
		cancel()

		if err != nil {
			VerboseLog("Warning: LLM monthly summary failed for %s, using fallback: %v", monthStart.Format("January 2006"), err)
			content = buildFallbackMonthlySummary(monthStart, monthEnd, monthCommitData, loc)
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

func buildFallbackWeeklySummary(weekStart time.Time, weekDays []dayGroup, weekCommits []commitData, loc *time.Location) string {
	weekEnd := weekStart.AddDate(0, 0, 6)
	adds, dels := computeCommitStats(weekCommits)
	fileSet := make(map[string]bool)
	branchSet := make(map[string]bool)
	for _, c := range weekCommits {
		for _, f := range c.Files {
			fileSet[f] = true
		}
		if c.BranchName != "" {
			branchSet[c.BranchName] = true
		}
	}

	dayCount := len(weekDays)
	branchCount := len(branchSet)
	fileCount := len(fileSet)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## %s - %s\n\n", weekStart.In(loc).Format("Jan 2"), weekEnd.In(loc).Format("Jan 2, 2006")))
	sb.WriteString("- Weekly summary fallback (LLM unavailable for this run)\n")
	sb.WriteString(fmt.Sprintf("- %d active day(s), %d commit(s), %d branch(es)\n", dayCount, len(weekCommits), branchCount))
	sb.WriteString(fmt.Sprintf("- +%d/-%d lines changed across %d file(s)\n", adds, dels, fileCount))
	sb.WriteString("\n### Notable commits\n\n")

	limit := 8
	if len(weekCommits) < limit {
		limit = len(weekCommits)
	}
	for i := 0; i < limit; i++ {
		c := weekCommits[i]
		msg := strings.Split(strings.TrimSpace(c.Message), "\n")[0]
		if len(msg) > 100 {
			msg = msg[:97] + "..."
		}
		branch := c.BranchName
		if branch == "" {
			branch = "unknown"
		}
		sb.WriteString(fmt.Sprintf("- `%s` (%s) %s\n", c.Hash[:7], branch, msg))
	}

	return sb.String()
}

func buildFallbackMonthlySummary(monthStart, monthEnd time.Time, monthCommits []commitData, loc *time.Location) string {
	adds, dels := computeCommitStats(monthCommits)
	fileSet := make(map[string]bool)
	branchSet := make(map[string]bool)
	for _, c := range monthCommits {
		for _, f := range c.Files {
			fileSet[f] = true
		}
		if c.BranchName != "" {
			branchSet[c.BranchName] = true
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## %s\n\n", monthStart.In(loc).Format("January 2006")))
	sb.WriteString("- Monthly summary fallback (LLM unavailable for this run)\n")
	sb.WriteString(fmt.Sprintf("- %d commit(s) across %d branch(es)\n", len(monthCommits), len(branchSet)))
	sb.WriteString(fmt.Sprintf("- +%d/-%d lines changed across %d file(s)\n", adds, dels, len(fileSet)))
	sb.WriteString(fmt.Sprintf("- Period: %s - %s\n", monthStart.In(loc).Format("Jan 2"), monthEnd.In(loc).Format("Jan 2, 2006")))
	return sb.String()
}
