package git

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type CommitInfo struct {
	Hash        string
	Message     string
	AuthorName  string
	AuthorEmail string
	CommittedAt time.Time
	Stats       CommitStats
	FileChanges []FileChangeInfo
}

type CommitStats struct {
	TotalAdditions int
	TotalDeletions int
	FilesChanged   int
}

type FileChangeInfo struct {
	ID         string
	FilePath   string
	ChangeType string
	Additions  int
	Deletions  int
	Patch      string
}

type WalkOptions struct {
	Workers    int
	StopAtHash string
	Since      time.Time // Only include commits after this date
	Verbose    bool
	OnProgress func(processed, total int)
	OnCommit   func(CommitInfo) error
}

func WalkCommits(repo *Repository, opts WalkOptions) (int, error) {
	if opts.Workers <= 0 {
		opts.Workers = runtime.NumCPU()
	}

	gitRepo := repo.Git()
	head, err := gitRepo.Head()
	if err != nil {
		return 0, fmt.Errorf("failed to get HEAD: %w", err)
	}

	iter, err := gitRepo.Log(&git.LogOptions{
		From:  head.Hash(),
		Order: git.LogOrderCommitterTime,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create log iterator: %w", err)
	}

	// Collect commits to process
	var commits []*object.Commit
	err = iter.ForEach(func(c *object.Commit) error {
		// Stop at previously ingested commit
		if opts.StopAtHash != "" && c.Hash.String() == opts.StopAtHash {
			return fmt.Errorf("stop")
		}
		// Stop if commit is older than the Since date
		if !opts.Since.IsZero() && c.Author.When.Before(opts.Since) {
			return fmt.Errorf("stop")
		}
		commits = append(commits, c)
		return nil
	})
	if err != nil && err.Error() != "stop" {
		return 0, fmt.Errorf("failed to iterate commits: %w", err)
	}

	if len(commits) == 0 {
		return 0, nil
	}

	// Process commits with worker pool
	jobs := make(chan *object.Commit, len(commits))
	results := make(chan CommitInfo, len(commits))
	errors := make(chan error, opts.Workers)

	var wg sync.WaitGroup
	var processed int64

	// Start workers
	for i := 0; i < opts.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for commit := range jobs {
				info, err := processCommit(commit)
				if err != nil {
					errors <- err
					continue
				}
				results <- info
				current := atomic.AddInt64(&processed, 1)
				if opts.OnProgress != nil {
					opts.OnProgress(int(current), len(commits))
				}
			}
		}()
	}

	// Send jobs
	for _, c := range commits {
		jobs <- c
	}
	close(jobs)

	// Wait for workers to complete
	go func() {
		wg.Wait()
		close(results)
		close(errors)
	}()

	// Collect results
	count := 0
	for info := range results {
		if opts.OnCommit != nil {
			if err := opts.OnCommit(info); err != nil {
				return count, err
			}
		}
		count++
	}

	// Check for errors
	select {
	case err := <-errors:
		if err != nil {
			return count, err
		}
	default:
	}

	return count, nil
}

func processCommit(c *object.Commit) (CommitInfo, error) {
	info := CommitInfo{
		Hash:        c.Hash.String(),
		Message:     c.Message,
		AuthorName:  c.Author.Name,
		AuthorEmail: c.Author.Email,
		CommittedAt: c.Author.When,
	}

	// Get parent for diff calculation
	var parent *object.Commit
	if c.NumParents() > 0 {
		var err error
		parent, err = c.Parent(0)
		if err != nil {
			// Initial commit has no parent, that's okay
			parent = nil
		}
	}

	// Calculate diff
	var parentTree *object.Tree
	if parent != nil {
		var err error
		parentTree, err = parent.Tree()
		if err != nil {
			return info, nil // Continue without diff
		}
	}

	commitTree, err := c.Tree()
	if err != nil {
		return info, nil // Continue without diff
	}

	changes, err := object.DiffTree(parentTree, commitTree)
	if err != nil {
		return info, nil // Continue without diff
	}

	for _, change := range changes {
		filePath := getChangePath(change)
		fc := FileChangeInfo{
			ID:       fmt.Sprintf("%s:%s", c.Hash.String(), filePath),
			FilePath: filePath,
		}

		// Determine change type
		switch {
		case change.From.Name == "":
			fc.ChangeType = "add"
		case change.To.Name == "":
			fc.ChangeType = "delete"
		case change.From.Name != change.To.Name:
			fc.ChangeType = "rename"
		default:
			fc.ChangeType = "modify"
		}

		// Get patch for stats
		patch, err := change.Patch()
		if err == nil && patch != nil {
			for _, fileStat := range patch.Stats() {
				fc.Additions += fileStat.Addition
				fc.Deletions += fileStat.Deletion
			}
			// Store truncated patch to save space
			patchStr := patch.String()
			if len(patchStr) > 10000 {
				patchStr = patchStr[:10000] + "\n... (truncated)"
			}
			fc.Patch = patchStr
		}

		info.Stats.TotalAdditions += fc.Additions
		info.Stats.TotalDeletions += fc.Deletions
		info.Stats.FilesChanged++
		info.FileChanges = append(info.FileChanges, fc)
	}

	return info, nil
}

func getChangePath(change *object.Change) string {
	if change.To.Name != "" {
		return change.To.Name
	}
	return change.From.Name
}
