package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "github.com/marcboeker/go-duckdb"

	"github.com/ishaan812/devlog/internal/config"
)

var (
	connections   = make(map[string]*sql.DB)
	activeProfile = "default"
	mu            sync.RWMutex
)

// SetActiveProfile sets the active profile for database operations
func SetActiveProfile(profile string) {
	mu.Lock()
	defer mu.Unlock()
	activeProfile = profile
}

// GetActiveProfile returns the current active profile
func GetActiveProfile() string {
	mu.RLock()
	defer mu.RUnlock()
	return activeProfile
}

// GetDB returns the database connection for the active profile
func GetDB() (*sql.DB, error) {
	mu.RLock()
	profile := activeProfile
	mu.RUnlock()
	return GetDBForProfile(profile)
}

// GetDBForProfile returns a database connection for a specific profile
func GetDBForProfile(profile string) (*sql.DB, error) {
	mu.Lock()
	defer mu.Unlock()

	// Check if connection already exists
	if db, ok := connections[profile]; ok {
		// Verify connection is still alive
		if err := db.Ping(); err == nil {
			return db, nil
		}
		// Connection is dead, close and recreate
		db.Close()
		delete(connections, profile)
	}

	// Get database path
	dbPath := config.GetProfileDBPath(profile)

	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open DuckDB connection with access mode setting
	// Using read_write mode with access_mode=automatic
	connStr := fmt.Sprintf("%s?access_mode=read_write", dbPath)
	db, err := sql.Open("duckdb", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(1) // DuckDB works best with single connection
	db.SetMaxIdleConns(1)

	// Force checkpoint to reduce WAL usage
	db.Exec("CHECKPOINT")

	// Initialize schema
	if err := initializeSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Cache connection
	connections[profile] = db

	return db, nil
}

// initializeSchema creates all tables if they don't exist
func initializeSchema(db *sql.DB) error {
	_, err := db.Exec(Schema)
	return err
}

// CloseDB closes the database connection for a specific profile
func CloseDB(profile string) {
	mu.Lock()
	defer mu.Unlock()

	if db, ok := connections[profile]; ok {
		db.Close()
		delete(connections, profile)
	}
}

// CloseAllDBs closes all database connections
func CloseAllDBs() {
	mu.Lock()
	defer mu.Unlock()

	for profile, db := range connections {
		db.Close()
		delete(connections, profile)
	}
}

// Transaction executes a function within a transaction
func Transaction(db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

// GetSchemaDescription returns a description of the database schema
func GetSchemaDescription() string {
	return `Tables in the database:

1. developers(id, name, email, is_current_user)
   - Stores developer information, marks current user

2. codebases(id, path, name, summary, tech_stack, default_branch, indexed_at)
   - Stores repository metadata

3. branches(id, codebase_id, name, is_default, summary, story, status, commit_count, ...)
   - Stores branch information with stories/descriptions

4. commits(id, hash, codebase_id, branch_id, author_email, message, summary, is_user_commit, ...)
   - Stores commits with branch association, user commits get summaries

5. file_changes(id, commit_id, file_path, change_type, additions, deletions, patch)
   - Stores individual file changes within commits

6. folders(id, codebase_id, path, name, depth, summary, purpose, file_count)
   - Stores folder summaries

7. file_indexes(id, codebase_id, path, name, language, summary, purpose)
   - Stores file summaries for semantic search

8. ingest_cursors(id, codebase_id, branch_name, last_commit_hash)
   - Tracks ingestion state per branch for incremental updates`
}
