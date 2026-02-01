package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Codebase struct {
	ID        string
	Path      string
	Name      string
	IndexedAt time.Time
	Summary   string
	TechStack map[string]int
}

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
	Embedding  []float32
	IndexedAt  time.Time
}

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
	Embedding    []float32
	ContentHash  string
	IndexedAt    time.Time
}

// InsertCodebase inserts or updates a codebase record
func InsertCodebase(db *sql.DB, c Codebase) error {
	techStackJSON, _ := json.Marshal(c.TechStack)

	_, err := db.Exec(`
		INSERT INTO codebase (id, path, name, indexed_at, summary, tech_stack)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (path) DO UPDATE SET
			name = excluded.name,
			indexed_at = excluded.indexed_at,
			summary = excluded.summary,
			tech_stack = excluded.tech_stack
	`, c.ID, c.Path, c.Name, c.IndexedAt, c.Summary, string(techStackJSON))
	return err
}

// InsertFolder inserts or updates a folder record
func InsertFolder(db *sql.DB, f Folder) error {
	var embeddingStr *string
	if len(f.Embedding) > 0 {
		parts := make([]string, len(f.Embedding))
		for i, v := range f.Embedding {
			parts[i] = fmt.Sprintf("%f", v)
		}
		s := "[" + strings.Join(parts, ",") + "]"
		embeddingStr = &s
	}

	_, err := db.Exec(`
		INSERT INTO folder (id, codebase_id, path, name, depth, parent_path, summary, purpose, file_count, embedding, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (codebase_id, path) DO UPDATE SET
			name = excluded.name,
			depth = excluded.depth,
			parent_path = excluded.parent_path,
			summary = excluded.summary,
			purpose = excluded.purpose,
			file_count = excluded.file_count,
			embedding = excluded.embedding,
			indexed_at = excluded.indexed_at
	`, f.ID, f.CodebaseID, f.Path, f.Name, f.Depth, f.ParentPath, f.Summary, f.Purpose, f.FileCount, embeddingStr, f.IndexedAt)
	return err
}

// InsertFileIndex inserts or updates a file index record
func InsertFileIndex(db *sql.DB, f FileIndex) error {
	keyExportsJSON, _ := json.Marshal(f.KeyExports)
	depsJSON, _ := json.Marshal(f.Dependencies)

	var embeddingStr *string
	if len(f.Embedding) > 0 {
		parts := make([]string, len(f.Embedding))
		for i, v := range f.Embedding {
			parts[i] = fmt.Sprintf("%f", v)
		}
		s := "[" + strings.Join(parts, ",") + "]"
		embeddingStr = &s
	}

	_, err := db.Exec(`
		INSERT INTO file_index (id, codebase_id, folder_id, path, name, extension, language, size_bytes, line_count, summary, purpose, key_exports, dependencies, embedding, content_hash, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (codebase_id, path) DO UPDATE SET
			name = excluded.name,
			extension = excluded.extension,
			size_bytes = excluded.size_bytes,
			line_count = excluded.line_count,
			summary = excluded.summary,
			purpose = excluded.purpose,
			key_exports = excluded.key_exports,
			dependencies = excluded.dependencies,
			embedding = excluded.embedding,
			content_hash = excluded.content_hash,
			indexed_at = excluded.indexed_at
	`, f.ID, f.CodebaseID, f.FolderID, f.Path, f.Name, f.Extension, f.Language, f.SizeBytes, f.LineCount, f.Summary, f.Purpose, string(keyExportsJSON), string(depsJSON), embeddingStr, f.ContentHash, f.IndexedAt)
	return err
}

// GetCodebaseByPath retrieves a codebase by path
func GetCodebaseByPath(db *sql.DB, path string) (*Codebase, error) {
	var c Codebase
	var techStackJSON sql.NullString
	var summary sql.NullString
	err := db.QueryRow(`
		SELECT id, path, name, indexed_at, summary, CAST(tech_stack AS VARCHAR) as tech_stack
		FROM codebase WHERE path = ?
	`, path).Scan(&c.ID, &c.Path, &c.Name, &c.IndexedAt, &summary, &techStackJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if summary.Valid {
		c.Summary = summary.String
	}
	if techStackJSON.Valid {
		json.Unmarshal([]byte(techStackJSON.String), &c.TechStack)
	}
	return &c, nil
}

// GetFoldersByCodebase retrieves all folders for a codebase
func GetFoldersByCodebase(db *sql.DB, codebaseID string) ([]Folder, error) {
	rows, err := db.Query(`
		SELECT id, codebase_id, path, name, depth, parent_path, summary, purpose, file_count
		FROM folder WHERE codebase_id = ? ORDER BY depth, path
	`, codebaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folders []Folder
	for rows.Next() {
		var f Folder
		var parentPath sql.NullString
		if err := rows.Scan(&f.ID, &f.CodebaseID, &f.Path, &f.Name, &f.Depth, &parentPath, &f.Summary, &f.Purpose, &f.FileCount); err != nil {
			return nil, err
		}
		if parentPath.Valid {
			f.ParentPath = parentPath.String
		}
		folders = append(folders, f)
	}
	return folders, rows.Err()
}

// GetFilesByCodebase retrieves all files for a codebase
func GetFilesByCodebase(db *sql.DB, codebaseID string) ([]FileIndex, error) {
	rows, err := db.Query(`
		SELECT id, codebase_id, folder_id, path, name, extension, language, size_bytes, line_count, summary, purpose
		FROM file_index WHERE codebase_id = ? ORDER BY path
	`, codebaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []FileIndex
	for rows.Next() {
		var f FileIndex
		var folderID sql.NullString
		if err := rows.Scan(&f.ID, &f.CodebaseID, &folderID, &f.Path, &f.Name, &f.Extension, &f.Language, &f.SizeBytes, &f.LineCount, &f.Summary, &f.Purpose); err != nil {
			return nil, err
		}
		if folderID.Valid {
			f.FolderID = folderID.String
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// GetFilesByFolder retrieves all files in a folder
func GetFilesByFolder(db *sql.DB, folderID string) ([]FileIndex, error) {
	rows, err := db.Query(`
		SELECT id, codebase_id, folder_id, path, name, extension, language, size_bytes, line_count, summary, purpose
		FROM file_index WHERE folder_id = ? ORDER BY name
	`, folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []FileIndex
	for rows.Next() {
		var f FileIndex
		var folderID sql.NullString
		if err := rows.Scan(&f.ID, &f.CodebaseID, &folderID, &f.Path, &f.Name, &f.Extension, &f.Language, &f.SizeBytes, &f.LineCount, &f.Summary, &f.Purpose); err != nil {
			return nil, err
		}
		if folderID.Valid {
			f.FolderID = folderID.String
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// SearchFilesBySummary searches files by summary text
func SearchFilesBySummary(db *sql.DB, codebaseID, query string) ([]FileIndex, error) {
	rows, err := db.Query(`
		SELECT id, codebase_id, folder_id, path, name, extension, language, size_bytes, line_count, summary, purpose
		FROM file_index
		WHERE codebase_id = ? AND (summary ILIKE ? OR purpose ILIKE ? OR name ILIKE ?)
		ORDER BY path LIMIT 20
	`, codebaseID, "%"+query+"%", "%"+query+"%", "%"+query+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []FileIndex
	for rows.Next() {
		var f FileIndex
		var folderID sql.NullString
		if err := rows.Scan(&f.ID, &f.CodebaseID, &folderID, &f.Path, &f.Name, &f.Extension, &f.Language, &f.SizeBytes, &f.LineCount, &f.Summary, &f.Purpose); err != nil {
			return nil, err
		}
		if folderID.Valid {
			f.FolderID = folderID.String
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// SemanticSearchFiles searches files using vector similarity
func SemanticSearchFiles(db *sql.DB, codebaseID string, queryEmbedding []float32, limit int) ([]FileIndex, error) {
	if len(queryEmbedding) == 0 {
		return nil, fmt.Errorf("query embedding is empty")
	}

	// Convert embedding to string format for DuckDB
	parts := make([]string, len(queryEmbedding))
	for i, v := range queryEmbedding {
		parts[i] = fmt.Sprintf("%f", v)
	}
	embeddingStr := "[" + strings.Join(parts, ",") + "]"

	// Use cosine similarity: dot(a,b) / (norm(a) * norm(b))
	// DuckDB array functions: list_cosine_similarity available in newer versions
	// Fallback to manual calculation if needed
	rows, err := db.Query(`
		SELECT id, codebase_id, folder_id, path, name, extension, language, size_bytes, line_count, summary, purpose,
			list_cosine_similarity(embedding, ?::FLOAT[]) as similarity
		FROM file_index
		WHERE codebase_id = ? AND embedding IS NOT NULL
		ORDER BY similarity DESC
		LIMIT ?
	`, embeddingStr, codebaseID, limit)
	if err != nil {
		return nil, fmt.Errorf("semantic search failed: %w", err)
	}
	defer rows.Close()

	var files []FileIndex
	for rows.Next() {
		var f FileIndex
		var folderID sql.NullString
		var similarity float64
		if err := rows.Scan(&f.ID, &f.CodebaseID, &folderID, &f.Path, &f.Name, &f.Extension, &f.Language, &f.SizeBytes, &f.LineCount, &f.Summary, &f.Purpose, &similarity); err != nil {
			return nil, err
		}
		if folderID.Valid {
			f.FolderID = folderID.String
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// SemanticSearchFolders searches folders using vector similarity
func SemanticSearchFolders(db *sql.DB, codebaseID string, queryEmbedding []float32, limit int) ([]Folder, error) {
	if len(queryEmbedding) == 0 {
		return nil, fmt.Errorf("query embedding is empty")
	}

	// Convert embedding to string format for DuckDB
	parts := make([]string, len(queryEmbedding))
	for i, v := range queryEmbedding {
		parts[i] = fmt.Sprintf("%f", v)
	}
	embeddingStr := "[" + strings.Join(parts, ",") + "]"

	rows, err := db.Query(`
		SELECT id, codebase_id, path, name, depth, parent_path, summary, purpose, file_count,
			list_cosine_similarity(embedding, ?::FLOAT[]) as similarity
		FROM folder
		WHERE codebase_id = ? AND embedding IS NOT NULL
		ORDER BY similarity DESC
		LIMIT ?
	`, embeddingStr, codebaseID, limit)
	if err != nil {
		return nil, fmt.Errorf("semantic search failed: %w", err)
	}
	defer rows.Close()

	var folders []Folder
	for rows.Next() {
		var f Folder
		var parentPath sql.NullString
		var similarity float64
		if err := rows.Scan(&f.ID, &f.CodebaseID, &f.Path, &f.Name, &f.Depth, &parentPath, &f.Summary, &f.Purpose, &f.FileCount, &similarity); err != nil {
			return nil, err
		}
		if parentPath.Valid {
			f.ParentPath = parentPath.String
		}
		folders = append(folders, f)
	}
	return folders, rows.Err()
}

// HasEmbeddings checks if the codebase has any embeddings stored
func HasEmbeddings(db *sql.DB, codebaseID string) bool {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM file_index WHERE codebase_id = ? AND embedding IS NOT NULL", codebaseID).Scan(&count)
	return count > 0
}

// GetCodebaseStats returns statistics about an indexed codebase
func GetCodebaseStats(db *sql.DB, codebaseID string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	var folderCount, fileCount int
	var totalSize int64
	var totalLines int

	db.QueryRow("SELECT COUNT(*) FROM folder WHERE codebase_id = ?", codebaseID).Scan(&folderCount)
	db.QueryRow("SELECT COUNT(*), COALESCE(SUM(size_bytes), 0), COALESCE(SUM(line_count), 0) FROM file_index WHERE codebase_id = ?", codebaseID).Scan(&fileCount, &totalSize, &totalLines)

	stats["folder_count"] = folderCount
	stats["file_count"] = fileCount
	stats["total_size_bytes"] = totalSize
	stats["total_lines"] = totalLines

	// Language breakdown
	rows, err := db.Query(`
		SELECT language, COUNT(*) as count
		FROM file_index
		WHERE codebase_id = ? AND language IS NOT NULL AND language != ''
		GROUP BY language
		ORDER BY count DESC
	`, codebaseID)
	if err == nil {
		defer rows.Close()
		languages := make(map[string]int)
		for rows.Next() {
			var lang string
			var count int
			rows.Scan(&lang, &count)
			languages[lang] = count
		}
		stats["languages"] = languages
	}

	return stats, nil
}
