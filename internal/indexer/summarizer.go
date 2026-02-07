package indexer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ishaan812/devlog/internal/llm"
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
	Summary string `json:"summary"`
	Purpose string `json:"purpose"`
}

const fileSummaryPrompt = `Analyze this code file and provide a brief summary.

File: %s
Language: %s

Content (first 2000 chars):
%s

Respond with exactly 3 lines:
1. SUMMARY: A one-sentence summary of what this file does
2. PURPOSE: The main purpose (e.g., "API handler", "Data model", "Utility functions", "Configuration")
3. EXPORTS: Key exports/functions/classes (comma-separated, max 5)

Example:
SUMMARY: Handles user authentication and session management
PURPOSE: Authentication middleware
EXPORTS: loginHandler, logoutHandler, validateToken, refreshSession`

const folderSummaryPrompt = `Analyze this folder structure and provide a brief summary.

Folder: %s
Files: %s
Subfolders: %s

Respond with exactly 2 lines:
1. SUMMARY: A one-sentence summary of what this folder contains
2. PURPOSE: The main purpose (e.g., "API routes", "Database models", "UI components", "Utilities")

Example:
SUMMARY: Contains REST API endpoint handlers for user management
PURPOSE: API handlers`

const codebaseSummaryPrompt = `Analyze this codebase structure and provide a brief summary.

Name: %s
Tech Stack: %s
Main Folders: %s
Total Files: %d

Respond with exactly 2 lines:
1. SUMMARY: A 2-3 sentence summary of what this project does and its architecture
2. TECH: Primary technologies and frameworks used

Example:
SUMMARY: A REST API service for e-commerce operations built with Go. Uses PostgreSQL for data storage and Redis for caching. Follows clean architecture with separate layers for handlers, services, and repositories.
TECH: Go, PostgreSQL, Redis, Docker`

// SummarizeFile generates a summary for a file
func (s *Summarizer) SummarizeFile(ctx context.Context, file FileInfo) (*FileSummary, error) {
	content := file.Content
	if len(content) > 2000 {
		content = content[:2000]
	}

	prompt := fmt.Sprintf(fileSummaryPrompt, file.Path, file.Language, content)

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	response, err := s.client.Complete(ctx, prompt)
	if err != nil {
		return nil, err
	}

	return parseFileSummary(response), nil
}

// SummarizeFolder generates a summary for a folder
func (s *Summarizer) SummarizeFolder(ctx context.Context, folder *FolderInfo) (*FolderSummary, error) {
	var fileNames []string
	for _, f := range folder.Files {
		fileNames = append(fileNames, f.Name)
	}
	if len(fileNames) > 20 {
		fileNames = fileNames[:20]
	}

	var subfolderNames []string
	for _, sf := range folder.SubFolders {
		parts := strings.Split(sf, "/")
		subfolderNames = append(subfolderNames, parts[len(parts)-1])
	}

	prompt := fmt.Sprintf(folderSummaryPrompt,
		folder.Path,
		strings.Join(fileNames, ", "),
		strings.Join(subfolderNames, ", "))

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	response, err := s.client.Complete(ctx, prompt)
	if err != nil {
		return nil, err
	}

	return parseFolderSummary(response), nil
}

// SummarizeCodebase generates an overall summary for the codebase
func (s *Summarizer) SummarizeCodebase(ctx context.Context, result *ScanResult) (string, error) {
	techStack := DetectTechStack(result.Files)
	var techList []string
	for tech, count := range techStack {
		if count > 2 {
			techList = append(techList, tech)
		}
	}

	var mainFolders []string
	for path, folder := range result.Folders {
		if folder.Depth == 1 {
			mainFolders = append(mainFolders, path)
		}
	}

	prompt := fmt.Sprintf(codebaseSummaryPrompt,
		result.Name,
		strings.Join(techList, ", "),
		strings.Join(mainFolders, ", "),
		len(result.Files))

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	response, err := s.client.Complete(ctx, prompt)
	if err != nil {
		return "", err
	}

	// Extract just the summary line
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "SUMMARY:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "SUMMARY:")), nil
		}
	}

	return response, nil
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

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SUMMARY:") {
			summary.Summary = strings.TrimSpace(strings.TrimPrefix(line, "SUMMARY:"))
		} else if strings.HasPrefix(line, "PURPOSE:") {
			summary.Purpose = strings.TrimSpace(strings.TrimPrefix(line, "PURPOSE:"))
		}
	}

	return summary
}
