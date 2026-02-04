package git

import (
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type BlameResult struct {
	FilePath        string
	PreviousAuthors map[string]int // email -> line count
	Category        string         // "new_feature", "refactor", "fix"
}

func AnalyzeBlame(repo *Repository, filePath string) (*BlameResult, error) {
	result := &BlameResult{
		FilePath:        filePath,
		PreviousAuthors: make(map[string]int),
	}

	gitRepo := repo.Git()
	head, err := gitRepo.Head()
	if err != nil {
		return nil, err
	}

	commit, err := gitRepo.CommitObject(head.Hash())
	if err != nil {
		return nil, err
	}

	blame, err := git.Blame(commit, filePath)
	if err != nil {
		// File might be new or deleted
		result.Category = "new_feature"
		return result, nil
	}

	// Count lines by author
	for _, line := range blame.Lines {
		result.PreviousAuthors[line.Author]++
	}

	// Categorize based on authors
	result.Category = categorizeChange(result.PreviousAuthors, commit.Author.Email)

	return result, nil
}

func categorizeChange(previousAuthors map[string]int, currentAuthor string) string {
	if len(previousAuthors) == 0 {
		return "new_feature"
	}

	totalLines := 0
	selfLines := 0
	for email, count := range previousAuthors {
		totalLines += count
		if email == currentAuthor {
			selfLines += count
		}
	}

	if totalLines == 0 {
		return "new_feature"
	}

	// If more than 70% was written by the same author, it's likely a refactor
	selfRatio := float64(selfLines) / float64(totalLines)
	if selfRatio > 0.7 {
		return "refactor"
	}

	// If mostly other authors' code, could be a fix
	if selfRatio < 0.3 {
		return "fix"
	}

	return "refactor"
}

func GetFileBlameInfo(repo *Repository, commit *object.Commit, filePath string) (map[string]int, error) {
	blame, err := git.Blame(commit, filePath)
	if err != nil {
		return nil, err
	}

	authors := make(map[string]int)
	for _, line := range blame.Lines {
		authors[line.Author]++
	}

	return authors, nil
}
