package db

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ==================== Developer Repository ====================

// UpsertDeveloper creates or updates a developer
func UpsertDeveloper(db *gorm.DB, dev *Developer) error {
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "email"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "is_current_user"}),
	}).Create(dev).Error
}

// GetDeveloperByEmail retrieves a developer by email
func GetDeveloperByEmail(db *gorm.DB, email string) (*Developer, error) {
	var dev Developer
	err := db.Where("email = ?", email).First(&dev).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &dev, err
}

// SetCurrentUser marks a developer as the current user
func SetCurrentUser(db *gorm.DB, email string) error {
	// Unset all current users
	if err := db.Model(&Developer{}).Where("is_current_user = ?", true).Update("is_current_user", false).Error; err != nil {
		return err
	}
	// Set the specified user
	return db.Model(&Developer{}).Where("email = ?", email).Update("is_current_user", true).Error
}

// GetCurrentUser retrieves the current user
func GetCurrentUser(db *gorm.DB) (*Developer, error) {
	var dev Developer
	err := db.Where("is_current_user = ?", true).First(&dev).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &dev, err
}

// ==================== Codebase Repository ====================

// UpsertCodebase creates or updates a codebase
func UpsertCodebase(db *gorm.DB, codebase *Codebase) error {
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "path"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "summary", "tech_stack", "default_branch", "indexed_at"}),
	}).Create(codebase).Error
}

// GetCodebaseByPath retrieves a codebase by path
func GetCodebaseByPath(db *gorm.DB, path string) (*Codebase, error) {
	var codebase Codebase
	err := db.Where("path = ?", path).First(&codebase).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &codebase, err
}

// GetCodebaseByID retrieves a codebase by ID
func GetCodebaseByID(db *gorm.DB, id string) (*Codebase, error) {
	var codebase Codebase
	err := db.First(&codebase, "id = ?", id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &codebase, err
}

// GetAllCodebases retrieves all codebases
func GetAllCodebases(db *gorm.DB) ([]Codebase, error) {
	var codebases []Codebase
	err := db.Order("name").Find(&codebases).Error
	return codebases, err
}

// ==================== Branch Repository ====================

// UpsertBranch creates or updates a branch
func UpsertBranch(db *gorm.DB, branch *Branch) error {
	// Check for existing branch
	var existing Branch
	err := db.Where("codebase_id = ? AND name = ?", branch.CodebaseID, branch.Name).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		// Create new
		if branch.ID == "" {
			branch.ID = uuid.New().String()
		}
		return db.Create(branch).Error
	}
	if err != nil {
		return err
	}
	// Update existing
	branch.ID = existing.ID
	branch.CreatedAt = existing.CreatedAt
	return db.Save(branch).Error
}

// GetBranch retrieves a branch by codebase and name
func GetBranch(db *gorm.DB, codebaseID, name string) (*Branch, error) {
	var branch Branch
	err := db.Where("codebase_id = ? AND name = ?", codebaseID, name).First(&branch).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &branch, err
}

// GetBranchByID retrieves a branch by ID
func GetBranchByID(db *gorm.DB, id string) (*Branch, error) {
	var branch Branch
	err := db.First(&branch, "id = ?", id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &branch, err
}

// GetBranchesByCodebase retrieves all branches for a codebase
func GetBranchesByCodebase(db *gorm.DB, codebaseID string) ([]Branch, error) {
	var branches []Branch
	err := db.Where("codebase_id = ?", codebaseID).
		Order("is_default DESC, updated_at DESC").
		Find(&branches).Error
	return branches, err
}

// GetActiveBranches retrieves active branches for a codebase
func GetActiveBranches(db *gorm.DB, codebaseID string) ([]Branch, error) {
	var branches []Branch
	err := db.Where("codebase_id = ? AND status = ?", codebaseID, "active").
		Order("is_default DESC, updated_at DESC").
		Find(&branches).Error
	return branches, err
}

// UpdateBranchSummary updates the summary for a branch
func UpdateBranchSummary(db *gorm.DB, branchID, summary string) error {
	return db.Model(&Branch{}).Where("id = ?", branchID).
		Updates(map[string]interface{}{"summary": summary, "updated_at": time.Now()}).Error
}

// GetBranchCommits retrieves recent commits for a branch
func GetBranchCommits(db *gorm.DB, branchID string, limit int) ([]Commit, error) {
	var commits []Commit
	err := db.Where("branch_id = ?", branchID).
		Order("committed_at DESC").
		Limit(limit).
		Find(&commits).Error
	return commits, err
}

// ClearDefaultBranch clears the default flag on all branches for a codebase
func ClearDefaultBranch(db *gorm.DB, codebaseID string) error {
	return db.Model(&Branch{}).Where("codebase_id = ?", codebaseID).
		Update("is_default", false).Error
}

// ==================== Commit Repository ====================

// UpsertCommit creates or updates a commit
func UpsertCommit(db *gorm.DB, commit *Commit) error {
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "codebase_id"}, {Name: "hash"}},
		DoUpdates: clause.AssignmentColumns([]string{"branch_id", "summary", "is_user_commit", "is_on_default_branch"}),
	}).Create(commit).Error
}

// GetCommitByHash retrieves a commit by codebase and hash
func GetCommitByHash(db *gorm.DB, codebaseID, hash string) (*Commit, error) {
	var commit Commit
	err := db.Where("codebase_id = ? AND hash = ?", codebaseID, hash).First(&commit).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &commit, err
}

// CommitExists checks if a commit exists
func CommitExists(db *gorm.DB, codebaseID, hash string) (bool, error) {
	var count int64
	err := db.Model(&Commit{}).Where("codebase_id = ? AND hash = ?", codebaseID, hash).Count(&count).Error
	return count > 0, err
}

// GetCommitsByBranch retrieves all commits for a branch
func GetCommitsByBranch(db *gorm.DB, branchID string) ([]Commit, error) {
	var commits []Commit
	err := db.Where("branch_id = ?", branchID).Order("committed_at DESC").Find(&commits).Error
	return commits, err
}

// GetUserCommits retrieves commits by the current user within a date range
func GetUserCommits(db *gorm.DB, codebaseID string, since time.Time) ([]Commit, error) {
	var commits []Commit
	err := db.Where("codebase_id = ? AND is_user_commit = ? AND committed_at >= ?", codebaseID, true, since).
		Order("committed_at DESC").Find(&commits).Error
	return commits, err
}

// GetUserCommitsByBranch retrieves user commits grouped by branch
func GetUserCommitsByBranch(db *gorm.DB, codebaseID string, since time.Time) (map[string][]Commit, error) {
	commits, err := GetUserCommits(db, codebaseID, since)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]Commit)
	for _, c := range commits {
		result[c.BranchID] = append(result[c.BranchID], c)
	}
	return result, nil
}

// GetAllUserCommits retrieves all commits by the current user across codebases
func GetAllUserCommits(db *gorm.DB, since time.Time) ([]Commit, error) {
	var commits []Commit
	err := db.Where("is_user_commit = ? AND committed_at >= ?", true, since).
		Order("committed_at DESC").Find(&commits).Error
	return commits, err
}

// UpdateCommitSummary updates the summary for a commit
func UpdateCommitSummary(db *gorm.DB, commitID, summary string) error {
	return db.Model(&Commit{}).Where("id = ?", commitID).Update("summary", summary).Error
}

// GetCommitCount returns the number of commits for a codebase
func GetCommitCount(db *gorm.DB, codebaseID string) (int64, error) {
	var count int64
	err := db.Model(&Commit{}).Where("codebase_id = ?", codebaseID).Count(&count).Error
	return count, err
}

// GetCommitCountByPath returns the number of commits for a codebase by path
func GetCommitCountByPath(db *gorm.DB, repoPath string) (int64, error) {
	var codebase Codebase
	if err := db.Where("path = ?", repoPath).First(&codebase).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, nil
		}
		return 0, err
	}
	return GetCommitCount(db, codebase.ID)
}

// ==================== FileChange Repository ====================

// CreateFileChange creates a file change
func CreateFileChange(db *gorm.DB, fc *FileChange) error {
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(fc).Error
}

// GetFileChangesByCommit retrieves file changes for a commit
func GetFileChangesByCommit(db *gorm.DB, commitID string) ([]FileChange, error) {
	var changes []FileChange
	err := db.Where("commit_id = ?", commitID).Find(&changes).Error
	return changes, err
}

// GetFileChangeCount returns the number of file changes for a codebase
func GetFileChangeCount(db *gorm.DB, codebaseID string) (int64, error) {
	var count int64
	err := db.Model(&FileChange{}).
		Joins("JOIN commits ON file_changes.commit_id = commits.id").
		Where("commits.codebase_id = ?", codebaseID).
		Count(&count).Error
	return count, err
}

// GetFileChangeCountByPath returns the number of file changes by repo path
func GetFileChangeCountByPath(db *gorm.DB, repoPath string) (int64, error) {
	var codebase Codebase
	if err := db.Where("path = ?", repoPath).First(&codebase).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, nil
		}
		return 0, err
	}
	return GetFileChangeCount(db, codebase.ID)
}

// ==================== IngestCursor Repository ====================

// GetBranchCursor retrieves the last scanned hash for a branch
func GetBranchCursor(db *gorm.DB, codebaseID, branchName string) (string, error) {
	var cursor IngestCursor
	err := db.Where("codebase_id = ? AND branch_name = ?", codebaseID, branchName).First(&cursor).Error
	if err == gorm.ErrRecordNotFound {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return cursor.LastCommitHash, nil
}

// UpdateBranchCursor updates the last scanned hash for a branch
func UpdateBranchCursor(db *gorm.DB, codebaseID, branchName, hash string) error {
	cursor := IngestCursor{
		ID:             fmt.Sprintf("%s:%s", codebaseID, branchName),
		CodebaseID:     codebaseID,
		BranchName:     branchName,
		LastCommitHash: hash,
		UpdatedAt:      time.Now(),
	}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_commit_hash", "updated_at"}),
	}).Create(&cursor).Error
}

// ==================== Folder Repository ====================

// UpsertFolder creates or updates a folder
func UpsertFolder(db *gorm.DB, folder *Folder) error {
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "codebase_id"}, {Name: "path"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "depth", "parent_path", "summary", "purpose", "file_count", "indexed_at"}),
	}).Create(folder).Error
}

// GetFoldersByCodebase retrieves all folders for a codebase
func GetFoldersByCodebase(db *gorm.DB, codebaseID string) ([]Folder, error) {
	var folders []Folder
	err := db.Where("codebase_id = ?", codebaseID).Order("depth, path").Find(&folders).Error
	return folders, err
}

// GetFolderByPath retrieves a folder by path
func GetFolderByPath(db *gorm.DB, codebaseID, path string) (*Folder, error) {
	var folder Folder
	err := db.Where("codebase_id = ? AND path = ?", codebaseID, path).First(&folder).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &folder, err
}

// ==================== FileIndex Repository ====================

// UpsertFileIndex creates or updates a file index
func UpsertFileIndex(db *gorm.DB, file *FileIndex) error {
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "codebase_id"}, {Name: "path"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "extension", "language", "size_bytes", "line_count", "summary", "purpose", "key_exports", "dependencies", "content_hash", "indexed_at"}),
	}).Create(file).Error
}

// GetFilesByCodebase retrieves all files for a codebase
func GetFilesByCodebase(db *gorm.DB, codebaseID string) ([]FileIndex, error) {
	var files []FileIndex
	err := db.Where("codebase_id = ?", codebaseID).Order("path").Find(&files).Error
	return files, err
}

// GetFilesByFolder retrieves all files in a folder
func GetFilesByFolder(db *gorm.DB, folderID string) ([]FileIndex, error) {
	var files []FileIndex
	err := db.Where("folder_id = ?", folderID).Order("name").Find(&files).Error
	return files, err
}

// SearchFilesBySummary searches files by summary text
func SearchFilesBySummary(db *gorm.DB, codebaseID, query string) ([]FileIndex, error) {
	var files []FileIndex
	searchPattern := "%" + query + "%"
	err := db.Where("codebase_id = ? AND (summary LIKE ? OR purpose LIKE ? OR name LIKE ?)",
		codebaseID, searchPattern, searchPattern, searchPattern).
		Order("path").Limit(20).Find(&files).Error
	return files, err
}

// ==================== Statistics ====================

// CodebaseStats holds statistics for a codebase
type CodebaseStats struct {
	FolderCount int64
	FileCount   int64
	TotalSize   int64
	TotalLines  int64
	Languages   map[string]int
}

// GetCodebaseStats returns statistics about an indexed codebase
func GetCodebaseStats(db *gorm.DB, codebaseID string) (*CodebaseStats, error) {
	stats := &CodebaseStats{
		Languages: make(map[string]int),
	}

	db.Model(&Folder{}).Where("codebase_id = ?", codebaseID).Count(&stats.FolderCount)

	var fileStats struct {
		Count      int64
		TotalSize  int64
		TotalLines int64
	}
	db.Model(&FileIndex{}).Select("COUNT(*) as count, COALESCE(SUM(size_bytes), 0) as total_size, COALESCE(SUM(line_count), 0) as total_lines").
		Where("codebase_id = ?", codebaseID).Scan(&fileStats)

	stats.FileCount = fileStats.Count
	stats.TotalSize = fileStats.TotalSize
	stats.TotalLines = fileStats.TotalLines

	// Language breakdown
	var langCounts []struct {
		Language string
		Count    int
	}
	db.Model(&FileIndex{}).Select("language, COUNT(*) as count").
		Where("codebase_id = ? AND language IS NOT NULL AND language != ''", codebaseID).
		Group("language").Order("count DESC").Scan(&langCounts)

	for _, lc := range langCounts {
		stats.Languages[lc.Language] = lc.Count
	}

	return stats, nil
}

// ==================== Semantic Search ====================

// HasEmbeddings checks if the codebase has any embeddings stored
// Note: SQLite doesn't support native vector operations, so this always returns false
func HasEmbeddings(db *gorm.DB, codebaseID string) bool {
	// In the future, we could add vector search using sqlite-vss or similar
	// For now, return false to fall back to keyword search
	return false
}

// SemanticSearchFiles searches files using vector similarity
// Note: SQLite doesn't support native vector operations, falls back to text search
func SemanticSearchFiles(db *gorm.DB, codebaseID string, queryEmbedding []float32, limit int) ([]FileIndex, error) {
	// Without vector support, return empty results
	// The search.go will fall back to keyword search
	return nil, nil
}

// SemanticSearchFolders searches folders using vector similarity
// Note: SQLite doesn't support native vector operations, falls back to text search
func SemanticSearchFolders(db *gorm.DB, codebaseID string, queryEmbedding []float32, limit int) ([]Folder, error) {
	// Without vector support, return empty results
	// The search.go will fall back to keyword search
	return nil, nil
}

// ==================== Raw Query Support ====================

// ExecuteQuery executes a raw SQL query and returns results as maps
func ExecuteQuery(db *gorm.DB, query string) ([]map[string]interface{}, error) {
	var results []map[string]interface{}
	err := db.Raw(query).Scan(&results).Error
	return results, err
}
