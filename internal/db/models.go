package db

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// JSON type for storing JSON data in SQLite
type JSON map[string]interface{}

func (j JSON) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

func (j *JSON) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, j)
}

// StringSlice type for storing string arrays in SQLite
type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

func (s *StringSlice) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, s)
}

// IntMap type for storing map[string]int in SQLite
type IntMap map[string]int

func (m IntMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

func (m *IntMap) Scan(value interface{}) error {
	if value == nil {
		*m = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, m)
}

// Developer represents a git author
type Developer struct {
	ID            string `gorm:"primaryKey"`
	Name          string `gorm:"not null"`
	Email         string `gorm:"uniqueIndex;not null"`
	IsCurrentUser bool   `gorm:"default:false"`

	Commits []Commit `gorm:"foreignKey:AuthorEmail;references:Email"`
}

// Codebase represents an indexed repository
type Codebase struct {
	ID            string `gorm:"primaryKey"`
	Path          string `gorm:"uniqueIndex;not null"`
	Name          string `gorm:"not null"`
	Summary       string
	TechStack     IntMap `gorm:"type:json"`
	DefaultBranch string
	IndexedAt     time.Time

	Branches      []Branch      `gorm:"foreignKey:CodebaseID"`
	Commits       []Commit      `gorm:"foreignKey:CodebaseID"`
	Folders       []Folder      `gorm:"foreignKey:CodebaseID"`
	Files         []FileIndex   `gorm:"foreignKey:CodebaseID"`
	IngestCursors []IngestCursor `gorm:"foreignKey:CodebaseID"`
}

// Branch represents a git branch
type Branch struct {
	ID              string `gorm:"primaryKey"`
	CodebaseID      string `gorm:"index;not null"`
	Name            string `gorm:"not null"`
	IsDefault       bool   `gorm:"default:false"`
	BaseBranch      string
	Summary         string
	Story           string // User-provided description of work on this branch
	Status          string `gorm:"default:'active'"`
	FirstCommitHash string
	LastCommitHash  string
	CommitCount     int `gorm:"default:0"`
	CreatedAt       time.Time
	UpdatedAt       time.Time

	Codebase Codebase `gorm:"foreignKey:CodebaseID"`
	Commits  []Commit `gorm:"foreignKey:BranchID"`
}

func (Branch) TableName() string {
	return "branches"
}

// BeforeCreate sets default values
func (b *Branch) BeforeCreate(tx *gorm.DB) error {
	if b.Status == "" {
		b.Status = "active"
	}
	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now()
	}
	b.UpdatedAt = time.Now()
	return nil
}

// Commit represents a git commit
type Commit struct {
	ID                string `gorm:"primaryKey"`
	Hash              string `gorm:"index;not null"`
	CodebaseID        string `gorm:"index;not null"`
	BranchID          string `gorm:"index"`
	AuthorEmail       string `gorm:"index;not null"`
	Message           string `gorm:"not null"`
	Summary           string
	CommittedAt       time.Time `gorm:"index;not null"`
	Stats             JSON      `gorm:"type:json"`
	IsUserCommit      bool      `gorm:"index;default:false"`
	IsOnDefaultBranch bool      `gorm:"default:false"`

	Codebase    Codebase     `gorm:"foreignKey:CodebaseID"`
	Branch      Branch       `gorm:"foreignKey:BranchID"`
	Author      Developer    `gorm:"foreignKey:AuthorEmail;references:Email"`
	FileChanges []FileChange `gorm:"foreignKey:CommitID"`
}

func (Commit) TableName() string {
	return "commits"
}

// FileChange represents a file change within a commit
type FileChange struct {
	ID         string `gorm:"primaryKey"`
	CommitID   string `gorm:"index;not null"`
	FilePath   string `gorm:"index;not null"`
	ChangeType string `gorm:"not null"`
	Additions  int    `gorm:"default:0"`
	Deletions  int    `gorm:"default:0"`
	Patch      string

	Commit Commit `gorm:"foreignKey:CommitID"`
}

func (FileChange) TableName() string {
	return "file_changes"
}

// Folder represents a folder in the codebase
type Folder struct {
	ID         string `gorm:"primaryKey"`
	CodebaseID string `gorm:"index;not null"`
	Path       string `gorm:"not null"`
	Name       string `gorm:"not null"`
	Depth      int    `gorm:"not null"`
	ParentPath string
	Summary    string
	Purpose    string
	FileCount  int `gorm:"default:0"`
	IndexedAt  time.Time

	Codebase Codebase    `gorm:"foreignKey:CodebaseID"`
	Files    []FileIndex `gorm:"foreignKey:FolderID"`
}

func (Folder) TableName() string {
	return "folders"
}

// FileIndex represents an indexed file
type FileIndex struct {
	ID           string `gorm:"primaryKey"`
	CodebaseID   string `gorm:"index;not null"`
	FolderID     string `gorm:"index"`
	Path         string `gorm:"not null"`
	Name         string `gorm:"not null"`
	Extension    string
	Language     string `gorm:"index"`
	SizeBytes    int64
	LineCount    int
	Summary      string
	Purpose      string
	KeyExports   StringSlice `gorm:"type:json"`
	Dependencies StringSlice `gorm:"type:json"`
	ContentHash  string
	IndexedAt    time.Time

	Codebase Codebase `gorm:"foreignKey:CodebaseID"`
	Folder   Folder   `gorm:"foreignKey:FolderID"`
}

func (FileIndex) TableName() string {
	return "file_indexes"
}

// IngestCursor tracks ingestion state per branch
type IngestCursor struct {
	ID             string `gorm:"primaryKey"`
	CodebaseID     string `gorm:"index;not null"`
	BranchName     string `gorm:"not null"`
	LastCommitHash string `gorm:"not null"`
	UpdatedAt      time.Time

	Codebase Codebase `gorm:"foreignKey:CodebaseID"`
}

func (IngestCursor) TableName() string {
	return "ingest_cursors"
}

// FileDependency represents a dependency between files
type FileDependency struct {
	ID             string `gorm:"primaryKey"`
	SourceFileID   string `gorm:"index;not null"`
	TargetFileID   string `gorm:"index;not null"`
	DependencyType string `gorm:"not null"`

	SourceFile FileIndex `gorm:"foreignKey:SourceFileID"`
	TargetFile FileIndex `gorm:"foreignKey:TargetFileID"`
}

func (FileDependency) TableName() string {
	return "file_dependencies"
}

// DeveloperCollaboration tracks collaboration between developers
type DeveloperCollaboration struct {
	Developer1Email   string `gorm:"primaryKey"`
	Developer2Email   string `gorm:"primaryKey"`
	SharedFiles       int    `gorm:"default:0"`
	SharedCommits     int    `gorm:"default:0"`
	LastCollaboration time.Time
}

func (DeveloperCollaboration) TableName() string {
	return "developer_collaborations"
}
