package indexer

import (
	"context"
	"strings"
	"time"

	"github.com/ishaan812/devlog/internal/llm"
	"github.com/ishaan812/devlog/internal/prompts"
)

// Summarizer generates summaries for code files and folders.
type Summarizer struct {
	client  llm.Client
	verbose bool
}

// NewSummarizer creates a new summarizer.
func NewSummarizer(client llm.Client, verbose bool) *Summarizer {
	return &Summarizer{client: client, verbose: verbose}
}

// FileSummary holds the generated summary for a file
type FileSummary struct {
	Summary    string   `json:"summary"`
	Purpose    string   `json:"purpose"`
	KeyExports []string `json:"key_exports"`
}

// FolderSummary holds the generated summary for a folder
type FolderSummary struct {
	Summary               string   `json:"summary"`
	Purpose               string   `json:"purpose"`
	Themes                string   `json:"themes"`
	FileDescriptions      []string `json:"file_descriptions"`
	SubfolderDescriptions []string `json:"subfolder_descriptions"`
}

// SummarizeFile generates a summary for a file
func (s *Summarizer) SummarizeFile(ctx context.Context, file FileInfo) (*FileSummary, error) {
	content := file.Content
	if len(content) > 2000 {
		content = content[:2000]
	}

	prompt := prompts.BuildFileSummaryPrompt(file.Path, file.Language, content)

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	response, err := s.client.Complete(ctx, prompt)
	if err != nil {
		return nil, err
	}

	return parseFileSummary(response), nil
}

// SummarizeFolder generates a summary for a folder
func (s *Summarizer) SummarizeFolder(ctx context.Context, folder *FolderInfo, touchedFiles []string, maxChildren int) (*FolderSummary, error) {
	var fileNames []string
	for _, f := range folder.Files {
		fileNames = append(fileNames, f.Name)
	}
	if maxChildren <= 0 {
		maxChildren = 12
	}
	if len(fileNames) > maxChildren {
		fileNames = fileNames[:maxChildren]
	}

	var subfolderNames []string
	for _, sf := range folder.SubFolders {
		parts := strings.Split(sf, "/")
		subfolderNames = append(subfolderNames, parts[len(parts)-1])
	}
	if len(subfolderNames) > maxChildren {
		subfolderNames = subfolderNames[:maxChildren]
	}
	if len(touchedFiles) > maxChildren {
		touchedFiles = touchedFiles[:maxChildren]
	}

	files := strings.Join(fileNames, ", ")
	if files == "" {
		files = "None"
	}
	subfolders := strings.Join(subfolderNames, ", ")
	if subfolders == "" {
		subfolders = "None"
	}
	touched := strings.Join(touchedFiles, ", ")
	if touched == "" {
		touched = "None"
	}

	prompt := prompts.BuildFolderSummaryPrompt(
		folder.Path,
		files,
		subfolders,
		touched)

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	response, err := s.client.Complete(ctx, prompt)
	if err != nil {
		return nil, err
	}

	return parseFolderSummary(response), nil
}

// SummarizeCodebase generates an overall summary for the codebase
func (s *Summarizer) SummarizeCodebase(ctx context.Context, result *ScanResult, readmeContent string) (string, error) {
	var mainFolders []string
	for path, folder := range result.Folders {
		if folder.Depth == 1 {
			mainFolders = append(mainFolders, path)
		}
	}

	if readmeContent == "" {
		readmeContent = "(No README found)"
	}

	prompt := prompts.BuildCodebaseSummaryPrompt(
		result.Name,
		strings.Join(mainFolders, ", "),
		len(result.Files),
		readmeContent)

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	response, err := s.client.Complete(ctx, prompt)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(response), nil
}

func parseFileSummary(response string) *FileSummary {
	summary := &FileSummary{}
	lines := strings.Split(response, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SUMMARY:") {
			summary.Summary = strings.TrimSpace(strings.TrimPrefix(line, "SUMMARY:"))
		} else if strings.HasPrefix(line, "PURPOSE:") {
			summary.Purpose = strings.TrimSpace(strings.TrimPrefix(line, "PURPOSE:"))
		} else if strings.HasPrefix(line, "EXPORTS:") {
			exports := strings.TrimSpace(strings.TrimPrefix(line, "EXPORTS:"))
			if exports != "" && exports != "None" && exports != "N/A" {
				for _, e := range strings.Split(exports, ",") {
					e = strings.TrimSpace(e)
					if e != "" {
						summary.KeyExports = append(summary.KeyExports, e)
					}
				}
			}
		}
	}

	return summary
}

func parseFolderSummary(response string) *FolderSummary {
	summary := &FolderSummary{}
	lines := strings.Split(response, "\n")
	section := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "SUMMARY:") {
			summary.Summary = strings.TrimSpace(strings.TrimPrefix(trimmed, "SUMMARY:"))
			section = ""
		} else if strings.HasPrefix(trimmed, "PURPOSE:") {
			summary.Purpose = strings.TrimSpace(strings.TrimPrefix(trimmed, "PURPOSE:"))
			section = ""
		} else if strings.HasPrefix(trimmed, "THEMES:") {
			summary.Themes = strings.TrimSpace(strings.TrimPrefix(trimmed, "THEMES:"))
			section = ""
		} else if strings.HasPrefix(trimmed, "FILES:") {
			section = "files"
		} else if strings.HasPrefix(trimmed, "SUBFOLDERS:") {
			section = "subfolders"
		} else if strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			if item == "" || item == "None" {
				continue
			}
			if section == "files" {
				summary.FileDescriptions = append(summary.FileDescriptions, item)
			} else if section == "subfolders" {
				summary.SubfolderDescriptions = append(summary.SubfolderDescriptions, item)
			}
		}
	}

	return summary
}
