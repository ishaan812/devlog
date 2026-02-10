package db

import (
	"database/sql"
	"encoding/json"
	"time"
)

// Developer represents a git author
type Developer struct {
	ID            string
	Name          string
	Email         string
	IsCurrentUser bool
}

// Codebase represents an indexed repository
type Codebase struct {
	ID              string
	Path            string
	Name            string
	Summary         string
	TechStack       map[string]int
	DefaultBranch   string
	IndexedAt       time.Time
	ProjectContext  string // Higher-level summary of features being worked on across branches
	LongtermContext string // Long-term goals and ongoing initiatives
}

// Branch represents a git branch
type Branch struct {
	ID              string
	CodebaseID      string
	Name            string
	IsDefault       bool
	BaseBranch      string
	Summary         string
	Story           string
	Status          string
	FirstCommitHash string
	LastCommitHash  string
	CommitCount     int
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ContextSummary  string // Day-by-day progress context for multi-day feature tracking
}

// Commit represents a git commit
type Commit struct {
	ID                string
	Hash              string
	CodebaseID        string
	BranchID          string
	AuthorEmail       string
	Message           string
	Summary           string
	CommittedAt       time.Time
	Stats             map[string]any
	IsUserCommit      bool
	IsOnDefaultBranch bool
}

// FileChange represents a file change within a commit
type FileChange struct {
	ID         string
	CommitID   string
	FilePath   string
	ChangeType string
	Additions  int
	Deletions  int
	Patch      string
}

// Folder represents a folder in the codebase
type Folder struct {
	ID         string
	CodebaseID string
	Path       string
	Name       string
	Depth      int
	ParentPath string
	Summary    string
	Purpose    string
	FileCount  int
	IndexedAt  time.Time
}

// FileIndex represents an indexed file
type FileIndex struct {
	ID           string
	CodebaseID   string
	FolderID     string
	Path         string
	Name         string
	Extension    string
	Language     string
	SizeBytes    int64
	LineCount    int
	Summary      string
	Purpose      string
	KeyExports   []string
	Dependencies []string
	ContentHash  string
	IndexedAt    time.Time
}

// IngestCursor tracks ingestion state per branch
type IngestCursor struct {
	ID             string
	CodebaseID     string
	BranchName     string
	LastCommitHash string
	UpdatedAt      time.Time
}

// WorklogEntry represents a cached LLM-generated worklog entry
type WorklogEntry struct {
	ID           string
	CodebaseID   string
	ProfileName  string
	EntryDate    time.Time
	BranchID     string
	BranchName   string
	EntryType    string // "day_updates" or "branch_summary"
	GroupBy      string // "date" or "branch"
	Content      string
	CommitCount  int
	Additions    int
	Deletions    int
	CommitHashes string // sorted, comma-joined hashes for invalidation
	CreatedAt    time.Time
}

// WorklogDateInfo holds aggregated info for a single date's cached worklogs
type WorklogDateInfo struct {
	EntryDate   time.Time
	EntryCount  int
	CommitCount int
	Additions   int
	Deletions   int
}

// JSON is a type alias for map[string]any used for JSON columns
type JSON = map[string]any

// Helper functions for JSON serialization

// NullString converts a string to sql.NullString
func NullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// NullTime converts a time.Time to sql.NullTime
func NullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

// ToJSON converts a value to JSON string
func ToJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// FromJSON parses a JSON string into a map
func FromJSON(s string) map[string]any {
	if s == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	return m
}

// FromJSONIntMap parses a JSON string into a map[string]int
func FromJSONIntMap(s string) map[string]int {
	if s == "" {
		return nil
	}
	var m map[string]int
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	return m
}

// FromJSONStringSlice parses a JSON string into a []string
func FromJSONStringSlice(s string) []string {
	if s == "" {
		return nil
	}
	var slice []string
	if err := json.Unmarshal([]byte(s), &slice); err != nil {
		return nil
	}
	return slice
}

// convertToIntMap converts DuckDB JSON (which comes as native Go types) to map[string]int
func convertToIntMap(v any) map[string]int {
	if v == nil {
		return nil
	}

	// DuckDB returns JSON as map[string]any
	switch m := v.(type) {
	case map[string]any:
		result := make(map[string]int)
		for k, val := range m {
			switch n := val.(type) {
			case float64:
				result[k] = int(n)
			case int64:
				result[k] = int(n)
			case int:
				result[k] = n
			}
		}
		return result
	case string:
		// Fallback to string parsing
		return FromJSONIntMap(m)
	}
	return nil
}

// convertToMap converts DuckDB JSON to map[string]any
func convertToMap(v any) map[string]any {
	if v == nil {
		return nil
	}

	switch m := v.(type) {
	case map[string]any:
		return m
	case string:
		return FromJSON(m)
	}
	return nil
}

// convertToStringSlice converts DuckDB JSON array to []string
func convertToStringSlice(v any) []string {
	if v == nil {
		return nil
	}

	switch s := v.(type) {
	case []any:
		result := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	case string:
		return FromJSONStringSlice(s)
	}
	return nil
}
