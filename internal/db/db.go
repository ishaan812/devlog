package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "github.com/marcboeker/go-duckdb" // DuckDB driver

	"github.com/ishaan812/devlog/internal/config"
)

// Manager handles database connections and repositories.
type Manager struct {
	mu            sync.RWMutex
	connections   map[string]*sql.DB
	repositories  map[string]*SQLRepository
	activeProfile string
}

var globalManager = &Manager{
	connections:   make(map[string]*sql.DB),
	repositories:  make(map[string]*SQLRepository),
	activeProfile: "default",
}

// NewManager creates a new connection manager.
func NewManager() *Manager {
	return &Manager{
		connections:   make(map[string]*sql.DB),
		repositories:  make(map[string]*SQLRepository),
		activeProfile: "default",
	}
}

// SetActiveProfile sets the active profile.
func (m *Manager) SetActiveProfile(profile string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeProfile = profile
}

// GetActiveProfile returns the active profile.
func (m *Manager) GetActiveProfile() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeProfile
}

// GetDB returns the database for the active profile.
func (m *Manager) GetDB() (*sql.DB, error) {
	m.mu.RLock()
	profile := m.activeProfile
	m.mu.RUnlock()
	return m.GetDBForProfile(profile)
}

// GetRepository returns the repository for the active profile.
func (m *Manager) GetRepository() (*SQLRepository, error) {
	m.mu.RLock()
	profile := m.activeProfile
	m.mu.RUnlock()
	return m.GetRepositoryForProfile(profile)
}

// GetDBForProfile returns a database for a specific profile.
func (m *Manager) GetDBForProfile(profile string) (*sql.DB, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if db, ok := m.connections[profile]; ok {
		if err := db.Ping(); err == nil {
			return db, nil
		}
		db.Close()
		delete(m.connections, profile)
		delete(m.repositories, profile)
	}
	dbPath := config.GetProfileDBPath(profile)
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	connStr := fmt.Sprintf("%s?access_mode=read_write", dbPath)
	db, err := sql.Open("duckdb", connStr)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	_, _ = db.Exec("CHECKPOINT")
	if err := initializeSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize schema: %w", err)
	}
	m.connections[profile] = db
	return db, nil
}

// GetRepositoryForProfile returns the repository for a specific profile.
func (m *Manager) GetRepositoryForProfile(profile string) (*SQLRepository, error) {
	m.mu.Lock()
	if repo, ok := m.repositories[profile]; ok {
		if err := repo.db.Ping(); err == nil {
			m.mu.Unlock()
			return repo, nil
		}
		delete(m.repositories, profile)
		delete(m.connections, profile)
	}
	m.mu.Unlock()
	db, err := m.GetDBForProfile(profile)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	repo := NewRepository(db)
	m.repositories[profile] = repo
	return repo, nil
}

// CloseDB closes the database for a profile.
func (m *Manager) CloseDB(profile string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if db, ok := m.connections[profile]; ok {
		db.Close()
		delete(m.connections, profile)
		delete(m.repositories, profile)
	}
}

// CloseAllDBs closes all databases.
func (m *Manager) CloseAllDBs() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for profile, db := range m.connections {
		db.Close()
		delete(m.connections, profile)
		delete(m.repositories, profile)
	}
}

func initializeSchema(db *sql.DB) error {
	_, err := db.Exec(Schema)
	if err != nil {
		return fmt.Errorf("execute schema: %w", err)
	}
	return nil
}

// Transaction executes a function within a transaction.
func Transaction(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return errors.Join(fmt.Errorf("rollback failed: %w", rbErr), err)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// GetSchemaDescription returns the database schema description.
func GetSchemaDescription() string {
	return `Tables:
1. developers(id, name, email, is_current_user)
2. codebases(id, path, name, summary, tech_stack, default_branch, indexed_at)
3. branches(id, codebase_id, name, is_default, summary, story, status, commit_count, ...)
4. commits(id, hash, codebase_id, branch_id, author_email, message, summary, is_user_commit, ...)
5. file_changes(id, commit_id, file_path, change_type, additions, deletions, patch)
6. folders(id, codebase_id, path, name, depth, summary, purpose, file_count)
7. file_indexes(id, codebase_id, path, name, language, summary, purpose)
8. ingest_cursors(id, codebase_id, branch_name, last_commit_hash)`
}

// Global functions for convenience.

// SetActiveProfile sets the active profile on the global manager.
func SetActiveProfile(profile string) { globalManager.SetActiveProfile(profile) }

// GetActiveProfile returns the active profile from the global manager.
func GetActiveProfile() string { return globalManager.GetActiveProfile() }

// GetDB returns the database from the global manager.
func GetDB() (*sql.DB, error) { return globalManager.GetDB() }

// GetDBForProfile returns a database for a profile from the global manager.
func GetDBForProfile(profile string) (*sql.DB, error) { return globalManager.GetDBForProfile(profile) }

// GetRepository returns the repository from the global manager.
func GetRepository() (*SQLRepository, error) { return globalManager.GetRepository() }

// GetRepositoryForProfile returns a repository for a profile from the global manager.
func GetRepositoryForProfile(profile string) (*SQLRepository, error) {
	return globalManager.GetRepositoryForProfile(profile)
}

// CloseDB closes the database for a profile on the global manager.
func CloseDB(profile string) { globalManager.CloseDB(profile) }

// CloseAllDBs closes all databases on the global manager.
func CloseAllDBs() { globalManager.CloseAllDBs() }
