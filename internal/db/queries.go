package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type Developer struct {
	ID    string
	Name  string
	Email string
}

type Commit struct {
	Hash        string
	RepoPath    string
	Message     string
	AuthorEmail string
	CommittedAt time.Time
	Stats       map[string]interface{}
}

type FileChange struct {
	ID         string
	CommitHash string
	FilePath   string
	ChangeType string
	Additions  int
	Deletions  int
	Patch      string
}

func InsertDeveloper(db *sql.DB, dev Developer) error {
	_, err := db.Exec(`
		INSERT INTO developer (id, name, email)
		VALUES (?, ?, ?)
		ON CONFLICT (email) DO UPDATE SET name = excluded.name
	`, dev.ID, dev.Name, dev.Email)
	return err
}

func InsertCommit(db *sql.DB, c Commit) error {
	statsJSON, err := json.Marshal(c.Stats)
	if err != nil {
		statsJSON = []byte("{}")
	}

	_, err = db.Exec(`
		INSERT INTO commit (hash, repo_path, message, author_email, committed_at, stats)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (hash) DO NOTHING
	`, c.Hash, c.RepoPath, c.Message, c.AuthorEmail, c.CommittedAt, string(statsJSON))
	return err
}

func InsertFileChange(db *sql.DB, fc FileChange) error {
	_, err := db.Exec(`
		INSERT INTO file_change (id, commit_hash, file_path, change_type, additions, deletions, patch)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING
	`, fc.ID, fc.CommitHash, fc.FilePath, fc.ChangeType, fc.Additions, fc.Deletions, fc.Patch)
	return err
}

func GetLastScannedHash(db *sql.DB, repoPath string) (string, error) {
	var hash string
	err := db.QueryRow(`
		SELECT last_hash FROM ingest_cursor WHERE repo_path = ?
	`, repoPath).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return hash, err
}

func UpdateCursor(db *sql.DB, repoPath, hash string) error {
	now := time.Now()
	_, err := db.Exec(`
		INSERT INTO ingest_cursor (repo_path, last_hash, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT (repo_path) DO UPDATE SET
			last_hash = excluded.last_hash,
			updated_at = excluded.updated_at
	`, repoPath, hash, now)
	return err
}

func CommitExists(db *sql.DB, hash string) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM commit WHERE hash = ?`, hash).Scan(&count)
	return count > 0, err
}

func GetCommitCount(db *sql.DB, repoPath string) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM commit WHERE repo_path = ?`, repoPath).Scan(&count)
	return count, err
}

func GetFileChangeCount(db *sql.DB, repoPath string) (int, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM file_change fc
		JOIN commit c ON fc.commit_hash = c.hash
		WHERE c.repo_path = ?
	`, repoPath).Scan(&count)
	return count, err
}

func ExecuteQuery(db *sql.DB, query string) ([]map[string]interface{}, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	return results, rows.Err()
}
