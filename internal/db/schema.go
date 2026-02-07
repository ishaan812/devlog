package db

// Schema defines the DuckDB table schema
const Schema = `
-- Developers table
CREATE TABLE IF NOT EXISTS developers (
    id VARCHAR PRIMARY KEY,
    name VARCHAR NOT NULL,
    email VARCHAR UNIQUE NOT NULL,
    is_current_user BOOLEAN DEFAULT FALSE
);

-- Codebases table
CREATE TABLE IF NOT EXISTS codebases (
    id VARCHAR PRIMARY KEY,
    path VARCHAR UNIQUE NOT NULL,
    name VARCHAR NOT NULL,
    summary VARCHAR,
    tech_stack JSON,
    default_branch VARCHAR,
    indexed_at TIMESTAMP
);

-- Branches table
CREATE TABLE IF NOT EXISTS branches (
    id VARCHAR PRIMARY KEY,
    codebase_id VARCHAR NOT NULL REFERENCES codebases(id),
    name VARCHAR NOT NULL,
    is_default BOOLEAN DEFAULT FALSE,
    base_branch VARCHAR,
    summary VARCHAR,
    story VARCHAR,
    status VARCHAR DEFAULT 'active',
    first_commit_hash VARCHAR,
    last_commit_hash VARCHAR,
    commit_count INTEGER DEFAULT 0,
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    UNIQUE(codebase_id, name)
);

-- Commits table
CREATE TABLE IF NOT EXISTS commits (
    id VARCHAR PRIMARY KEY,
    hash VARCHAR NOT NULL,
    codebase_id VARCHAR NOT NULL REFERENCES codebases(id),
    branch_id VARCHAR REFERENCES branches(id),
    author_email VARCHAR NOT NULL,
    message VARCHAR NOT NULL,
    summary VARCHAR,
    committed_at TIMESTAMP NOT NULL,
    stats JSON,
    is_user_commit BOOLEAN DEFAULT FALSE,
    is_on_default_branch BOOLEAN DEFAULT FALSE,
    UNIQUE(codebase_id, hash)
);

-- File changes table
CREATE TABLE IF NOT EXISTS file_changes (
    id VARCHAR PRIMARY KEY,
    commit_id VARCHAR NOT NULL REFERENCES commits(id),
    file_path VARCHAR NOT NULL,
    change_type VARCHAR NOT NULL,
    additions INTEGER DEFAULT 0,
    deletions INTEGER DEFAULT 0,
    patch VARCHAR
);

-- Folders table
CREATE TABLE IF NOT EXISTS folders (
    id VARCHAR PRIMARY KEY,
    codebase_id VARCHAR NOT NULL REFERENCES codebases(id),
    path VARCHAR NOT NULL,
    name VARCHAR NOT NULL,
    depth INTEGER NOT NULL,
    parent_path VARCHAR,
    summary VARCHAR,
    purpose VARCHAR,
    file_count INTEGER DEFAULT 0,
    indexed_at TIMESTAMP,
    UNIQUE(codebase_id, path)
);

-- File indexes table
CREATE TABLE IF NOT EXISTS file_indexes (
    id VARCHAR PRIMARY KEY,
    codebase_id VARCHAR NOT NULL REFERENCES codebases(id),
    folder_id VARCHAR REFERENCES folders(id),
    path VARCHAR NOT NULL,
    name VARCHAR NOT NULL,
    extension VARCHAR,
    language VARCHAR,
    size_bytes BIGINT,
    line_count INTEGER,
    summary VARCHAR,
    purpose VARCHAR,
    key_exports JSON,
    dependencies JSON,
    content_hash VARCHAR,
    indexed_at TIMESTAMP,
    UNIQUE(codebase_id, path)
);

-- Ingest cursors table
CREATE TABLE IF NOT EXISTS ingest_cursors (
    id VARCHAR PRIMARY KEY,
    codebase_id VARCHAR NOT NULL REFERENCES codebases(id),
    branch_name VARCHAR NOT NULL,
    last_commit_hash VARCHAR NOT NULL,
    updated_at TIMESTAMP,
    UNIQUE(codebase_id, branch_name)
);

-- Create indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_commits_codebase ON commits(codebase_id);
CREATE INDEX IF NOT EXISTS idx_commits_branch ON commits(branch_id);
CREATE INDEX IF NOT EXISTS idx_commits_author ON commits(author_email);
CREATE INDEX IF NOT EXISTS idx_commits_date ON commits(committed_at);
CREATE INDEX IF NOT EXISTS idx_commits_user ON commits(is_user_commit);
CREATE INDEX IF NOT EXISTS idx_file_changes_commit ON file_changes(commit_id);
CREATE INDEX IF NOT EXISTS idx_file_changes_path ON file_changes(file_path);
CREATE INDEX IF NOT EXISTS idx_branches_codebase ON branches(codebase_id);
CREATE INDEX IF NOT EXISTS idx_folders_codebase ON folders(codebase_id);
CREATE INDEX IF NOT EXISTS idx_file_indexes_codebase ON file_indexes(codebase_id);
`
