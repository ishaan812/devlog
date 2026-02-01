package git

import (
	"fmt"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

type Repository struct {
	repo *git.Repository
	path string
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
