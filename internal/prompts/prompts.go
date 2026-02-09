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

//go:embed commit_message.md
var commitMessagePromptTemplate string

func BuildFileSummaryPrompt(filePath, language, content string) string {
	return fmt.Sprintf(strings.TrimSpace(fileSummaryPromptTemplate), filePath, language, content)
}

func BuildFolderSummaryPrompt(folderPath, files, subfolders string) string {
	return fmt.Sprintf(strings.TrimSpace(folderSummaryPromptTemplate), folderPath, files, subfolders)
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

func BuildWorklogOverallSummaryPrompt(projectContext, commits, stats string) string {
	return fmt.Sprintf(strings.TrimSpace(worklogOverallSummaryPromptTemplate), projectContext, commits, stats)
}

func BuildWorklogDayUpdatesPrompt(projectContext, commits string) string {
	return fmt.Sprintf(strings.TrimSpace(worklogDayUpdatesPromptTemplate), projectContext, commits)
}

func BuildWorklogBranchSummaryPrompt(projectContext, commits, stats string) string {
	return fmt.Sprintf(strings.TrimSpace(worklogBranchSummaryPromptTemplate), projectContext, commits, stats)
}

func BuildWorklogOverallSummaryPromptNonTechnical(projectContext, commits, stats string) string {
	return fmt.Sprintf(strings.TrimSpace(worklogOverallSummaryNonTechnicalPromptTemplate), projectContext, commits, stats)
}

func BuildWorklogDayUpdatesPromptNonTechnical(projectContext, commits string) string {
	return fmt.Sprintf(strings.TrimSpace(worklogDayUpdatesNonTechnicalPromptTemplate), projectContext, commits)
}

func BuildWorklogBranchSummaryPromptNonTechnical(projectContext, commits, stats string) string {
	return fmt.Sprintf(strings.TrimSpace(worklogBranchSummaryNonTechnicalPromptTemplate), projectContext, commits, stats)
}

func BuildCommitMessagePrompt(projectContext, diff string) string {
	return fmt.Sprintf(strings.TrimSpace(commitMessagePromptTemplate), projectContext, diff)
}
