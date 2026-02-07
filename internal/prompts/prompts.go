package prompts

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed sql_generation.md
var sqlGenerationPromptTemplate string

//go:embed summarization.md
var summarizationPromptTemplate string

//go:embed time_filter.md
var timeFilterPromptTemplate string

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

func BuildSQLPrompt(schema, question string) string {
	return fmt.Sprintf(strings.TrimSpace(sqlGenerationPromptTemplate), schema, question)
}

func BuildSummarizationPrompt(question, results string) string {
	return fmt.Sprintf(strings.TrimSpace(summarizationPromptTemplate), question, results)
}

func BuildTimeFilterPrompt(question string) string {
	return fmt.Sprintf(strings.TrimSpace(timeFilterPromptTemplate), question)
}

func BuildFileSummaryPrompt(filePath, language, content string) string {
	return fmt.Sprintf(strings.TrimSpace(fileSummaryPromptTemplate), filePath, language, content)
}

func BuildFolderSummaryPrompt(folderPath, files, subfolders string) string {
	return fmt.Sprintf(strings.TrimSpace(folderSummaryPromptTemplate), folderPath, files, subfolders)
}

func BuildCodebaseSummaryPrompt(name, techStack, mainFolders string, totalFiles int) string {
	return fmt.Sprintf(strings.TrimSpace(codebaseSummaryPromptTemplate), name, techStack, mainFolders, totalFiles)
}

func BuildCommitSummaryPrompt(commitContent string) string {
	return fmt.Sprintf(strings.TrimSpace(commitSummaryPromptTemplate), commitContent)
}

func BuildCommitSummarizerPrompt(commitContent string) string {
	return fmt.Sprintf(strings.TrimSpace(commitSummarizerPromptTemplate), commitContent)
}
