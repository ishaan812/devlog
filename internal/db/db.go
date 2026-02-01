package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ishaan812/devlog/internal/config"

	_ "github.com/marcboeker/go-duckdb"
)

// DBManager manages database connections for multiple profiles
type DBManager struct {
	connections   map[string]*sql.DB
	activeProfile string
	mu            sync.RWMutex
}

var (
	manager     *DBManager
	managerOnce sync.Once
	// Legacy support: custom path override
	customDBPath string
)

// getManager returns the singleton DBManager instance
func getManager() *DBManager {
	managerOnce.Do(func() {
		manager = &DBManager{
			connections:   make(map[string]*sql.DB),
			activeProfile: "default",
		}
	})
	return manager
}

// SetDBPath sets a custom database path (legacy support, overrides profile)
func SetDBPath(path string) {
	customDBPath = path
}

// SetActiveProfile sets the active profile for database operations
func SetActiveProfile(name string) {
	m := getManager()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeProfile = name
}

// GetActiveProfile returns the current active profile name
func GetActiveProfile() string {
	m := getManager()
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeProfile
}

// GetDB returns the database connection for the active profile
func GetDB() (*sql.DB, error) {
	// If custom path is set, use legacy behavior
	if customDBPath != "" {
		return GetDBForPath(customDBPath)
	}

	m := getManager()
	m.mu.RLock()
	profile := m.activeProfile
	m.mu.RUnlock()

	return GetDBForProfile(profile)
}

// GetDBForProfile returns the database connection for a specific profile
func GetDBForProfile(name string) (*sql.DB, error) {
	m := getManager()

	// Check if already connected
	m.mu.RLock()
	if db, exists := m.connections[name]; exists {
		m.mu.RUnlock()
		return db, nil
	}
	m.mu.RUnlock()

	// Need to create connection
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if db, exists := m.connections[name]; exists {
		return db, nil
	}

	// Get profile DB path
	dbPath := config.GetProfileDBPath(name)

	// Ensure directory exists
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create profile directory: %w", err)
	}

	// Initialize the database
	db, err := initDB(dbPath)
	if err != nil {
		return nil, err
	}

	m.connections[name] = db
	return db, nil
}

// GetDBForPath returns a database connection for a specific path
func GetDBForPath(path string) (*sql.DB, error) {
	m := getManager()

	// Use path as key
	m.mu.RLock()
	if db, exists := m.connections[path]; exists {
		m.mu.RUnlock()
		return db, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check
	if db, exists := m.connections[path]; exists {
		return db, nil
	}

	// Ensure directory exists
	dbDir := filepath.Dir(path)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := initDB(path)
	if err != nil {
		return nil, err
	}

	m.connections[path] = db
	return db, nil
}

// initDB initializes a DuckDB connection at the given path
func initDB(path string) (*sql.DB, error) {
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open DuckDB: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping DuckDB: %w", err)
	}

	// Install and load DuckPGQ extension (optional)
	if _, err := db.Exec("INSTALL duckpgq FROM community"); err != nil {
		// Extension might already be installed, continue
	}
	if _, err := db.Exec("LOAD duckpgq"); err != nil {
		// Extension might not be available, continue without it
	}

	if err := CreateSchema(db); err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	return db, nil
}

// InitDB initializes a database at the given path (legacy compatibility)
func InitDB(customPath string) (*sql.DB, error) {
	path := customPath
	if path == "" {
		// Use default profile path
		path = config.GetProfileDBPath("default")
	}
	return GetDBForPath(path)
}

// Close closes all database connections
func Close() error {
	m := getManager()
	m.mu.Lock()
	defer m.mu.Unlock()

	var lastErr error
	for name, db := range m.connections {
		if err := db.Close(); err != nil {
			lastErr = err
		}
		delete(m.connections, name)
	}

	return lastErr
}

// CloseProfile closes the database connection for a specific profile
func CloseProfile(name string) error {
	m := getManager()
	m.mu.Lock()
	defer m.mu.Unlock()

	if db, exists := m.connections[name]; exists {
		err := db.Close()
		delete(m.connections, name)
		return err
	}
	return nil
}
