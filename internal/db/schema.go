package db

import (
	"database/sql"
	"fmt"
)

const schemaSQL = `
-- Core tables
CREATE TABLE IF NOT EXISTS developer (
    id VARCHAR PRIMARY KEY,
    name VARCHAR NOT NULL,
    email VARCHAR UNIQUE NOT NULL
);

CREATE TABLE IF NOT EXISTS commit (
    hash VARCHAR PRIMARY KEY,
    repo_path VARCHAR NOT NULL,
    message TEXT NOT NULL,
    author_email VARCHAR NOT NULL,
    committed_at TIMESTAMP NOT NULL,
    stats JSON,
    FOREIGN KEY (author_email) REFERENCES developer(email)
);

CREATE TABLE IF NOT EXISTS file_change (
    id VARCHAR PRIMARY KEY,
    commit_hash VARCHAR NOT NULL,
    file_path VARCHAR NOT NULL,
    change_type VARCHAR NOT NULL,
    additions INTEGER DEFAULT 0,
    deletions INTEGER DEFAULT 0,
    patch TEXT,
    FOREIGN KEY (commit_hash) REFERENCES commit(hash)
);

CREATE TABLE IF NOT EXISTS ingest_cursor (
    repo_path VARCHAR PRIMARY KEY,
    last_hash VARCHAR NOT NULL,
    updated_at TIMESTAMP
);

-- Codebase index tables
CREATE TABLE IF NOT EXISTS codebase (
    id VARCHAR PRIMARY KEY,
    path VARCHAR UNIQUE NOT NULL,
    name VARCHAR NOT NULL,
    indexed_at TIMESTAMP,
    summary TEXT,
    tech_stack JSON
);

CREATE TABLE IF NOT EXISTS folder (
    id VARCHAR PRIMARY KEY,
    codebase_id VARCHAR NOT NULL,
    path VARCHAR NOT NULL,
    name VARCHAR NOT NULL,
    depth INTEGER NOT NULL,
    parent_path VARCHAR,
    summary TEXT,
    purpose VARCHAR,
    file_count INTEGER DEFAULT 0,
    embedding FLOAT[],
    indexed_at TIMESTAMP,
    FOREIGN KEY (codebase_id) REFERENCES codebase(id),
    UNIQUE(codebase_id, path)
);

CREATE TABLE IF NOT EXISTS file_index (
    id VARCHAR PRIMARY KEY,
    codebase_id VARCHAR NOT NULL,
    folder_id VARCHAR,
    path VARCHAR NOT NULL,
    name VARCHAR NOT NULL,
    extension VARCHAR,
    language VARCHAR,
    size_bytes INTEGER,
    line_count INTEGER,
    summary TEXT,
    purpose VARCHAR,
    key_exports JSON,
    dependencies JSON,
    embedding FLOAT[],
    content_hash VARCHAR,
    indexed_at TIMESTAMP,
    FOREIGN KEY (codebase_id) REFERENCES codebase(id),
    FOREIGN KEY (folder_id) REFERENCES folder(id),
    UNIQUE(codebase_id, path)
);

-- Relationship tables for graph
CREATE TABLE IF NOT EXISTS file_dependency (
    id VARCHAR PRIMARY KEY,
    source_file_id VARCHAR NOT NULL,
    target_file_id VARCHAR NOT NULL,
    dependency_type VARCHAR NOT NULL,
    FOREIGN KEY (source_file_id) REFERENCES file_index(id),
    FOREIGN KEY (target_file_id) REFERENCES file_index(id)
);

CREATE TABLE IF NOT EXISTS developer_collaboration (
    developer1_email VARCHAR NOT NULL,
    developer2_email VARCHAR NOT NULL,
    shared_files INTEGER DEFAULT 0,
    shared_commits INTEGER DEFAULT 0,
    last_collaboration TIMESTAMP,
    PRIMARY KEY (developer1_email, developer2_email)
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_commit_author ON commit(author_email);
CREATE INDEX IF NOT EXISTS idx_commit_date ON commit(committed_at);
CREATE INDEX IF NOT EXISTS idx_commit_repo ON commit(repo_path);
CREATE INDEX IF NOT EXISTS idx_file_change_commit ON file_change(commit_hash);
CREATE INDEX IF NOT EXISTS idx_file_change_path ON file_change(file_path);
CREATE INDEX IF NOT EXISTS idx_folder_codebase ON folder(codebase_id);
CREATE INDEX IF NOT EXISTS idx_folder_path ON folder(path);
CREATE INDEX IF NOT EXISTS idx_file_index_codebase ON file_index(codebase_id);
CREATE INDEX IF NOT EXISTS idx_file_index_folder ON file_index(folder_id);
CREATE INDEX IF NOT EXISTS idx_file_index_language ON file_index(language);
`

const graphSchemaSQL = `
-- Create property graph (DuckPGQ)
CREATE OR REPLACE PROPERTY GRAPH devlog_graph
VERTEX TABLES (
    developer PROPERTIES (id, name, email) LABEL Developer,
    commit PROPERTIES (hash, message, committed_at) LABEL Commit,
    file_index PROPERTIES (id, path, name, language, summary) LABEL File,
    folder PROPERTIES (id, path, name, summary) LABEL Folder
)
EDGE TABLES (
    commit SOURCE KEY (author_email) REFERENCES developer (email)
           DESTINATION KEY (hash) REFERENCES commit (hash)
           LABEL AUTHORED,
    file_change SOURCE KEY (commit_hash) REFERENCES commit (hash)
                DESTINATION KEY (file_path) REFERENCES file_index (path)
                LABEL CHANGED,
    file_index SOURCE KEY (folder_id) REFERENCES folder (id)
               DESTINATION KEY (id) REFERENCES file_index (id)
               LABEL CONTAINS,
    file_dependency SOURCE KEY (source_file_id) REFERENCES file_index (id)
                    DESTINATION KEY (target_file_id) REFERENCES file_index (id)
                    LABEL DEPENDS_ON
);
`

func CreateSchema(db *sql.DB) error {
	_, err := db.Exec(schemaSQL)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}
	return nil
}

func CreateGraphSchema(db *sql.DB) error {
	_, err := db.Exec(graphSchemaSQL)
	if err != nil {
		// Graph schema is optional, log but don't fail
		return fmt.Errorf("failed to create graph schema (DuckPGQ may not be available): %w", err)
	}
	return nil
}

func GetSchemaDescription() string {
	return `Tables in the database:

1. developer(id, name, email)
   - Stores developer information

2. commit(hash, repo_path, message, author_email, committed_at, stats)
   - Stores git commits with author reference and statistics

3. file_change(id, commit_hash, file_path, change_type, additions, deletions, patch)
   - Stores individual file changes within commits

4. codebase(id, path, name, indexed_at, summary, tech_stack)
   - Stores indexed codebase metadata

5. folder(id, codebase_id, path, name, depth, summary, purpose, embedding)
   - Stores folder summaries with vector embeddings

6. file_index(id, codebase_id, path, name, language, summary, purpose, embedding)
   - Stores file summaries with vector embeddings for semantic search

7. file_dependency(source_file_id, target_file_id, dependency_type)
   - Tracks import/dependency relationships between files

8. developer_collaboration(developer1_email, developer2_email, shared_files)
   - Tracks collaboration patterns between developers`
}
