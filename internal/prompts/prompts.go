package prompts

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed file_summary.md
var fileSummaryPromptTemplate string

//go:embed folder_summary.md
var folderSummaryPromptTemplate string

//go:embed codebase_summary.md
var codebaseSummaryPromptTemplate string

//go:embed commit_summary.md
var commitSummaryPromptTemplate string

//go:embed commit_summarizer.md
var commitSummarizerPromptTemplate string

//go:embed worklog_overall_summary.md
var worklogOverallSummaryPromptTemplate string

//go:embed worklog_day_updates.md
var worklogDayUpdatesPromptTemplate string

//go:embed worklog_branch_summary.md
var worklogBranchSummaryPromptTemplate string

//go:embed worklog_overall_summary_nontechnical.md
var worklogOverallSummaryNonTechnicalPromptTemplate string

//go:embed worklog_day_updates_nontechnical.md
var worklogDayUpdatesNonTechnicalPromptTemplate string

//go:embed worklog_branch_summary_nontechnical.md
var worklogBranchSummaryNonTechnicalPromptTemplate string

//go:embed worklog_week_summary.md
var worklogWeekSummaryPromptTemplate string

//go:embed worklog_week_summary_nontechnical.md
var worklogWeekSummaryNonTechnicalPromptTemplate string

//go:embed worklog_month_summary.md
var worklogMonthSummaryPromptTemplate string

//go:embed worklog_month_summary_nontechnical.md
var worklogMonthSummaryNonTechnicalPromptTemplate string

//go:embed commit_message.md
var commitMessagePromptTemplate string

func BuildFileSummaryPrompt(filePath, language, content string) string {
	return fmt.Sprintf(strings.TrimSpace(fileSummaryPromptTemplate), filePath, language, content)
}

func BuildFolderSummaryPrompt(folderPath, files, subfolders, touchedFiles string) string {
	return fmt.Sprintf(strings.TrimSpace(folderSummaryPromptTemplate), folderPath, files, subfolders, touchedFiles)
}

func BuildCodebaseSummaryPrompt(name, mainFolders string, totalFiles int, readmeContent string) string {
	return fmt.Sprintf(strings.TrimSpace(codebaseSummaryPromptTemplate), name, mainFolders, totalFiles, readmeContent)
}

func BuildCommitSummaryPrompt(commitContent string) string {
	return fmt.Sprintf(strings.TrimSpace(commitSummaryPromptTemplate), commitContent)
}

func BuildCommitSummarizerPrompt(projectContext, commitContent string) string {
	return fmt.Sprintf(strings.TrimSpace(commitSummarizerPromptTemplate), projectContext, commitContent)
}

func BuildWorklogOverallSummaryPrompt(nameOfUser, projectContext, codebaseContext, commits, stats string) string {
	return fmt.Sprintf(strings.TrimSpace(worklogOverallSummaryPromptTemplate), nameOfUser, projectContext, codebaseContext, commits, stats)
}

func BuildWorklogDayUpdatesPrompt(nameOfUser, projectContext, branchContext, commits string) string {
	return fmt.Sprintf(strings.TrimSpace(worklogDayUpdatesPromptTemplate), nameOfUser, projectContext, branchContext, commits)
}

func BuildWorklogBranchSummaryPrompt(nameOfUser, projectContext, branchContext, commits, stats string) string {
	return fmt.Sprintf(strings.TrimSpace(worklogBranchSummaryPromptTemplate), nameOfUser, projectContext, branchContext, commits, stats)
}

func BuildWorklogOverallSummaryPromptNonTechnical(nameOfUser, projectContext, codebaseContext, commits, stats string) string {
	return fmt.Sprintf(strings.TrimSpace(worklogOverallSummaryNonTechnicalPromptTemplate), nameOfUser, projectContext, codebaseContext, commits, stats)
}

func BuildWorklogDayUpdatesPromptNonTechnical(nameOfUser, projectContext, branchContext, commits string) string {
	return fmt.Sprintf(strings.TrimSpace(worklogDayUpdatesNonTechnicalPromptTemplate), nameOfUser, projectContext, branchContext, commits)
}

func BuildWorklogBranchSummaryPromptNonTechnical(nameOfUser, projectContext, branchContext, commits, stats string) string {
	return fmt.Sprintf(strings.TrimSpace(worklogBranchSummaryNonTechnicalPromptTemplate), nameOfUser, projectContext, branchContext, commits, stats)
}

func BuildCommitMessagePrompt(projectContext, diff string) string {
	return fmt.Sprintf(strings.TrimSpace(commitMessagePromptTemplate), projectContext, diff)
}

func BuildWorklogWeekSummaryPrompt(nameOfUser, projectContext, codebaseContext, periodContext, dailySummaries, stats string) string {
	return fmt.Sprintf(strings.TrimSpace(worklogWeekSummaryPromptTemplate), nameOfUser, projectContext, codebaseContext, periodContext, dailySummaries, stats)
}

func BuildWorklogWeekSummaryPromptNonTechnical(nameOfUser, projectContext, codebaseContext, periodContext, dailySummaries, stats string) string {
	return fmt.Sprintf(strings.TrimSpace(worklogWeekSummaryNonTechnicalPromptTemplate), nameOfUser, projectContext, codebaseContext, periodContext, dailySummaries, stats)
}

func BuildWorklogMonthSummaryPrompt(nameOfUser, projectContext, codebaseContext, periodContext, weeklySummaries, stats string) string {
	return fmt.Sprintf(strings.TrimSpace(worklogMonthSummaryPromptTemplate), nameOfUser, projectContext, codebaseContext, periodContext, weeklySummaries, stats)
}

func BuildWorklogMonthSummaryPromptNonTechnical(nameOfUser, projectContext, codebaseContext, periodContext, weeklySummaries, stats string) string {
	return fmt.Sprintf(strings.TrimSpace(worklogMonthSummaryNonTechnicalPromptTemplate), nameOfUser, projectContext, codebaseContext, periodContext, weeklySummaries, stats)
}
