# DevLog Storage Strategy

This document describes how DevLog stores configuration, profiles, and data.

## Directory Structure

All DevLog data is stored under `~/.devlog/`:

```
~/.devlog/
├── config.json                 # Global configuration file
├── worklogs/                   # Generated worklog files
│   ├── 2026-01-19.md
│   ├── 2026-01-12.md
│   └── ...
└── profiles/
    ├── default/
    │   └── devlog.db           # Default profile database
    ├── personal/
    │   └── devlog.db           # Personal profile database
    └── work/
        └── devlog.db           # Work profile database
```

## Configuration File

**Location:** `~/.devlog/config.json`

The configuration file stores:
- LLM provider settings
- API keys
- User information (name, email for commit filtering)
- Profile definitions
- Active profile reference

### Config Schema

```json
{
  "default_provider": "ollama",
  "default_model": "",

  "anthropic_api_key": "",
  "openai_api_key": "",

  "aws_region": "us-east-1",
  "aws_access_key_id": "",
  "aws_secret_access_key": "",

  "ollama_base_url": "http://localhost:11434",
  "ollama_model": "llama3.2",

  "user_name": "John Doe",
  "user_email": "john@example.com",

  "onboarding_complete": true,

  "profiles": {
    "default": {
      "name": "default",
      "description": "Default profile",
      "created_at": "2024-01-15T10:30:00Z",
      "repos": [
        "/Users/john/projects/app1",
        "/Users/john/projects/app2"
      ]
    }
  },
  "active_profile": "default"
}
```

**Important:** `user_email` is used to filter commits - only commits by this user get descriptions generated.

---

## Database Schema

**Engine:** DuckDB (embedded analytical database)

Each profile has its own isolated database at `~/.devlog/profiles/<name>/devlog.db`.

### Core Entities

#### 1. Codebase (Repository)

Represents an indexed repository.

```sql
CREATE TABLE codebase (
    id VARCHAR PRIMARY KEY,
    path VARCHAR UNIQUE NOT NULL,     -- Absolute path to repo root
    name VARCHAR NOT NULL,            -- Directory name
    summary TEXT,                     -- LLM: "What is this project?"
    tech_stack JSON,                  -- {"go": 45, "typescript": 30}
    default_branch VARCHAR,           -- "main" or "master"
    indexed_at TIMESTAMP,
    embedding FLOAT[]                 -- For semantic search
);
```

#### 2. Branch

Tracks branches with their story/description.

```sql
CREATE TABLE branch (
    id VARCHAR PRIMARY KEY,
    codebase_id VARCHAR NOT NULL,
    name VARCHAR NOT NULL,            -- "main", "feature/auth", etc.
    is_default BOOLEAN DEFAULT FALSE, -- Is this main/master?
    base_branch VARCHAR,              -- Parent branch name (null for default)
    summary TEXT,                     -- LLM: "What is this branch about?"
    status VARCHAR,                   -- "active", "merged", "stale"
    first_commit_hash VARCHAR,
    last_commit_hash VARCHAR,
    commit_count INTEGER DEFAULT 0,   -- Commits unique to this branch
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    embedding FLOAT[],                -- For semantic search
    FOREIGN KEY (codebase_id) REFERENCES codebase(id),
    UNIQUE(codebase_id, name)
);
```

**Branch Ingestion Strategy:**
- Default branch (main/master): Ingest all commits
- Feature branches: Only ingest commits NOT on the default branch
- This avoids duplicate commit storage

#### 3. Folder

Folders with descriptions for semantic search.

```sql
CREATE TABLE folder (
    id VARCHAR PRIMARY KEY,
    codebase_id VARCHAR NOT NULL,
    path VARCHAR NOT NULL,            -- Relative path: "internal/cli"
    name VARCHAR NOT NULL,            -- "cli"
    depth INTEGER NOT NULL,
    parent_path VARCHAR,
    summary TEXT,                     -- LLM: "What does this folder contain?"
    purpose VARCHAR,                  -- Short label: "CLI commands"
    file_count INTEGER DEFAULT 0,
    embedding FLOAT[],                -- For semantic search
    indexed_at TIMESTAMP,
    FOREIGN KEY (codebase_id) REFERENCES codebase(id),
    UNIQUE(codebase_id, path)
);
```

#### 4. File

Files with descriptions for semantic search.

```sql
CREATE TABLE file_index (
    id VARCHAR PRIMARY KEY,
    codebase_id VARCHAR NOT NULL,
    folder_id VARCHAR,
    path VARCHAR NOT NULL,            -- Relative path: "internal/cli/root.go"
    name VARCHAR NOT NULL,            -- "root.go"
    extension VARCHAR,
    language VARCHAR,                 -- "go", "typescript", etc.
    size_bytes INTEGER,
    line_count INTEGER,
    summary TEXT,                     -- LLM: "What does this file do?"
    purpose VARCHAR,                  -- Short label: "CLI root command"
    key_exports JSON,                 -- ["Execute", "rootCmd"]
    dependencies JSON,                -- ["cobra", "config"]
    embedding FLOAT[],                -- For semantic search
    content_hash VARCHAR,             -- For change detection
    indexed_at TIMESTAMP,
    FOREIGN KEY (codebase_id) REFERENCES codebase(id),
    FOREIGN KEY (folder_id) REFERENCES folder(id),
    UNIQUE(codebase_id, path)
);
```

#### 5. Developer

```sql
CREATE TABLE developer (
    id VARCHAR PRIMARY KEY,
    name VARCHAR NOT NULL,
    email VARCHAR UNIQUE NOT NULL,
    is_current_user BOOLEAN DEFAULT FALSE  -- Matches config.user_email
);
```

#### 6. Commit

Commits with user-specific descriptions.

```sql
CREATE TABLE commit (
    id VARCHAR PRIMARY KEY,           -- UUID (not git hash, for uniqueness across repos)
    hash VARCHAR NOT NULL,            -- Git commit hash
    codebase_id VARCHAR NOT NULL,
    branch_id VARCHAR,                -- Which branch this commit belongs to
    author_email VARCHAR NOT NULL,
    message TEXT NOT NULL,            -- Original commit message
    summary TEXT,                     -- LLM: "What did this commit do?" (only for user's commits)
    committed_at TIMESTAMP NOT NULL,
    stats JSON,                       -- {"additions": N, "deletions": N, "files_changed": N}
    is_user_commit BOOLEAN DEFAULT FALSE,  -- Is this by the current user?
    is_on_default_branch BOOLEAN DEFAULT FALSE,
    embedding FLOAT[],                -- For semantic search (only user commits)
    FOREIGN KEY (codebase_id) REFERENCES codebase(id),
    FOREIGN KEY (branch_id) REFERENCES branch(id),
    FOREIGN KEY (author_email) REFERENCES developer(email),
    UNIQUE(codebase_id, hash)
);
```

**Commit Processing:**
- All commits are stored for history
- Only commits where `author_email == config.user_email` get:
  - `is_user_commit = TRUE`
  - LLM-generated `summary`
  - Vector `embedding`

#### 7. File Change

```sql
CREATE TABLE file_change (
    id VARCHAR PRIMARY KEY,
    commit_id VARCHAR NOT NULL,
    file_path VARCHAR NOT NULL,
    change_type VARCHAR NOT NULL,     -- "add", "modify", "delete", "rename"
    additions INTEGER DEFAULT 0,
    deletions INTEGER DEFAULT 0,
    patch TEXT,                       -- Diff content
    FOREIGN KEY (commit_id) REFERENCES commit(id)
);
```

#### 8. Ingest Cursor

Tracks ingestion state per branch.

```sql
CREATE TABLE ingest_cursor (
    id VARCHAR PRIMARY KEY,
    codebase_id VARCHAR NOT NULL,
    branch_name VARCHAR NOT NULL,
    last_commit_hash VARCHAR NOT NULL,
    updated_at TIMESTAMP,
    FOREIGN KEY (codebase_id) REFERENCES codebase(id),
    UNIQUE(codebase_id, branch_name)
);
```

---

### Indexes

```sql
-- Commit queries
CREATE INDEX idx_commit_codebase ON commit(codebase_id);
CREATE INDEX idx_commit_branch ON commit(branch_id);
CREATE INDEX idx_commit_author ON commit(author_email);
CREATE INDEX idx_commit_date ON commit(committed_at);
CREATE INDEX idx_commit_user ON commit(is_user_commit);

-- File change queries
CREATE INDEX idx_file_change_commit ON file_change(commit_id);
CREATE INDEX idx_file_change_path ON file_change(file_path);

-- Codebase index queries
CREATE INDEX idx_folder_codebase ON folder(codebase_id);
CREATE INDEX idx_file_index_codebase ON file_index(codebase_id);
CREATE INDEX idx_file_index_folder ON file_index(folder_id);
CREATE INDEX idx_file_index_language ON file_index(language);

-- Branch queries
CREATE INDEX idx_branch_codebase ON branch(codebase_id);
CREATE INDEX idx_branch_status ON branch(status);
```

---

## Semantic Search

All searchable entities have `summary`, `embedding` fields.

### Searchable Entities

| Entity | What's Searchable |
|--------|-------------------|
| Codebase | Project description, tech stack |
| Branch | Branch story/purpose |
| Folder | Folder description, purpose |
| File | File description, purpose, exports |
| Commit | Commit summary (user's commits only) |

### Search Filtering

Search can be scoped by:
- **Project**: `devlog search "auth" --project myapp`
- **Folder**: `devlog search "auth" --folder internal/auth`
- **File**: `devlog search "auth" --file auth.go`
- **Branch**: `devlog search "auth" --branch feature/login`
- **Date range**: `devlog search "auth" --since 2026-01-01`

### Vector Search Query

```sql
SELECT
    'commit' as type,
    c.hash,
    c.summary,
    list_cosine_similarity(c.embedding, ?) as score
FROM commit c
WHERE c.codebase_id = ?
  AND c.is_user_commit = TRUE
  AND c.embedding IS NOT NULL
  AND c.committed_at >= ?

UNION ALL

SELECT
    'file' as type,
    f.path,
    f.summary,
    list_cosine_similarity(f.embedding, ?) as score
FROM file_index f
WHERE f.codebase_id = ?
  AND f.embedding IS NOT NULL

ORDER BY score DESC
LIMIT 20;
```

---

## Branch Strategy

### Ingestion Flow

1. **Detect default branch** (main/master)
2. **Ingest default branch first** - all commits
3. **For each feature branch:**
   - Find merge-base with default branch
   - Only ingest commits after merge-base (unique to branch)
   - Generate branch summary from those commits

### Branch Commit Ownership

```
main:     A---B---C---D---E
               \
feature:        X---Y---Z
```

- Commits A, B, C, D, E → stored under `main` branch
- Commits X, Y, Z → stored under `feature` branch (not duplicated)

### Listing Branches

```bash
devlog branch list              # All branches with summaries
devlog branch list --active     # Only active branches
devlog branch show feature/auth # Show branch details + commits
```

---

## Worklog Generation

### Storage

Worklogs are saved to `~/.devlog/worklogs/` as markdown files.

### Format

```markdown
# Worklog

**Date:** 19/01/2026
**Period:** Last 7 days
**Author:** John Doe

---

## Updates

- Implemented user authentication flow with JWT tokens
  - Branch: `feature/auth` | Commits: `a1b2c3d`, `e4f5g6h`
- Fixed database connection pooling issue
  - Branch: `main` | Commits: `i7j8k9l`
- Added rate limiting middleware
  - Branch: `feature/rate-limit` | Commits: `m0n1o2p`, `q3r4s5t`

---

## Branches Worked On

| Branch | Status | Commits | Summary |
|--------|--------|---------|---------|
| `main` | active | 3 | Bug fixes and maintenance |
| `feature/auth` | active | 5 | JWT-based authentication system |
| `feature/rate-limit` | merged | 2 | API rate limiting |

---

## Detailed Changes

### feature/auth (5 commits)

**Story:** Implementing JWT-based authentication with refresh tokens...

| Date | Commit | Summary |
|------|--------|---------|
| 19/01 | `a1b2c3d` | Add JWT token generation |
| 18/01 | `e4f5g6h` | Implement refresh token flow |
| ... | ... | ... |

### main (3 commits)

| Date | Commit | Summary |
|------|--------|---------|
| 19/01 | `i7j8k9l` | Fix connection pool leak |
| ... | ... | ... |

---

*Generated by DevLog*
```

### CLI Commands

```bash
devlog worklog                  # Last 7 days (default)
devlog worklog --days 1         # Today only
devlog worklog --days 3         # Last 3 days
devlog worklog --days 30        # Last month
devlog worklog --output custom.md  # Custom output path
devlog worklog --no-save        # Print only, don't save to .worklog
```

### Generation Flow

1. Query all user commits in date range
2. Group by branch
3. Generate branch summaries (if not cached)
4. Aggregate into worklog format
5. Save to `~/.devlog/worklogs/{date}.md`

---

## Property Graph (Reserved)

The DuckPGQ extension setup is kept for future use but not actively used:

```sql
-- Schema kept for future graph queries
CREATE PROPERTY GRAPH devlog_graph
VERTEX TABLES (
    developer LABEL Developer,
    commit LABEL Commit,
    file_index LABEL File,
    folder LABEL Folder,
    branch LABEL Branch
)
EDGE TABLES (
    commit -> developer LABEL AUTHORED,
    commit -> branch LABEL ON_BRANCH,
    file_change -> commit, file_index LABEL CHANGED,
    file_index -> folder LABEL IN_FOLDER,
    branch -> codebase LABEL BELONGS_TO
);
```

---

## Data Flow Summary

```
Repository
    │
    ├── Branches (with stories)
    │       │
    │       └── Commits (user commits get summaries)
    │               │
    │               └── File Changes
    │
    ├── Folders (with descriptions)
    │       │
    │       └── Files (with descriptions)
    │
    └── Codebase metadata
```

### What Gets Summaries/Embeddings

| Entity | Summary | Embedding | Condition |
|--------|---------|-----------|-----------|
| Codebase | Yes | Yes | Always |
| Branch | Yes | Yes | Always |
| Folder | Yes | Yes | Depth ≤ 2 |
| File | Yes | Yes | Code files > 100 bytes |
| Commit | Yes | Yes | **Only user's commits** |

---

## Profile Isolation

Each profile has:
- Separate database file
- Own repo list
- Independent ingest cursors
- Isolated worklogs (stored globally but filtered by profile on generation)

---

## Migration

### From Pre-Profile Versions

If old `~/.devlog/devlog.db` exists:
1. Moved to `~/.devlog/profiles/default/devlog.db`
2. Default profile created
3. Schema migrated to add new tables (branch, etc.)

---

## Future Considerations

- [ ] Incremental codebase re-indexing (only changed files)
- [ ] Profile export/import
- [ ] Database compaction/optimization
- [ ] Remote profile sync
- [ ] Encrypted storage for API keys
- [ ] Graph-based queries (using DuckPGQ)
- [ ] Team collaboration features
