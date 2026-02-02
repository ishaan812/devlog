package git

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type Repository struct {
	repo *git.Repository
	path string
}

// Commit is an alias for the go-git commit object
type Commit = object.Commit

// BranchInfo holds information about a git branch
type BranchInfo struct {
	Name      string
	Hash      string
	IsDefault bool
	IsRemote  bool
}

func OpenRepo(path string) (*Repository, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	repo, err := git.PlainOpen(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository at %s: %w", absPath, err)
	}

	return &Repository{
		repo: repo,
		path: absPath,
	}, nil
}

func (r *Repository) Path() string {
	return r.path
}

func (r *Repository) Git() *git.Repository {
	return r.repo
}

func (r *Repository) GetUserEmail() (string, error) {
	cfg, err := r.repo.ConfigScoped(config.GlobalScope)
	if err != nil {
		return "", fmt.Errorf("failed to get git config: %w", err)
	}
	return cfg.User.Email, nil
}

func (r *Repository) GetUserName() (string, error) {
	cfg, err := r.repo.ConfigScoped(config.GlobalScope)
	if err != nil {
		return "", fmt.Errorf("failed to get git config: %w", err)
	}
	return cfg.User.Name, nil
}

func (r *Repository) HeadHash() (string, error) {
	head, err := r.repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}
	return head.Hash().String(), nil
}

// CurrentBranch returns the name of the current branch
func (r *Repository) CurrentBranch() (string, error) {
	head, err := r.repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}
	if !head.Name().IsBranch() {
		return "", fmt.Errorf("HEAD is not a branch (detached HEAD)")
	}
	return head.Name().Short(), nil
}

// GetCurrentBranch is an alias for CurrentBranch
func (r *Repository) GetCurrentBranch() (string, error) {
	return r.CurrentBranch()
}

// GetDefaultBranch returns the default branch name (main or master)
func (r *Repository) GetDefaultBranch() (string, error) {
	// Check for common default branch names
	for _, name := range []string{"main", "master"} {
		_, err := r.repo.Reference(plumbing.NewBranchReferenceName(name), true)
		if err == nil {
			return name, nil
		}
	}

	// Try to get from remote HEAD
	remote, err := r.repo.Remote("origin")
	if err == nil {
		refs, err := remote.List(&git.ListOptions{})
		if err == nil {
			for _, ref := range refs {
				if ref.Name() == plumbing.HEAD {
					target := ref.Target()
					if target.IsBranch() {
						return target.Short(), nil
					}
				}
			}
		}
	}

	// Default to main
	return "main", nil
}

// ListBranches returns all local branches
func (r *Repository) ListBranches() ([]BranchInfo, error) {
	iter, err := r.repo.Branches()
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}

	defaultBranch, _ := r.GetDefaultBranch()
	var branches []BranchInfo

	err = iter.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().Short()
		branches = append(branches, BranchInfo{
			Name:      name,
			Hash:      ref.Hash().String(),
			IsDefault: name == defaultBranch,
			IsRemote:  false,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Sort: default branch first, then alphabetically
	sort.Slice(branches, func(i, j int) bool {
		if branches[i].IsDefault != branches[j].IsDefault {
			return branches[i].IsDefault
		}
		return branches[i].Name < branches[j].Name
	})

	return branches, nil
}

// GetBranchHash returns the commit hash for a branch
func (r *Repository) GetBranchHash(branchName string) (string, error) {
	ref, err := r.repo.Reference(plumbing.NewBranchReferenceName(branchName), true)
	if err != nil {
		return "", fmt.Errorf("branch '%s' not found: %w", branchName, err)
	}
	return ref.Hash().String(), nil
}

// GetMergeBase finds the common ancestor between two branches
func (r *Repository) GetMergeBase(branch1, branch2 string) (string, error) {
	hash1, err := r.GetBranchHash(branch1)
	if err != nil {
		return "", err
	}
	hash2, err := r.GetBranchHash(branch2)
	if err != nil {
		return "", err
	}

	commit1, err := r.repo.CommitObject(plumbing.NewHash(hash1))
	if err != nil {
		return "", err
	}
	commit2, err := r.repo.CommitObject(plumbing.NewHash(hash2))
	if err != nil {
		return "", err
	}

	// Find common ancestors using commit history
	ancestors1 := make(map[string]bool)

	// Collect all ancestors of commit1
	iter1, err := r.repo.Log(&git.LogOptions{From: commit1.Hash})
	if err != nil {
		return "", err
	}
	err = iter1.ForEach(func(c *object.Commit) error {
		ancestors1[c.Hash.String()] = true
		return nil
	})
	if err != nil {
		return "", err
	}

	// Find first ancestor of commit2 that's also in commit1's ancestry
	iter2, err := r.repo.Log(&git.LogOptions{From: commit2.Hash})
	if err != nil {
		return "", err
	}

	var mergeBase string
	err = iter2.ForEach(func(c *object.Commit) error {
		if ancestors1[c.Hash.String()] {
			mergeBase = c.Hash.String()
			return fmt.Errorf("found") // Stop iteration
		}
		return nil
	})
	if err != nil && err.Error() != "found" {
		return "", err
	}

	if mergeBase == "" {
		return "", fmt.Errorf("no common ancestor found between %s and %s", branch1, branch2)
	}

	return mergeBase, nil
}

// GetCommitsOnBranch returns commits unique to a branch (not on base branch)
func (r *Repository) GetCommitsOnBranch(branchName, baseBranch string) ([]string, error) {
	branchHash, err := r.GetBranchHash(branchName)
	if err != nil {
		return nil, err
	}

	// If same as base branch, return empty
	if branchName == baseBranch {
		return nil, nil
	}

	// Find merge base
	mergeBase, err := r.GetMergeBase(branchName, baseBranch)
	if err != nil {
		// If no merge base, return all commits on the branch
		return r.getAllCommitHashes(branchHash, "")
	}

	// Get commits from branch head to merge base
	return r.getAllCommitHashes(branchHash, mergeBase)
}

// getAllCommitHashes returns all commit hashes from start to stop (exclusive)
func (r *Repository) getAllCommitHashes(startHash, stopHash string) ([]string, error) {
	iter, err := r.repo.Log(&git.LogOptions{
		From: plumbing.NewHash(startHash),
	})
	if err != nil {
		return nil, err
	}

	var hashes []string
	err = iter.ForEach(func(c *object.Commit) error {
		hash := c.Hash.String()
		if stopHash != "" && hash == stopHash {
			return fmt.Errorf("stop")
		}
		hashes = append(hashes, hash)
		return nil
	})
	if err != nil && err.Error() != "stop" {
		return nil, err
	}

	return hashes, nil
}

// IsBranchMerged checks if a branch has been merged into the base branch
func (r *Repository) IsBranchMerged(branchName, baseBranch string) (bool, error) {
	branchHash, err := r.GetBranchHash(branchName)
	if err != nil {
		return false, err
	}

	baseHash, err := r.GetBranchHash(baseBranch)
	if err != nil {
		return false, err
	}

	// Check if branch commit is an ancestor of base
	baseCommit, err := r.repo.CommitObject(plumbing.NewHash(baseHash))
	if err != nil {
		return false, err
	}

	iter, err := r.repo.Log(&git.LogOptions{From: baseCommit.Hash})
	if err != nil {
		return false, err
	}

	merged := false
	err = iter.ForEach(func(c *object.Commit) error {
		if c.Hash.String() == branchHash {
			merged = true
			return fmt.Errorf("found")
		}
		return nil
	})
	if err != nil && err.Error() != "found" {
		return false, err
	}

	return merged, nil
}

// BranchExists checks if a branch exists
func (r *Repository) BranchExists(name string) bool {
	_, err := r.repo.Reference(plumbing.NewBranchReferenceName(name), true)
	return err == nil
}

// GetCommit retrieves a commit by hash
func (r *Repository) GetCommit(hash string) (*object.Commit, error) {
	return r.repo.CommitObject(plumbing.NewHash(hash))
}

// DetectBranchStatus determines if a branch is active, merged, or stale
func (r *Repository) DetectBranchStatus(branchName, defaultBranch string) string {
	if branchName == defaultBranch {
		return "active"
	}

	merged, err := r.IsBranchMerged(branchName, defaultBranch)
	if err == nil && merged {
		return "merged"
	}

	// Check if branch has recent activity (within last 30 days)
	hash, err := r.GetBranchHash(branchName)
	if err != nil {
		return "unknown"
	}

	commit, err := r.GetCommit(hash)
	if err != nil {
		return "unknown"
	}

	// Consider stale if no commits in 30 days
	// For now, just return active
	_ = commit
	return "active"
}

// ParseBranchName normalizes branch name (removes refs/heads/ prefix if present)
func ParseBranchName(name string) string {
	name = strings.TrimPrefix(name, "refs/heads/")
	name = strings.TrimPrefix(name, "refs/remotes/origin/")
	return name
}
