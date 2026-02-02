package db

import (
	"database/sql"
	"fmt"
	"time"
)

// ==================== Developer Repository ====================

// UpsertDeveloper creates or updates a developer
func UpsertDeveloper(db *sql.DB, dev *Developer) error {
	// Delete existing record first (DuckDB ON CONFLICT has limitations)
	db.Exec(`DELETE FROM developers WHERE email = $1`, dev.Email)

	_, err := db.Exec(`
		INSERT INTO developers (id, name, email, is_current_user)
		VALUES ($1, $2, $3, $4)
	`, dev.ID, dev.Name, dev.Email, dev.IsCurrentUser)
	return err
}

// GetDeveloperByEmail retrieves a developer by email
func GetDeveloperByEmail(db *sql.DB, email string) (*Developer, error) {
	row := db.QueryRow(`SELECT id, name, email, is_current_user FROM developers WHERE email = $1`, email)
	dev := &Developer{}
	err := row.Scan(&dev.ID, &dev.Name, &dev.Email, &dev.IsCurrentUser)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return dev, err
}

// SetCurrentUser marks a developer as the current user
func SetCurrentUser(db *sql.DB, email string) error {
	_, err := db.Exec(`UPDATE developers SET is_current_user = FALSE WHERE is_current_user = TRUE`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE developers SET is_current_user = TRUE WHERE email = $1`, email)
	return err
}

// GetCurrentUser retrieves the current user
func GetCurrentUser(db *sql.DB) (*Developer, error) {
	row := db.QueryRow(`SELECT id, name, email, is_current_user FROM developers WHERE is_current_user = TRUE`)
	dev := &Developer{}
	err := row.Scan(&dev.ID, &dev.Name, &dev.Email, &dev.IsCurrentUser)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return dev, err
}

// ==================== Codebase Repository ====================

// UpsertCodebase creates or updates a codebase
func UpsertCodebase(db *sql.DB, codebase *Codebase) error {
	_, err := db.Exec(`
		INSERT INTO codebases (id, path, name, summary, tech_stack, default_branch, indexed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (path) DO UPDATE SET
			name = EXCLUDED.name,
			summary = EXCLUDED.summary,
			tech_stack = EXCLUDED.tech_stack,
			default_branch = EXCLUDED.default_branch,
			indexed_at = EXCLUDED.indexed_at
	`, codebase.ID, codebase.Path, codebase.Name, NullString(codebase.Summary),
		ToJSON(codebase.TechStack), NullString(codebase.DefaultBranch), NullTime(codebase.IndexedAt))
	return err
}

// GetCodebaseByPath retrieves a codebase by path
func GetCodebaseByPath(db *sql.DB, path string) (*Codebase, error) {
	row := db.QueryRow(`
		SELECT id, path, name, summary, tech_stack, default_branch, indexed_at
		FROM codebases WHERE path = $1
	`, path)
	return scanCodebase(row)
}

// GetCodebaseByID retrieves a codebase by ID
func GetCodebaseByID(db *sql.DB, id string) (*Codebase, error) {
	row := db.QueryRow(`
		SELECT id, path, name, summary, tech_stack, default_branch, indexed_at
		FROM codebases WHERE id = $1
	`, id)
	return scanCodebase(row)
}

func scanCodebase(row *sql.Row) (*Codebase, error) {
	c := &Codebase{}
	var summary, defaultBranch sql.NullString
	var techStack interface{}
	var indexedAt sql.NullTime

	err := row.Scan(&c.ID, &c.Path, &c.Name, &summary, &techStack, &defaultBranch, &indexedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	c.Summary = summary.String
	c.DefaultBranch = defaultBranch.String
	if indexedAt.Valid {
		c.IndexedAt = indexedAt.Time
	}

	// Handle tech_stack - DuckDB returns JSON as native Go types
	c.TechStack = convertToIntMap(techStack)

	return c, nil
}

// GetAllCodebases retrieves all codebases
func GetAllCodebases(db *sql.DB) ([]Codebase, error) {
	rows, err := db.Query(`SELECT id, path, name, summary, tech_stack, default_branch, indexed_at FROM codebases ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var codebases []Codebase
	for rows.Next() {
		c := Codebase{}
		var summary, defaultBranch sql.NullString
		var techStack interface{}
		var indexedAt sql.NullTime

		if err := rows.Scan(&c.ID, &c.Path, &c.Name, &summary, &techStack, &defaultBranch, &indexedAt); err != nil {
			return nil, err
		}

		c.Summary = summary.String
		c.DefaultBranch = defaultBranch.String
		if indexedAt.Valid {
			c.IndexedAt = indexedAt.Time
		}
		c.TechStack = convertToIntMap(techStack)
		codebases = append(codebases, c)
	}
	return codebases, rows.Err()
}

// ==================== Branch Repository ====================

// UpsertBranch creates or updates a branch
func UpsertBranch(db *sql.DB, branch *Branch) error {
	// Delete existing record first (DuckDB ON CONFLICT has limitations)
	db.Exec(`DELETE FROM branches WHERE codebase_id = $1 AND name = $2`, branch.CodebaseID, branch.Name)

	_, err := db.Exec(`
		INSERT INTO branches (id, codebase_id, name, is_default, base_branch, summary, story, status,
			first_commit_hash, last_commit_hash, commit_count, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, branch.ID, branch.CodebaseID, branch.Name, branch.IsDefault, NullString(branch.BaseBranch),
		NullString(branch.Summary), NullString(branch.Story), NullString(branch.Status),
		NullString(branch.FirstCommitHash), NullString(branch.LastCommitHash), branch.CommitCount,
		NullTime(branch.CreatedAt), NullTime(branch.UpdatedAt))
	return err
}

// GetBranch retrieves a branch by codebase and name
func GetBranch(db *sql.DB, codebaseID, name string) (*Branch, error) {
	row := db.QueryRow(`
		SELECT id, codebase_id, name, is_default, base_branch, summary, story, status,
			first_commit_hash, last_commit_hash, commit_count, created_at, updated_at
		FROM branches WHERE codebase_id = $1 AND name = $2
	`, codebaseID, name)
	return scanBranch(row)
}

// GetBranchByID retrieves a branch by ID
func GetBranchByID(db *sql.DB, id string) (*Branch, error) {
	row := db.QueryRow(`
		SELECT id, codebase_id, name, is_default, base_branch, summary, story, status,
			first_commit_hash, last_commit_hash, commit_count, created_at, updated_at
		FROM branches WHERE id = $1
	`, id)
	return scanBranch(row)
}

func scanBranch(row *sql.Row) (*Branch, error) {
	b := &Branch{}
	var baseBranch, summary, story, status, firstHash, lastHash sql.NullString
	var createdAt, updatedAt sql.NullTime

	err := row.Scan(&b.ID, &b.CodebaseID, &b.Name, &b.IsDefault, &baseBranch, &summary, &story,
		&status, &firstHash, &lastHash, &b.CommitCount, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	b.BaseBranch = baseBranch.String
	b.Summary = summary.String
	b.Story = story.String
	b.Status = status.String
	b.FirstCommitHash = firstHash.String
	b.LastCommitHash = lastHash.String
	if createdAt.Valid {
		b.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		b.UpdatedAt = updatedAt.Time
	}

	return b, nil
}

// GetBranchesByCodebase retrieves all branches for a codebase
func GetBranchesByCodebase(db *sql.DB, codebaseID string) ([]Branch, error) {
	rows, err := db.Query(`
		SELECT id, codebase_id, name, is_default, base_branch, summary, story, status,
			first_commit_hash, last_commit_hash, commit_count, created_at, updated_at
		FROM branches WHERE codebase_id = $1 ORDER BY is_default DESC, updated_at DESC
	`, codebaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var branches []Branch
	for rows.Next() {
		b := Branch{}
		var baseBranch, summary, story, status, firstHash, lastHash sql.NullString
		var createdAt, updatedAt sql.NullTime

		if err := rows.Scan(&b.ID, &b.CodebaseID, &b.Name, &b.IsDefault, &baseBranch, &summary, &story,
			&status, &firstHash, &lastHash, &b.CommitCount, &createdAt, &updatedAt); err != nil {
			return nil, err
		}

		b.BaseBranch = baseBranch.String
		b.Summary = summary.String
		b.Story = story.String
		b.Status = status.String
		b.FirstCommitHash = firstHash.String
		b.LastCommitHash = lastHash.String
		if createdAt.Valid {
			b.CreatedAt = createdAt.Time
		}
		if updatedAt.Valid {
			b.UpdatedAt = updatedAt.Time
		}
		branches = append(branches, b)
	}
	return branches, rows.Err()
}

// GetBranchCommits retrieves recent commits for a branch
func GetBranchCommits(db *sql.DB, branchID string, limit int) ([]Commit, error) {
	rows, err := db.Query(`
		SELECT id, hash, codebase_id, branch_id, author_email, message, summary,
			committed_at, stats, is_user_commit, is_on_default_branch
		FROM commits WHERE branch_id = $1 ORDER BY committed_at DESC LIMIT $2
	`, branchID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanCommits(rows)
}

// ClearDefaultBranch clears the default flag on all branches for a codebase
func ClearDefaultBranch(db *sql.DB, codebaseID string) error {
	_, err := db.Exec(`UPDATE branches SET is_default = FALSE WHERE codebase_id = $1`, codebaseID)
	return err
}

// ==================== Commit Repository ====================

// UpsertCommit creates or updates a commit
func UpsertCommit(db *sql.DB, commit *Commit) error {
	// Delete existing record first (DuckDB ON CONFLICT has limitations with indexed columns)
	db.Exec(`DELETE FROM commits WHERE codebase_id = $1 AND hash = $2`, commit.CodebaseID, commit.Hash)

	_, err := db.Exec(`
		INSERT INTO commits (id, hash, codebase_id, branch_id, author_email, message, summary,
			committed_at, stats, is_user_commit, is_on_default_branch)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, commit.ID, commit.Hash, commit.CodebaseID, NullString(commit.BranchID), commit.AuthorEmail,
		commit.Message, NullString(commit.Summary), commit.CommittedAt, ToJSON(commit.Stats),
		commit.IsUserCommit, commit.IsOnDefaultBranch)
	return err
}

// CommitExists checks if a commit exists
func CommitExists(db *sql.DB, codebaseID, hash string) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM commits WHERE codebase_id = $1 AND hash = $2`, codebaseID, hash).Scan(&count)
	return count > 0, err
}

// GetCommitByHash retrieves a commit by hash
func GetCommitByHash(db *sql.DB, codebaseID, hash string) (*Commit, error) {
	row := db.QueryRow(`
		SELECT id, hash, codebase_id, branch_id, author_email, message, summary,
			committed_at, stats, is_user_commit, is_on_default_branch
		FROM commits WHERE codebase_id = $1 AND hash = $2
	`, codebaseID, hash)

	c := &Commit{}
	var branchID, summary sql.NullString
	var stats interface{}

	err := row.Scan(&c.ID, &c.Hash, &c.CodebaseID, &branchID, &c.AuthorEmail, &c.Message, &summary,
		&c.CommittedAt, &stats, &c.IsUserCommit, &c.IsOnDefaultBranch)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	c.BranchID = branchID.String
	c.Summary = summary.String
	c.Stats = convertToMap(stats)

	return c, nil
}

func scanCommits(rows *sql.Rows) ([]Commit, error) {
	var commits []Commit
	for rows.Next() {
		c := Commit{}
		var branchID, summary sql.NullString
		var stats interface{}

		if err := rows.Scan(&c.ID, &c.Hash, &c.CodebaseID, &branchID, &c.AuthorEmail, &c.Message, &summary,
			&c.CommittedAt, &stats, &c.IsUserCommit, &c.IsOnDefaultBranch); err != nil {
			return nil, err
		}

		c.BranchID = branchID.String
		c.Summary = summary.String
		c.Stats = convertToMap(stats)
		commits = append(commits, c)
	}
	return commits, rows.Err()
}

// GetUserCommits retrieves commits by the current user within a date range
func GetUserCommits(db *sql.DB, codebaseID string, since time.Time) ([]Commit, error) {
	rows, err := db.Query(`
		SELECT id, hash, codebase_id, branch_id, author_email, message, summary,
			committed_at, stats, is_user_commit, is_on_default_branch
		FROM commits WHERE codebase_id = $1 AND is_user_commit = TRUE AND committed_at >= $2
		ORDER BY committed_at DESC
	`, codebaseID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanCommits(rows)
}

// GetCommitCount returns the number of commits for a codebase
func GetCommitCount(db *sql.DB, codebaseID string) (int64, error) {
	var count int64
	err := db.QueryRow(`SELECT COUNT(*) FROM commits WHERE codebase_id = $1`, codebaseID).Scan(&count)
	return count, err
}

// GetCommitCountByPath returns the number of commits for a codebase by path
func GetCommitCountByPath(db *sql.DB, repoPath string) (int64, error) {
	var count int64
	err := db.QueryRow(`
		SELECT COUNT(*) FROM commits c
		JOIN codebases cb ON c.codebase_id = cb.id
		WHERE cb.path = $1
	`, repoPath).Scan(&count)
	return count, err
}

// ==================== FileChange Repository ====================

// CreateFileChange creates a file change
func CreateFileChange(db *sql.DB, fc *FileChange) error {
	_, err := db.Exec(`
		INSERT INTO file_changes (id, commit_id, file_path, change_type, additions, deletions, patch)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT DO NOTHING
	`, fc.ID, fc.CommitID, fc.FilePath, fc.ChangeType, fc.Additions, fc.Deletions, NullString(fc.Patch))
	return err
}

// GetFileChangesByCommit retrieves file changes for a commit
func GetFileChangesByCommit(db *sql.DB, commitID string) ([]FileChange, error) {
	rows, err := db.Query(`
		SELECT id, commit_id, file_path, change_type, additions, deletions, patch
		FROM file_changes WHERE commit_id = $1
	`, commitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var changes []FileChange
	for rows.Next() {
		fc := FileChange{}
		var patch sql.NullString
		if err := rows.Scan(&fc.ID, &fc.CommitID, &fc.FilePath, &fc.ChangeType, &fc.Additions, &fc.Deletions, &patch); err != nil {
			return nil, err
		}
		fc.Patch = patch.String
		changes = append(changes, fc)
	}
	return changes, rows.Err()
}

// GetFileChangeCount returns the number of file changes for a codebase
func GetFileChangeCount(db *sql.DB, codebaseID string) (int64, error) {
	var count int64
	err := db.QueryRow(`
		SELECT COUNT(*) FROM file_changes fc
		JOIN commits c ON fc.commit_id = c.id
		WHERE c.codebase_id = $1
	`, codebaseID).Scan(&count)
	return count, err
}

// ==================== IngestCursor Repository ====================

// GetBranchCursor retrieves the last scanned hash for a branch
func GetBranchCursor(db *sql.DB, codebaseID, branchName string) (string, error) {
	var hash sql.NullString
	err := db.QueryRow(`
		SELECT last_commit_hash FROM ingest_cursors
		WHERE codebase_id = $1 AND branch_name = $2
	`, codebaseID, branchName).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return hash.String, err
}

// UpdateBranchCursor updates the last scanned hash for a branch
func UpdateBranchCursor(db *sql.DB, codebaseID, branchName, hash string) error {
	id := fmt.Sprintf("%s:%s", codebaseID, branchName)

	// Delete existing record first (DuckDB ON CONFLICT has limitations)
	db.Exec(`DELETE FROM ingest_cursors WHERE codebase_id = $1 AND branch_name = $2`, codebaseID, branchName)

	_, err := db.Exec(`
		INSERT INTO ingest_cursors (id, codebase_id, branch_name, last_commit_hash, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`, id, codebaseID, branchName, hash, time.Now())
	return err
}

// ==================== Folder Repository ====================

// UpsertFolder creates or updates a folder
func UpsertFolder(db *sql.DB, folder *Folder) error {
	// Delete existing record first (DuckDB ON CONFLICT has limitations)
	db.Exec(`DELETE FROM folders WHERE codebase_id = $1 AND path = $2`, folder.CodebaseID, folder.Path)

	_, err := db.Exec(`
		INSERT INTO folders (id, codebase_id, path, name, depth, parent_path, summary, purpose, file_count, indexed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, folder.ID, folder.CodebaseID, folder.Path, folder.Name, folder.Depth, NullString(folder.ParentPath),
		NullString(folder.Summary), NullString(folder.Purpose), folder.FileCount, NullTime(folder.IndexedAt))
	return err
}

// GetFoldersByCodebase retrieves all folders for a codebase
func GetFoldersByCodebase(db *sql.DB, codebaseID string) ([]Folder, error) {
	rows, err := db.Query(`
		SELECT id, codebase_id, path, name, depth, parent_path, summary, purpose, file_count, indexed_at
		FROM folders WHERE codebase_id = $1 ORDER BY depth, path
	`, codebaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folders []Folder
	for rows.Next() {
		f := Folder{}
		var parentPath, summary, purpose sql.NullString
		var indexedAt sql.NullTime

		if err := rows.Scan(&f.ID, &f.CodebaseID, &f.Path, &f.Name, &f.Depth, &parentPath,
			&summary, &purpose, &f.FileCount, &indexedAt); err != nil {
			return nil, err
		}

		f.ParentPath = parentPath.String
		f.Summary = summary.String
		f.Purpose = purpose.String
		if indexedAt.Valid {
			f.IndexedAt = indexedAt.Time
		}
		folders = append(folders, f)
	}
	return folders, rows.Err()
}

// GetFolderByPath retrieves a folder by path
func GetFolderByPath(db *sql.DB, codebaseID, path string) (*Folder, error) {
	row := db.QueryRow(`
		SELECT id, codebase_id, path, name, depth, parent_path, summary, purpose, file_count, indexed_at
		FROM folders WHERE codebase_id = $1 AND path = $2
	`, codebaseID, path)

	f := &Folder{}
	var parentPath, summary, purpose sql.NullString
	var indexedAt sql.NullTime

	err := row.Scan(&f.ID, &f.CodebaseID, &f.Path, &f.Name, &f.Depth, &parentPath,
		&summary, &purpose, &f.FileCount, &indexedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	f.ParentPath = parentPath.String
	f.Summary = summary.String
	f.Purpose = purpose.String
	if indexedAt.Valid {
		f.IndexedAt = indexedAt.Time
	}

	return f, nil
}

// ==================== FileIndex Repository ====================

// UpsertFileIndex creates or updates a file index
func UpsertFileIndex(db *sql.DB, file *FileIndex) error {
	// Delete existing record first (DuckDB ON CONFLICT has limitations)
	db.Exec(`DELETE FROM file_indexes WHERE codebase_id = $1 AND path = $2`, file.CodebaseID, file.Path)

	_, err := db.Exec(`
		INSERT INTO file_indexes (id, codebase_id, folder_id, path, name, extension, language,
			size_bytes, line_count, summary, purpose, key_exports, dependencies, content_hash, indexed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, file.ID, file.CodebaseID, NullString(file.FolderID), file.Path, file.Name, NullString(file.Extension),
		NullString(file.Language), file.SizeBytes, file.LineCount, NullString(file.Summary), NullString(file.Purpose),
		ToJSON(file.KeyExports), ToJSON(file.Dependencies), NullString(file.ContentHash), NullTime(file.IndexedAt))
	return err
}

// GetFilesByCodebase retrieves all files for a codebase
func GetFilesByCodebase(db *sql.DB, codebaseID string) ([]FileIndex, error) {
	rows, err := db.Query(`
		SELECT id, codebase_id, folder_id, path, name, extension, language,
			size_bytes, line_count, summary, purpose, key_exports, dependencies, content_hash, indexed_at
		FROM file_indexes WHERE codebase_id = $1 ORDER BY path
	`, codebaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanFileIndexes(rows)
}

// GetFilesByFolder retrieves all files in a folder
func GetFilesByFolder(db *sql.DB, folderID string) ([]FileIndex, error) {
	rows, err := db.Query(`
		SELECT id, codebase_id, folder_id, path, name, extension, language,
			size_bytes, line_count, summary, purpose, key_exports, dependencies, content_hash, indexed_at
		FROM file_indexes WHERE folder_id = $1 ORDER BY name
	`, folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanFileIndexes(rows)
}

func scanFileIndexes(rows *sql.Rows) ([]FileIndex, error) {
	var files []FileIndex
	for rows.Next() {
		f := FileIndex{}
		var folderID, extension, language, summary, purpose, contentHash sql.NullString
		var keyExports, deps interface{}
		var indexedAt sql.NullTime

		if err := rows.Scan(&f.ID, &f.CodebaseID, &folderID, &f.Path, &f.Name, &extension, &language,
			&f.SizeBytes, &f.LineCount, &summary, &purpose, &keyExports, &deps, &contentHash, &indexedAt); err != nil {
			return nil, err
		}

		f.FolderID = folderID.String
		f.Extension = extension.String
		f.Language = language.String
		f.Summary = summary.String
		f.Purpose = purpose.String
		f.KeyExports = convertToStringSlice(keyExports)
		f.Dependencies = convertToStringSlice(deps)
		f.ContentHash = contentHash.String
		if indexedAt.Valid {
			f.IndexedAt = indexedAt.Time
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// SearchFilesBySummary searches files by summary text
func SearchFilesBySummary(db *sql.DB, codebaseID, query string) ([]FileIndex, error) {
	searchPattern := "%" + query + "%"
	rows, err := db.Query(`
		SELECT id, codebase_id, folder_id, path, name, extension, language,
			size_bytes, line_count, summary, purpose, key_exports, dependencies, content_hash, indexed_at
		FROM file_indexes
		WHERE codebase_id = $1 AND (summary ILIKE $2 OR purpose ILIKE $2 OR name ILIKE $2)
		ORDER BY path LIMIT 20
	`, codebaseID, searchPattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanFileIndexes(rows)
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
func GetCodebaseStats(db *sql.DB, codebaseID string) (*CodebaseStats, error) {
	stats := &CodebaseStats{
		Languages: make(map[string]int),
	}

	// Folder count
	db.QueryRow(`SELECT COUNT(*) FROM folders WHERE codebase_id = $1`, codebaseID).Scan(&stats.FolderCount)

	// File stats
	db.QueryRow(`
		SELECT COALESCE(COUNT(*), 0), COALESCE(SUM(size_bytes), 0), COALESCE(SUM(line_count), 0)
		FROM file_indexes WHERE codebase_id = $1
	`, codebaseID).Scan(&stats.FileCount, &stats.TotalSize, &stats.TotalLines)

	// Language breakdown
	rows, err := db.Query(`
		SELECT language, COUNT(*) as count
		FROM file_indexes
		WHERE codebase_id = $1 AND language IS NOT NULL AND language != ''
		GROUP BY language ORDER BY count DESC
	`, codebaseID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var lang string
			var count int
			if rows.Scan(&lang, &count) == nil {
				stats.Languages[lang] = count
			}
		}
	}

	return stats, nil
}

// ==================== Semantic Search ====================

// HasEmbeddings checks if the codebase has any embeddings stored
// DuckDB can be extended with vector support via extensions
func HasEmbeddings(db *sql.DB, codebaseID string) bool {
	// For now, return false - can be extended later with DuckDB vector extension
	return false
}

// SemanticSearchFiles searches files using vector similarity
// Falls back to text search without vector support
func SemanticSearchFiles(db *sql.DB, codebaseID string, queryEmbedding []float32, limit int) ([]FileIndex, error) {
	// Without vector support, return empty - caller will fall back to keyword search
	return nil, nil
}

// SemanticSearchFolders searches folders using vector similarity
func SemanticSearchFolders(db *sql.DB, codebaseID string, queryEmbedding []float32, limit int) ([]Folder, error) {
	// Without vector support, return empty - caller will fall back to keyword search
	return nil, nil
}

// ==================== Raw Query Support ====================

// ExecuteQuery executes a raw SQL query and returns results as maps
func ExecuteQuery(db *sql.DB, query string) ([]map[string]interface{}, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	return results, rows.Err()
}
