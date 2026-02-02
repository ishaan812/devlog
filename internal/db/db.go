package db

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	dbManager     *DBManager
	dbManagerOnce sync.Once
)

// DBManager manages database connections per profile
type DBManager struct {
	connections   map[string]*gorm.DB
	activeProfile string
	mu            sync.RWMutex
}

// getDBManager returns the singleton DBManager instance
func getDBManager() *DBManager {
	dbManagerOnce.Do(func() {
		dbManager = &DBManager{
			connections:   make(map[string]*gorm.DB),
			activeProfile: "default",
		}
	})
	return dbManager
}

// SetActiveProfile sets the active profile for database operations
func SetActiveProfile(name string) {
	m := getDBManager()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeProfile = name
}

// GetActiveProfile returns the active profile name
func GetActiveProfile() string {
	m := getDBManager()
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeProfile
}

// GetDB returns the database connection for the active profile
func GetDB() (*gorm.DB, error) {
	m := getDBManager()
	m.mu.RLock()
	profile := m.activeProfile
	m.mu.RUnlock()
	return GetDBForProfile(profile)
}

// GetDBForProfile returns the database connection for a specific profile
func GetDBForProfile(profile string) (*gorm.DB, error) {
	m := getDBManager()

	m.mu.RLock()
	if db, ok := m.connections[profile]; ok {
		m.mu.RUnlock()
		return db, nil
	}
	m.mu.RUnlock()

	// Need to create connection
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if db, ok := m.connections[profile]; ok {
		return db, nil
	}

	// Create database directory
	dbPath := getProfileDBPath(profile)
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database connection with GORM
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign keys and WAL mode for SQLite
	db.Exec("PRAGMA foreign_keys = ON")
	db.Exec("PRAGMA journal_mode = WAL")

	// Run auto-migration
	if err := autoMigrate(db); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	m.connections[profile] = db
	return db, nil
}

// getProfileDBPath returns the database path for a profile
func getProfileDBPath(profile string) string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".devlog", "profiles", profile, "devlog.db")
}

// autoMigrate runs GORM auto-migration for all models
func autoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&Developer{},
		&Codebase{},
		&Branch{},
		&Commit{},
		&FileChange{},
		&Folder{},
		&FileIndex{},
		&IngestCursor{},
		&FileDependency{},
		&DeveloperCollaboration{},
	)
}

// CloseDB closes the database connection for a profile
func CloseDB(profile string) error {
	m := getDBManager()
	m.mu.Lock()
	defer m.mu.Unlock()

	if db, ok := m.connections[profile]; ok {
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		if err := sqlDB.Close(); err != nil {
			return err
		}
		delete(m.connections, profile)
	}
	return nil
}

// Close closes all database connections
func Close() error {
	m := getDBManager()
	m.mu.Lock()
	defer m.mu.Unlock()

	for profile, db := range m.connections {
		sqlDB, err := db.DB()
		if err != nil {
			continue
		}
		sqlDB.Close()
		delete(m.connections, profile)
	}
	return nil
}

// GetSchemaDescription returns a description of the database schema
func GetSchemaDescription() string {
	return `Tables in the database:

1. developers(id, name, email, is_current_user)
   - Stores developer information, marks current user

2. codebases(id, path, name, summary, tech_stack, default_branch, indexed_at)
   - Stores repository metadata

3. branches(id, codebase_id, name, is_default, summary, status, commit_count, ...)
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
