package db

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"time"
)

// Repository defines the interface for all database operations,
// grouped by functionality for clarity.
type Repository interface {
	// Developer operations
	// --------------------
	UpsertDeveloper(ctx context.Context, dev *Developer) error
	GetDeveloperByEmail(ctx context.Context, email string) (*Developer, error)
	SetCurrentUser(ctx context.Context, email string) error
	GetCurrentUser(ctx context.Context) (*Developer, error)

	// Codebase operations
	// -------------------
	UpsertCodebase(ctx context.Context, codebase *Codebase) error
	GetCodebaseByPath(ctx context.Context, path string) (*Codebase, error)
	GetCodebaseByID(ctx context.Context, id string) (*Codebase, error)
	GetAllCodebases(ctx context.Context) ([]Codebase, error)

	// Branch operations
	// -----------------
	UpsertBranch(ctx context.Context, branch *Branch) error
	GetBranch(ctx context.Context, codebaseID, name string) (*Branch, error)
	GetBranchByID(ctx context.Context, id string) (*Branch, error)
	GetBranchesByCodebase(ctx context.Context, codebaseID string) ([]Branch, error)
	GetBranchCommits(ctx context.Context, branchID string, limit int) ([]Commit, error)
	ClearDefaultBranch(ctx context.Context, codebaseID string) error

	// Commit operations
	// -----------------
	UpsertCommit(ctx context.Context, commit *Commit) error
	CommitExists(ctx context.Context, codebaseID, hash string) (bool, error)
	GetExistingCommitHashes(ctx context.Context, codebaseID string) (map[string]bool, error)
	GetUserCommitsMissingSummaries(ctx context.Context, codebaseID string) ([]Commit, error)
	UpdateCommitSummary(ctx context.Context, commitID, summary string) error
	GetCommitByHash(ctx context.Context, codebaseID, hash string) (*Commit, error)
	GetUserCommits(ctx context.Context, codebaseID string, since time.Time) ([]Commit, error)
	GetCommitCount(ctx context.Context, codebaseID string) (int64, error)
	GetCommitCountByPath(ctx context.Context, repoPath string) (int64, error)

	// File change operations
	// ----------------------
	CreateFileChange(ctx context.Context, fc *FileChange) error
	GetFileChangesByCommit(ctx context.Context, commitID string) ([]FileChange, error)
	GetFileChangeCount(ctx context.Context, codebaseID string) (int64, error)

	// Cursor operations
	// -----------------
	GetBranchCursor(ctx context.Context, codebaseID, branchName string) (string, error)
	UpdateBranchCursor(ctx context.Context, codebaseID, branchName, hash string) error

	// Folder and file indexing operations
	// -----------------------------------
	UpsertFolder(ctx context.Context, folder *Folder) error
	GetFoldersByCodebase(ctx context.Context, codebaseID string) ([]Folder, error)
	GetFolderByPath(ctx context.Context, codebaseID, path string) (*Folder, error)
	GetExistingFolderPaths(ctx context.Context, codebaseID string) (map[string]string, error)
	DeleteFoldersByPaths(ctx context.Context, codebaseID string, paths []string) error

	// File index operations
	UpsertFileIndex(ctx context.Context, file *FileIndex) error
	GetFilesByCodebase(ctx context.Context, codebaseID string) ([]FileIndex, error)
	GetExistingFileHashes(ctx context.Context, codebaseID string) (map[string]ExistingFileInfo, error)
	DeleteFileIndex(ctx context.Context, codebaseID, path string) error
	DeleteFileIndexesByPaths(ctx context.Context, codebaseID string, paths []string) error
	GetFilesByFolder(ctx context.Context, folderID string) ([]FileIndex, error)
	SearchFilesBySummary(ctx context.Context, codebaseID, query string) ([]FileIndex, error)

	// Codebase statistics and search operations
	// -----------------------------------------
	GetCodebaseStats(ctx context.Context, codebaseID string) (*CodebaseStats, error)
	HasEmbeddings(ctx context.Context, codebaseID string) bool
	SemanticSearchFiles(ctx context.Context, codebaseID string, queryEmbedding []float32, limit int) ([]FileIndex, error)
	SemanticSearchFolders(ctx context.Context, codebaseID string, queryEmbedding []float32, limit int) ([]Folder, error)

	// Raw query operations
	// --------------------
	ExecuteQuery(ctx context.Context, query string) ([]map[string]any, error)
}

// SQLRepository implements Repository using SQL database.
type SQLRepository struct {
	db *sql.DB
}

// NewRepository creates a new SQLRepository.
func NewRepository(db *sql.DB) *SQLRepository {
	return &SQLRepository{db: db}
}

// DB returns the underlying database connection.
func (r *SQLRepository) DB() *sql.DB {
	return r.db
}

// UpsertDeveloper creates or updates a developer.
func (r *SQLRepository) UpsertDeveloper(ctx context.Context, dev *Developer) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM developers WHERE email = $1`, dev.Email); err != nil {
		return fmt.Errorf("delete existing developer: %w", err)
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO developers (id, name, email, is_current_user) VALUES ($1, $2, $3, $4)`,
		dev.ID, dev.Name, dev.Email, dev.IsCurrentUser)
	if err != nil {
		return fmt.Errorf("insert developer: %w", err)
	}
	return nil
}

// GetDeveloperByEmail retrieves a developer by email.
func (r *SQLRepository) GetDeveloperByEmail(ctx context.Context, email string) (*Developer, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, name, email, is_current_user FROM developers WHERE email = $1`, email)
	dev := &Developer{}
	err := row.Scan(&dev.ID, &dev.Name, &dev.Email, &dev.IsCurrentUser)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan developer: %w", err)
	}
	return dev, nil
}

// SetCurrentUser marks a developer as the current user.
func (r *SQLRepository) SetCurrentUser(ctx context.Context, email string) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE developers SET is_current_user = FALSE WHERE is_current_user = TRUE`); err != nil {
		return fmt.Errorf("clear current user: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE developers SET is_current_user = TRUE WHERE email = $1`, email); err != nil {
		return fmt.Errorf("set current user: %w", err)
	}
	return nil
}

// GetCurrentUser retrieves the current user.
func (r *SQLRepository) GetCurrentUser(ctx context.Context) (*Developer, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, name, email, is_current_user FROM developers WHERE is_current_user = TRUE`)
	dev := &Developer{}
	err := row.Scan(&dev.ID, &dev.Name, &dev.Email, &dev.IsCurrentUser)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan current user: %w", err)
	}
	return dev, nil
}

// UpsertCodebase creates or updates a codebase.
func (r *SQLRepository) UpsertCodebase(ctx context.Context, codebase *Codebase) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO codebases (id, path, name, summary, tech_stack, default_branch, indexed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (path) DO UPDATE SET
			name = EXCLUDED.name, summary = EXCLUDED.summary, tech_stack = EXCLUDED.tech_stack,
			default_branch = EXCLUDED.default_branch, indexed_at = EXCLUDED.indexed_at`,
		codebase.ID, codebase.Path, codebase.Name, NullString(codebase.Summary),
		ToJSON(codebase.TechStack), NullString(codebase.DefaultBranch), NullTime(codebase.IndexedAt))
	if err != nil {
		return fmt.Errorf("upsert codebase: %w", err)
	}
	return nil
}

// GetCodebaseByPath retrieves a codebase by path.
func (r *SQLRepository) GetCodebaseByPath(ctx context.Context, path string) (*Codebase, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, path, name, summary, tech_stack, default_branch, indexed_at FROM codebases WHERE path = $1`, path)
	return r.scanCodebase(row)
}

// GetCodebaseByID retrieves a codebase by ID.
func (r *SQLRepository) GetCodebaseByID(ctx context.Context, id string) (*Codebase, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, path, name, summary, tech_stack, default_branch, indexed_at FROM codebases WHERE id = $1`, id)
	return r.scanCodebase(row)
}

func (r *SQLRepository) scanCodebase(row *sql.Row) (*Codebase, error) {
	c := &Codebase{}
	var summary, defaultBranch sql.NullString
	var techStack any
	var indexedAt sql.NullTime
	err := row.Scan(&c.ID, &c.Path, &c.Name, &summary, &techStack, &defaultBranch, &indexedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan codebase: %w", err)
	}
	c.Summary = summary.String
	c.DefaultBranch = defaultBranch.String
	if indexedAt.Valid {
		c.IndexedAt = indexedAt.Time
	}
	c.TechStack = convertToIntMap(techStack)
	return c, nil
}

// GetAllCodebases retrieves all codebases.
func (r *SQLRepository) GetAllCodebases(ctx context.Context) ([]Codebase, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, path, name, summary, tech_stack, default_branch, indexed_at FROM codebases ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("query codebases: %w", err)
	}
	defer rows.Close()
	var codebases []Codebase
	for rows.Next() {
		c := Codebase{}
		var summary, defaultBranch sql.NullString
		var techStack any
		var indexedAt sql.NullTime
		if err := rows.Scan(&c.ID, &c.Path, &c.Name, &summary, &techStack, &defaultBranch, &indexedAt); err != nil {
			return nil, fmt.Errorf("scan codebase row: %w", err)
		}
		c.Summary = summary.String
		c.DefaultBranch = defaultBranch.String
		if indexedAt.Valid {
			c.IndexedAt = indexedAt.Time
		}
		c.TechStack = convertToIntMap(techStack)
		codebases = append(codebases, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate codebases: %w", err)
	}
	return codebases, nil
}

// UpsertBranch creates or updates a branch.
func (r *SQLRepository) UpsertBranch(ctx context.Context, branch *Branch) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO branches (id, codebase_id, name, is_default, base_branch, summary, story, status,
			first_commit_hash, last_commit_hash, commit_count, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (codebase_id, name) DO UPDATE SET
			is_default = EXCLUDED.is_default, base_branch = EXCLUDED.base_branch,
			summary = EXCLUDED.summary, story = EXCLUDED.story, status = EXCLUDED.status,
			first_commit_hash = EXCLUDED.first_commit_hash, last_commit_hash = EXCLUDED.last_commit_hash,
			commit_count = EXCLUDED.commit_count, updated_at = EXCLUDED.updated_at`,
		branch.ID, branch.CodebaseID, branch.Name, branch.IsDefault, NullString(branch.BaseBranch),
		NullString(branch.Summary), NullString(branch.Story), NullString(branch.Status),
		NullString(branch.FirstCommitHash), NullString(branch.LastCommitHash), branch.CommitCount,
		NullTime(branch.CreatedAt), NullTime(branch.UpdatedAt))
	if err != nil {
		return fmt.Errorf("upsert branch: %w", err)
	}
	return nil
}

// GetBranch retrieves a branch by codebase and name.
func (r *SQLRepository) GetBranch(ctx context.Context, codebaseID, name string) (*Branch, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, codebase_id, name, is_default, base_branch, summary, story, status,
			first_commit_hash, last_commit_hash, commit_count, created_at, updated_at
		FROM branches WHERE codebase_id = $1 AND name = $2`, codebaseID, name)
	return r.scanBranch(row)
}

// GetBranchByID retrieves a branch by ID.
func (r *SQLRepository) GetBranchByID(ctx context.Context, id string) (*Branch, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, codebase_id, name, is_default, base_branch, summary, story, status,
			first_commit_hash, last_commit_hash, commit_count, created_at, updated_at
		FROM branches WHERE id = $1`, id)
	return r.scanBranch(row)
}

func (r *SQLRepository) scanBranch(row *sql.Row) (*Branch, error) {
	b := &Branch{}
	var baseBranch, summary, story, status, firstHash, lastHash sql.NullString
	var createdAt, updatedAt sql.NullTime
	err := row.Scan(&b.ID, &b.CodebaseID, &b.Name, &b.IsDefault, &baseBranch, &summary, &story,
		&status, &firstHash, &lastHash, &b.CommitCount, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan branch: %w", err)
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

// GetBranchesByCodebase retrieves all branches for a codebase.
func (r *SQLRepository) GetBranchesByCodebase(ctx context.Context, codebaseID string) ([]Branch, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, codebase_id, name, is_default, base_branch, summary, story, status,
			first_commit_hash, last_commit_hash, commit_count, created_at, updated_at
		FROM branches WHERE codebase_id = $1 ORDER BY is_default DESC, updated_at DESC`, codebaseID)
	if err != nil {
		return nil, fmt.Errorf("query branches: %w", err)
	}
	defer rows.Close()
	var branches []Branch
	for rows.Next() {
		b := Branch{}
		var baseBranch, summary, story, status, firstHash, lastHash sql.NullString
		var createdAt, updatedAt sql.NullTime
		if err := rows.Scan(&b.ID, &b.CodebaseID, &b.Name, &b.IsDefault, &baseBranch, &summary, &story,
			&status, &firstHash, &lastHash, &b.CommitCount, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan branch row: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate branches: %w", err)
	}
	return branches, nil
}

// GetBranchCommits retrieves recent commits for a branch.
func (r *SQLRepository) GetBranchCommits(ctx context.Context, branchID string, limit int) ([]Commit, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, hash, codebase_id, branch_id, author_email, message, summary,
			committed_at, stats, is_user_commit, is_on_default_branch
		FROM commits WHERE branch_id = $1 ORDER BY committed_at DESC LIMIT $2`, branchID, limit)
	if err != nil {
		return nil, fmt.Errorf("query branch commits: %w", err)
	}
	defer rows.Close()
	return r.scanCommits(rows)
}

// ClearDefaultBranch clears the default flag on all branches for a codebase.
func (r *SQLRepository) ClearDefaultBranch(ctx context.Context, codebaseID string) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE branches SET is_default = FALSE WHERE codebase_id = $1`, codebaseID); err != nil {
		return fmt.Errorf("clear default branch: %w", err)
	}
	return nil
}

// UpsertCommit creates or updates a commit.
func (r *SQLRepository) UpsertCommit(ctx context.Context, commit *Commit) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM commits WHERE codebase_id = $1 AND hash = $2`, commit.CodebaseID, commit.Hash); err != nil {
		return fmt.Errorf("delete existing commit: %w", err)
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO commits (id, hash, codebase_id, branch_id, author_email, message, summary,
			committed_at, stats, is_user_commit, is_on_default_branch)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		commit.ID, commit.Hash, commit.CodebaseID, NullString(commit.BranchID), commit.AuthorEmail,
		commit.Message, NullString(commit.Summary), commit.CommittedAt, ToJSON(commit.Stats),
		commit.IsUserCommit, commit.IsOnDefaultBranch)
	if err != nil {
		return fmt.Errorf("insert commit: %w", err)
	}
	return nil
}

// CommitExists checks if a commit exists.
func (r *SQLRepository) CommitExists(ctx context.Context, codebaseID, hash string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM commits WHERE codebase_id = $1 AND hash = $2`, codebaseID, hash).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("count commits: %w", err)
	}
	return count > 0, nil
}

// GetExistingCommitHashes returns commit hashes that exist for a codebase.
func (r *SQLRepository) GetExistingCommitHashes(ctx context.Context, codebaseID string) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT hash FROM commits WHERE codebase_id = $1`, codebaseID)
	if err != nil {
		return nil, fmt.Errorf("query commit hashes: %w", err)
	}
	defer rows.Close()
	hashes := make(map[string]bool)
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, fmt.Errorf("scan commit hash: %w", err)
		}
		hashes[hash] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate commit hashes: %w", err)
	}
	return hashes, nil
}

// GetUserCommitsMissingSummaries returns user commits without summaries.
func (r *SQLRepository) GetUserCommitsMissingSummaries(ctx context.Context, codebaseID string) ([]Commit, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, hash, codebase_id, branch_id, author_email, message, summary,
			committed_at, stats, is_user_commit, is_on_default_branch
		FROM commits WHERE codebase_id = $1 AND is_user_commit = TRUE AND (summary IS NULL OR summary = '')
		ORDER BY committed_at DESC`, codebaseID)
	if err != nil {
		return nil, fmt.Errorf("query commits missing summaries: %w", err)
	}
	defer rows.Close()
	return r.scanCommits(rows)
}

// UpdateCommitSummary updates a commit's summary.
func (r *SQLRepository) UpdateCommitSummary(ctx context.Context, commitID, summary string) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE commits SET summary = $1 WHERE id = $2`, summary, commitID); err != nil {
		return fmt.Errorf("update commit summary: %w", err)
	}
	return nil
}

// GetCommitByHash retrieves a commit by hash.
func (r *SQLRepository) GetCommitByHash(ctx context.Context, codebaseID, hash string) (*Commit, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, hash, codebase_id, branch_id, author_email, message, summary,
			committed_at, stats, is_user_commit, is_on_default_branch
		FROM commits WHERE codebase_id = $1 AND hash = $2`, codebaseID, hash)
	c := &Commit{}
	var branchID, summary sql.NullString
	var stats any
	err := row.Scan(&c.ID, &c.Hash, &c.CodebaseID, &branchID, &c.AuthorEmail, &c.Message, &summary,
		&c.CommittedAt, &stats, &c.IsUserCommit, &c.IsOnDefaultBranch)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan commit: %w", err)
	}
	c.BranchID = branchID.String
	c.Summary = summary.String
	c.Stats = convertToMap(stats)
	return c, nil
}

func (r *SQLRepository) scanCommits(rows *sql.Rows) ([]Commit, error) {
	var commits []Commit
	for rows.Next() {
		c := Commit{}
		var branchID, summary sql.NullString
		var stats any
		if err := rows.Scan(&c.ID, &c.Hash, &c.CodebaseID, &branchID, &c.AuthorEmail, &c.Message, &summary,
			&c.CommittedAt, &stats, &c.IsUserCommit, &c.IsOnDefaultBranch); err != nil {
			return nil, fmt.Errorf("scan commit row: %w", err)
		}
		c.BranchID = branchID.String
		c.Summary = summary.String
		c.Stats = convertToMap(stats)
		commits = append(commits, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate commits: %w", err)
	}
	return commits, nil
}

// GetUserCommits retrieves user commits since a date.
func (r *SQLRepository) GetUserCommits(ctx context.Context, codebaseID string, since time.Time) ([]Commit, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, hash, codebase_id, branch_id, author_email, message, summary,
			committed_at, stats, is_user_commit, is_on_default_branch
		FROM commits WHERE codebase_id = $1 AND is_user_commit = TRUE AND committed_at >= $2
		ORDER BY committed_at DESC`, codebaseID, since)
	if err != nil {
		return nil, fmt.Errorf("query user commits: %w", err)
	}
	defer rows.Close()
	return r.scanCommits(rows)
}

// GetCommitCount returns the commit count for a codebase.
func (r *SQLRepository) GetCommitCount(ctx context.Context, codebaseID string) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM commits WHERE codebase_id = $1`, codebaseID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count commits: %w", err)
	}
	return count, nil
}

// GetCommitCountByPath returns the commit count for a codebase by path.
func (r *SQLRepository) GetCommitCountByPath(ctx context.Context, repoPath string) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM commits c JOIN codebases cb ON c.codebase_id = cb.id WHERE cb.path = $1`, repoPath).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count commits by path: %w", err)
	}
	return count, nil
}

// CreateFileChange creates a file change.
func (r *SQLRepository) CreateFileChange(ctx context.Context, fc *FileChange) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO file_changes (id, commit_id, file_path, change_type, additions, deletions, patch)
		VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT DO NOTHING`,
		fc.ID, fc.CommitID, fc.FilePath, fc.ChangeType, fc.Additions, fc.Deletions, NullString(fc.Patch))
	if err != nil {
		return fmt.Errorf("create file change: %w", err)
	}
	return nil
}

// GetFileChangesByCommit retrieves file changes for a commit.
func (r *SQLRepository) GetFileChangesByCommit(ctx context.Context, commitID string) ([]FileChange, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, commit_id, file_path, change_type, additions, deletions, patch
		FROM file_changes WHERE commit_id = $1`, commitID)
	if err != nil {
		return nil, fmt.Errorf("query file changes: %w", err)
	}
	defer rows.Close()
	var changes []FileChange
	for rows.Next() {
		fc := FileChange{}
		var patch sql.NullString
		if err := rows.Scan(&fc.ID, &fc.CommitID, &fc.FilePath, &fc.ChangeType, &fc.Additions, &fc.Deletions, &patch); err != nil {
			return nil, fmt.Errorf("scan file change: %w", err)
		}
		fc.Patch = patch.String
		changes = append(changes, fc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate file changes: %w", err)
	}
	return changes, nil
}

// GetFileChangeCount returns file change count for a codebase.
func (r *SQLRepository) GetFileChangeCount(ctx context.Context, codebaseID string) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM file_changes fc JOIN commits c ON fc.commit_id = c.id WHERE c.codebase_id = $1`, codebaseID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count file changes: %w", err)
	}
	return count, nil
}

// GetBranchCursor retrieves the last scanned hash for a branch.
func (r *SQLRepository) GetBranchCursor(ctx context.Context, codebaseID, branchName string) (string, error) {
	var hash sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT last_commit_hash FROM ingest_cursors WHERE codebase_id = $1 AND branch_name = $2`,
		codebaseID, branchName).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get branch cursor: %w", err)
	}
	return hash.String, nil
}

// UpdateBranchCursor updates the last scanned hash for a branch.
func (r *SQLRepository) UpdateBranchCursor(ctx context.Context, codebaseID, branchName, hash string) error {
	id := fmt.Sprintf("%s:%s", codebaseID, branchName)
	if _, err := r.db.ExecContext(ctx, `DELETE FROM ingest_cursors WHERE codebase_id = $1 AND branch_name = $2`, codebaseID, branchName); err != nil {
		return fmt.Errorf("delete existing cursor: %w", err)
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO ingest_cursors (id, codebase_id, branch_name, last_commit_hash, updated_at) VALUES ($1, $2, $3, $4, $5)`,
		id, codebaseID, branchName, hash, time.Now())
	if err != nil {
		return fmt.Errorf("insert branch cursor: %w", err)
	}
	return nil
}

// UpsertFolder creates or updates a folder.
func (r *SQLRepository) UpsertFolder(ctx context.Context, folder *Folder) error {
	// Use INSERT ON CONFLICT on the unique constraint (codebase_id, path)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO folders (id, codebase_id, path, name, depth, parent_path, summary, purpose, file_count, indexed_at, embedding)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (codebase_id, path) DO UPDATE SET
			name = EXCLUDED.name,
			depth = EXCLUDED.depth,
			parent_path = EXCLUDED.parent_path,
			summary = EXCLUDED.summary,
			purpose = EXCLUDED.purpose,
			file_count = EXCLUDED.file_count,
			indexed_at = EXCLUDED.indexed_at,
			embedding = EXCLUDED.embedding`,
		folder.ID, folder.CodebaseID, folder.Path, folder.Name, folder.Depth, NullString(folder.ParentPath),
		NullString(folder.Summary), NullString(folder.Purpose), folder.FileCount, NullTime(folder.IndexedAt),
		NullString(EmbeddingToJSON(folder.Embedding)))
	if err != nil {
		return fmt.Errorf("upsert folder: %w", err)
	}

	return nil
}

// GetFoldersByCodebase retrieves all folders for a codebase.
func (r *SQLRepository) GetFoldersByCodebase(ctx context.Context, codebaseID string) ([]Folder, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, codebase_id, path, name, depth, parent_path, summary, purpose, file_count, indexed_at, embedding
		FROM folders WHERE codebase_id = $1 ORDER BY depth, path`, codebaseID)
	if err != nil {
		return nil, fmt.Errorf("query folders: %w", err)
	}
	defer rows.Close()
	return r.scanFolders(rows)
}

func (r *SQLRepository) scanFolders(rows *sql.Rows) ([]Folder, error) {
	var folders []Folder
	for rows.Next() {
		f := Folder{}
		var parentPath, summary, purpose, embeddingJSON sql.NullString
		var indexedAt sql.NullTime
		if err := rows.Scan(&f.ID, &f.CodebaseID, &f.Path, &f.Name, &f.Depth, &parentPath, &summary, &purpose, &f.FileCount, &indexedAt, &embeddingJSON); err != nil {
			return nil, fmt.Errorf("scan folder: %w", err)
		}
		f.ParentPath = parentPath.String
		f.Summary = summary.String
		f.Purpose = purpose.String
		f.Embedding = EmbeddingFromJSON(embeddingJSON.String)
		if indexedAt.Valid {
			f.IndexedAt = indexedAt.Time
		}
		folders = append(folders, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate folders: %w", err)
	}
	return folders, nil
}

// GetFolderByPath retrieves a folder by path.
func (r *SQLRepository) GetFolderByPath(ctx context.Context, codebaseID, path string) (*Folder, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, codebase_id, path, name, depth, parent_path, summary, purpose, file_count, indexed_at, embedding
		FROM folders WHERE codebase_id = $1 AND path = $2`, codebaseID, path)
	f := &Folder{}
	var parentPath, summary, purpose, embeddingJSON sql.NullString
	var indexedAt sql.NullTime
	err := row.Scan(&f.ID, &f.CodebaseID, &f.Path, &f.Name, &f.Depth, &parentPath, &summary, &purpose, &f.FileCount, &indexedAt, &embeddingJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan folder: %w", err)
	}
	f.ParentPath = parentPath.String
	f.Summary = summary.String
	f.Purpose = purpose.String
	f.Embedding = EmbeddingFromJSON(embeddingJSON.String)
	if indexedAt.Valid {
		f.IndexedAt = indexedAt.Time
	}
	return f, nil
}

// GetExistingFolderPaths returns folder paths for a codebase.
func (r *SQLRepository) GetExistingFolderPaths(ctx context.Context, codebaseID string) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, path FROM folders WHERE codebase_id = $1`, codebaseID)
	if err != nil {
		return nil, fmt.Errorf("query folder paths: %w", err)
	}
	defer rows.Close()
	result := make(map[string]string)
	for rows.Next() {
		var id, path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, fmt.Errorf("scan folder path: %w", err)
		}
		result[path] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate folder paths: %w", err)
	}
	return result, nil
}

// DeleteFoldersByPaths deletes folders by paths.
func (r *SQLRepository) DeleteFoldersByPaths(ctx context.Context, codebaseID string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	for _, path := range paths {
		if _, err := r.db.ExecContext(ctx, `DELETE FROM folders WHERE codebase_id = $1 AND path = $2`, codebaseID, path); err != nil {
			return fmt.Errorf("delete folder %s: %w", path, err)
		}
	}
	return nil
}

// UpsertFileIndex creates or updates a file index.
func (r *SQLRepository) UpsertFileIndex(ctx context.Context, file *FileIndex) error {
	// Use INSERT ON CONFLICT on the unique constraint (codebase_id, path).
	// Note: DuckDB does not allow updating columns with FK/PK/INDEX constraints
	// in ON CONFLICT DO UPDATE, so folder_id is excluded from the update set.
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO file_indexes (id, codebase_id, folder_id, path, name, extension, language,
			size_bytes, line_count, summary, purpose, key_exports, dependencies, content_hash, indexed_at, embedding)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (codebase_id, path) DO UPDATE SET
			name = EXCLUDED.name,
			extension = EXCLUDED.extension,
			language = EXCLUDED.language,
			size_bytes = EXCLUDED.size_bytes,
			line_count = EXCLUDED.line_count,
			summary = EXCLUDED.summary,
			purpose = EXCLUDED.purpose,
			key_exports = EXCLUDED.key_exports,
			dependencies = EXCLUDED.dependencies,
			content_hash = EXCLUDED.content_hash,
			indexed_at = EXCLUDED.indexed_at,
			embedding = EXCLUDED.embedding`,
		file.ID, file.CodebaseID, NullString(file.FolderID), file.Path, file.Name, NullString(file.Extension),
		NullString(file.Language), file.SizeBytes, file.LineCount, NullString(file.Summary), NullString(file.Purpose),
		ToJSON(file.KeyExports), ToJSON(file.Dependencies), NullString(file.ContentHash), NullTime(file.IndexedAt),
		NullString(EmbeddingToJSON(file.Embedding)))
	if err != nil {
		return fmt.Errorf("upsert file index: %w", err)
	}

	return nil
}

// GetFilesByCodebase retrieves all files for a codebase.
func (r *SQLRepository) GetFilesByCodebase(ctx context.Context, codebaseID string) ([]FileIndex, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, codebase_id, folder_id, path, name, extension, language,
			size_bytes, line_count, summary, purpose, key_exports, dependencies, content_hash, indexed_at, embedding
		FROM file_indexes WHERE codebase_id = $1 ORDER BY path`, codebaseID)
	if err != nil {
		return nil, fmt.Errorf("query files: %w", err)
	}
	defer rows.Close()
	return r.scanFileIndexes(rows)
}

// ExistingFileInfo holds minimal info for incremental indexing.
type ExistingFileInfo struct {
	ID          string
	Path        string
	ContentHash string
	Summary     string
}

// GetExistingFileHashes returns file hashes for change detection.
func (r *SQLRepository) GetExistingFileHashes(ctx context.Context, codebaseID string) (map[string]ExistingFileInfo, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, path, content_hash, summary FROM file_indexes WHERE codebase_id = $1`, codebaseID)
	if err != nil {
		return nil, fmt.Errorf("query file hashes: %w", err)
	}
	defer rows.Close()
	result := make(map[string]ExistingFileInfo)
	for rows.Next() {
		var info ExistingFileInfo
		var contentHash, summary sql.NullString
		if err := rows.Scan(&info.ID, &info.Path, &contentHash, &summary); err != nil {
			return nil, fmt.Errorf("scan file hash: %w", err)
		}
		info.ContentHash = contentHash.String
		info.Summary = summary.String
		result[info.Path] = info
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate file hashes: %w", err)
	}
	return result, nil
}

// DeleteFileIndex deletes a file index.
func (r *SQLRepository) DeleteFileIndex(ctx context.Context, codebaseID, path string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM file_indexes WHERE codebase_id = $1 AND path = $2`, codebaseID, path); err != nil {
		return fmt.Errorf("delete file index: %w", err)
	}
	return nil
}

// DeleteFileIndexesByPaths deletes file indexes by paths.
func (r *SQLRepository) DeleteFileIndexesByPaths(ctx context.Context, codebaseID string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	for _, path := range paths {
		if _, err := r.db.ExecContext(ctx, `DELETE FROM file_indexes WHERE codebase_id = $1 AND path = $2`, codebaseID, path); err != nil {
			return fmt.Errorf("delete file index %s: %w", path, err)
		}
	}
	return nil
}

// GetFilesByFolder retrieves files in a folder.
func (r *SQLRepository) GetFilesByFolder(ctx context.Context, folderID string) ([]FileIndex, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, codebase_id, folder_id, path, name, extension, language,
			size_bytes, line_count, summary, purpose, key_exports, dependencies, content_hash, indexed_at, embedding
		FROM file_indexes WHERE folder_id = $1 ORDER BY name`, folderID)
	if err != nil {
		return nil, fmt.Errorf("query files by folder: %w", err)
	}
	defer rows.Close()
	return r.scanFileIndexes(rows)
}

func (r *SQLRepository) scanFileIndexes(rows *sql.Rows) ([]FileIndex, error) {
	var files []FileIndex
	for rows.Next() {
		f := FileIndex{}
		var folderID, extension, language, summary, purpose, contentHash, embeddingJSON sql.NullString
		var keyExports, deps any
		var indexedAt sql.NullTime
		if err := rows.Scan(&f.ID, &f.CodebaseID, &folderID, &f.Path, &f.Name, &extension, &language,
			&f.SizeBytes, &f.LineCount, &summary, &purpose, &keyExports, &deps, &contentHash, &indexedAt, &embeddingJSON); err != nil {
			return nil, fmt.Errorf("scan file index: %w", err)
		}
		f.FolderID = folderID.String
		f.Extension = extension.String
		f.Language = language.String
		f.Summary = summary.String
		f.Purpose = purpose.String
		f.KeyExports = convertToStringSlice(keyExports)
		f.Dependencies = convertToStringSlice(deps)
		f.ContentHash = contentHash.String
		f.Embedding = EmbeddingFromJSON(embeddingJSON.String)
		if indexedAt.Valid {
			f.IndexedAt = indexedAt.Time
		}
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate file indexes: %w", err)
	}
	return files, nil
}

// SearchFilesBySummary searches files by summary text.
func (r *SQLRepository) SearchFilesBySummary(ctx context.Context, codebaseID, query string) ([]FileIndex, error) {
	searchPattern := "%" + query + "%"
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, codebase_id, folder_id, path, name, extension, language,
			size_bytes, line_count, summary, purpose, key_exports, dependencies, content_hash, indexed_at, embedding
		FROM file_indexes WHERE codebase_id = $1 AND (summary ILIKE $2 OR purpose ILIKE $2 OR name ILIKE $2)
		ORDER BY path LIMIT 20`, codebaseID, searchPattern)
	if err != nil {
		return nil, fmt.Errorf("search files: %w", err)
	}
	defer rows.Close()
	return r.scanFileIndexes(rows)
}

// CodebaseStats holds statistics for a codebase.
type CodebaseStats struct {
	FolderCount int64
	FileCount   int64
	TotalSize   int64
	TotalLines  int64
	Languages   map[string]int
}

// GetCodebaseStats returns statistics for a codebase.
func (r *SQLRepository) GetCodebaseStats(ctx context.Context, codebaseID string) (*CodebaseStats, error) {
	stats := &CodebaseStats{Languages: make(map[string]int)}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM folders WHERE codebase_id = $1`, codebaseID).Scan(&stats.FolderCount); err != nil {
		return nil, fmt.Errorf("count folders: %w", err)
	}
	
	// Query file stats with explicit handling
	var fileCount sql.NullInt64
	var totalSize sql.NullInt64
	var totalLines sql.NullInt64
	
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) as file_count, 
		       COALESCE(SUM(size_bytes), 0) as total_size, 
		       COALESCE(SUM(line_count), 0) as total_lines
		FROM file_indexes WHERE codebase_id = $1`, codebaseID).Scan(&fileCount, &totalSize, &totalLines); err != nil {
		return nil, fmt.Errorf("get file stats: %w", err)
	}
	
	if fileCount.Valid {
		stats.FileCount = fileCount.Int64
	}
	if totalSize.Valid {
		stats.TotalSize = totalSize.Int64
	}
	if totalLines.Valid {
		stats.TotalLines = totalLines.Int64
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT language, COUNT(*) as count FROM file_indexes
		WHERE codebase_id = $1 AND language IS NOT NULL AND language != ''
		GROUP BY language ORDER BY count DESC`, codebaseID)
	if err != nil {
		return nil, fmt.Errorf("query language stats: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var lang string
		var count int
		if err := rows.Scan(&lang, &count); err != nil {
			return nil, fmt.Errorf("scan language stat: %w", err)
		}
		stats.Languages[lang] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate language stats: %w", err)
	}
	return stats, nil
}

// HasEmbeddings checks if any files in the codebase have embeddings.
func (r *SQLRepository) HasEmbeddings(ctx context.Context, codebaseID string) bool {
	var count int64
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM file_indexes WHERE codebase_id = $1 AND embedding IS NOT NULL AND embedding != ''`,
		codebaseID).Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}

// SemanticSearchFiles searches files using cosine similarity against embeddings.
func (r *SQLRepository) SemanticSearchFiles(ctx context.Context, codebaseID string, queryEmbedding []float32, limit int) ([]FileIndex, error) {
	// Load all files with embeddings
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, codebase_id, folder_id, path, name, extension, language,
			size_bytes, line_count, summary, purpose, key_exports, dependencies, content_hash, indexed_at, embedding
		FROM file_indexes WHERE codebase_id = $1 AND embedding IS NOT NULL AND embedding != ''`, codebaseID)
	if err != nil {
		return nil, fmt.Errorf("query files for semantic search: %w", err)
	}
	defer rows.Close()

	files, err := r.scanFileIndexes(rows)
	if err != nil {
		return nil, err
	}

	// Compute cosine similarity and rank
	type scored struct {
		file  FileIndex
		score float64
	}
	var results []scored
	for _, f := range files {
		if len(f.Embedding) > 0 {
			sim := cosineSimilarity(queryEmbedding, f.Embedding)
			results = append(results, scored{file: f, score: sim})
		}
	}

	// Sort by similarity (highest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// Return top N
	var out []FileIndex
	for i, r := range results {
		if i >= limit {
			break
		}
		out = append(out, r.file)
	}
	return out, nil
}

// SemanticSearchFolders searches folders using cosine similarity against embeddings.
func (r *SQLRepository) SemanticSearchFolders(ctx context.Context, codebaseID string, queryEmbedding []float32, limit int) ([]Folder, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, codebase_id, path, name, depth, parent_path, summary, purpose, file_count, indexed_at, embedding
		FROM folders WHERE codebase_id = $1 AND embedding IS NOT NULL AND embedding != ''`, codebaseID)
	if err != nil {
		return nil, fmt.Errorf("query folders for semantic search: %w", err)
	}
	defer rows.Close()

	folders, err := r.scanFolders(rows)
	if err != nil {
		return nil, err
	}

	type scored struct {
		folder Folder
		score  float64
	}
	var results []scored
	for _, f := range folders {
		if len(f.Embedding) > 0 {
			sim := cosineSimilarity(queryEmbedding, f.Embedding)
			results = append(results, scored{folder: f, score: sim})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	var out []Folder
	for i, r := range results {
		if i >= limit {
			break
		}
		out = append(out, r.folder)
	}
	return out, nil
}

// cosineSimilarity computes cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// ExecuteQuery executes a raw SQL query without parameters.
func (r *SQLRepository) ExecuteQuery(ctx context.Context, query string) ([]map[string]any, error) {
	return r.ExecuteQueryWithArgs(ctx, query)
}

// ExecuteQueryWithArgs executes a SQL query with optional parameters.
func (r *SQLRepository) ExecuteQueryWithArgs(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}
	var results []map[string]any
	for rows.Next() {
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		row := make(map[string]any)
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return results, nil
}
