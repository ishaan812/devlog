# DevLog Codebase Tour

Welcome to the DevLog codebase! This guide will walk you through the architecture and help you understand how everything fits together.

## What is DevLog?

DevLog is a CLI tool that turns your git history into a queryable knowledge base. You can:
- Ask questions about your commits in plain English
- Generate work logs automatically
- Search your codebase semantically

## Project Structure

```
devlog/
├── cmd/devlog/          # Application entry point
├── internal/            # Private application code
│   ├── cli/             # Command-line interface (Cobra commands)
│   ├── db/              # Database layer (DuckDB)
│   ├── llm/             # LLM provider integrations
│   ├── chat/            # Question-answering pipeline
│   ├── git/             # Git repository operations
│   ├── indexer/         # Codebase scanning & summarization
│   ├── config/          # Configuration management
│   └── tui/             # Terminal UI components (Bubble Tea)
├── guides/              # User documentation
└── Makefile             # Build and development commands
```

---

## Layer-by-Layer Walkthrough

### 1. Entry Point (`cmd/devlog/main.go`)

The simplest file in the project - just calls the CLI:

```go
func main() {
    if err := cli.Execute(); err != nil {
        os.Exit(1)
    }
}
```

All the real work happens in `internal/`.

---

### 2. CLI Layer (`internal/cli/`)

This is where user commands are defined using [Cobra](https://github.com/spf13/cobra).

#### Key Files:

| File | Purpose |
|------|---------|
| `root.go` | Root command, global flags, profile setup |
| `ingest.go` | Scans git history and indexes codebase |
| `ask.go` | Natural language queries about commits |
| `worklog.go` | Generates markdown work logs |
| `search.go` | Semantic and keyword search |
| `branch.go` | Branch management commands |
| `profile.go` | Profile management (switch, create, delete) |
| `graph.go` | Generate codebase visualizations |
| `onboard.go` | First-time setup wizard |
| `clear.go` | Clear database data |
| `list.go` | List profiles and stats |

#### How Commands Work:

Each command follows this pattern:

```go
var myCmd = &cobra.Command{
    Use:   "mycommand",
    Short: "Brief description",
    Long:  `Detailed description...`,
    RunE:  runMyCommand,  // The actual implementation
}

func init() {
    rootCmd.AddCommand(myCmd)
    myCmd.Flags().StringVar(&someFlag, "flag", "default", "description")
}

func runMyCommand(cmd *cobra.Command, args []string) error {
    // 1. Get database repository
    dbRepo, err := db.GetRepository()
    
    // 2. Do the work
    // ...
    
    // 3. Return nil on success, error on failure
    return nil
}
```

---

### 3. Database Layer (`internal/db/`)

DevLog uses [DuckDB](https://duckdb.org/) - an embedded analytical database (like SQLite but optimized for analytics).

#### Key Files:

| File | Purpose |
|------|---------|
| `schema.go` | SQL table definitions |
| `models.go` | Go struct definitions for database entities |
| `repository.go` | Repository pattern implementation |
| `db.go` | Connection management and global helpers |

#### The Repository Pattern

Instead of writing raw SQL everywhere, we use a `Repository` interface:

```go
// The interface defines what operations are available
type Repository interface {
    GetCodebaseByPath(ctx context.Context, path string) (*Codebase, error)
    UpsertCommit(ctx context.Context, commit *Commit) error
    // ... more methods
}

// SQLRepository implements the interface
type SQLRepository struct {
    db *sql.DB
}

// Usage in CLI commands:
dbRepo, _ := db.GetRepository()
codebase, _ := dbRepo.GetCodebaseByPath(ctx, "/path/to/repo")
```

**Why this pattern?**
- Testability: Easy to mock for unit tests
- Consistency: All database operations go through one place
- Context propagation: Every method takes `context.Context` for cancellation

#### Database Schema (Simplified)

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  codebases  │────<│   branches  │────<│   commits   │
│             │     │             │     │             │
│ id          │     │ id          │     │ id          │
│ path        │     │ codebase_id │     │ branch_id   │
│ name        │     │ name        │     │ hash        │
│ summary     │     │ is_default  │     │ message     │
└─────────────┘     │ summary     │     │ summary     │
                    └─────────────┘     │ author_email│
                                        └──────┬──────┘
                                               │
                                        ┌──────┴──────┐
                                        │file_changes │
                                        │             │
                                        │ commit_id   │
                                        │ file_path   │
                                        │ additions   │
                                        │ deletions   │
                                        └─────────────┘
```

Additional tables:
- `developers` - Track who made commits
- `folders` - Folder structure and summaries
- `file_indexes` - Individual file metadata and summaries
- `ingest_cursors` - Track last ingested commit per branch (for incremental updates)

---

### 4. LLM Layer (`internal/llm/`)

DevLog supports multiple LLM providers through a common interface.

#### Key Files:

| File | Purpose |
|------|---------|
| `client.go` | Interface definition and factory functions |
| `ollama.go` | Ollama (local) implementation |
| `openai.go` | OpenAI API implementation |
| `anthropic.go` | Anthropic Claude implementation |
| `bedrock.go` | AWS Bedrock implementation |
| `embeddings.go` | Text embedding generation |

#### The Client Interface

```go
// All LLM providers implement this interface
type Client interface {
    Complete(ctx context.Context, prompt string) (string, error)
    ChatComplete(ctx context.Context, messages []Message) (string, error)
}

// Creating a client:
client, err := llm.NewClient(llm.Config{
    Provider: llm.ProviderOllama,
    Model:    "llama3.2",
})

// Using it:
response, err := client.Complete(ctx, "Summarize this commit...")
```

#### Functional Options Pattern

For flexible configuration:

```go
// Instead of many constructor parameters...
client := llm.NewOllamaClientWithOptions(
    llm.WithModel("codellama"),
    llm.WithBaseURL("http://localhost:11434"),
)
```

---

### 5. Chat Pipeline (`internal/chat/`)

This is the "brain" that answers natural language questions.

#### How It Works:

```
User Question: "What did I work on last week?"
                        │
                        ▼
            ┌───────────────────┐
            │  1. Generate SQL  │  LLM converts question to SQL
            └─────────┬─────────┘
                      │
            ┌─────────▼─────────┐
            │  2. Execute Query │  Run SQL against DuckDB
            └─────────┬─────────┘
                      │
            ┌─────────▼─────────┐
            │  3. Summarize     │  LLM summarizes results
            └─────────┬─────────┘
                      │
                      ▼
            Human-readable answer
```

#### Key Files:

| File | Purpose |
|------|---------|
| `pipeline.go` | Orchestrates the question-answering flow |
| `prompts.go` | LLM prompt templates |

---

### 6. Git Layer (`internal/git/`)

Handles all git repository operations.

#### Key Files:

| File | Purpose |
|------|---------|
| `repo.go` | Repository wrapper, branch operations |
| `walker.go` | Iterates through commit history |
| `blame.go` | File blame/ownership analysis |

#### Example Usage:

```go
// Open a repository
repo, err := git.OpenRepository("/path/to/repo")

// Get all branches
branches, err := repo.ListBranches()

// Walk commits
walker, _ := git.NewWalker(repo)
for walker.Next() {
    commit := walker.Commit()
    // Process commit...
}
```

---

### 7. Indexer (`internal/indexer/`)

Scans and summarizes codebase files.

#### Key Files:

| File | Purpose |
|------|---------|
| `scanner.go` | Walks directory tree, detects languages |
| `summarizer.go` | Uses LLM to generate file/folder summaries |

#### The Scanning Process:

```
1. Walk directory tree (skip .git, node_modules, etc.)
2. For each file:
   - Detect language by extension
   - Read content (limited to avoid huge files)
   - Generate summary using LLM
   - Store in database
3. For each folder:
   - Count files
   - Generate purpose summary
```

---

### 8. Configuration (`internal/config/`)

Manages user settings and profiles.

#### Config File Location:

```
~/.devlog/
├── config.yaml      # User configuration
├── default.db       # Default profile database
└── profiles/
    ├── work.db      # "work" profile database
    └── personal.db  # "personal" profile database
```

#### Profile System:

Profiles let you maintain separate databases for different contexts:

```yaml
# ~/.devlog/config.yaml
active_profile: work
default_provider: ollama

profiles:
  work:
    description: "Work projects"
    repos:
      - /Users/me/work/project-a
      - /Users/me/work/project-b
  personal:
    description: "Personal projects"
    repos:
      - /Users/me/personal/my-app
```

---

### 9. TUI Components (`internal/tui/`)

Interactive terminal UIs using [Bubble Tea](https://github.com/charmbracelet/bubbletea).

#### Key Files:

| File | Purpose |
|------|---------|
| `onboard.go` | Setup wizard UI |
| `branch_select.go` | Branch selection interface |

#### Bubble Tea Pattern:

```go
// Model holds UI state
type Model struct {
    step    int
    cursor  int
    choices []string
}

// Init runs on startup
func (m Model) Init() tea.Cmd { return nil }

// Update handles input
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "up":
            m.cursor--
        case "down":
            m.cursor++
        case "enter":
            // Handle selection
        }
    }
    return m, nil
}

// View renders the UI
func (m Model) View() string {
    return "Choose an option:\n" + renderChoices(m.choices, m.cursor)
}
```

---

## Data Flow Examples

### Example 1: `devlog ingest`

```
User runs: devlog ingest

1. CLI (ingest.go)
   └── Get repository path
   └── Open git repository (git/repo.go)
   └── Get database repository (db/repository.go)
   
2. Git History Ingestion
   └── Walk commits (git/walker.go)
   └── For each commit:
       └── Extract file changes
       └── Generate summary (if LLM enabled)
       └── Store in database (db/repository.go)

3. Codebase Indexing
   └── Scan files (indexer/scanner.go)
   └── Generate summaries (indexer/summarizer.go)
   └── Store in database
```

### Example 2: `devlog ask "What did I do yesterday?"`

```
User runs: devlog ask "What did I do yesterday?"

1. CLI (ask.go)
   └── Load config
   └── Create LLM client (llm/client.go)
   └── Create pipeline (chat/pipeline.go)

2. Pipeline.Ask()
   └── GenerateSQL() - LLM converts question to SQL
   └── ExecuteQuery() - Run SQL on DuckDB
   └── Summarize() - LLM creates human-readable response

3. Display result to user
```

---

## Key Patterns Used

### 1. Repository Pattern
Database operations go through interfaces, not direct SQL.

### 2. Functional Options
Flexible configuration without huge constructors.

### 3. Context Propagation
Every operation takes `context.Context` for cancellation and timeouts.

### 4. Interface-Based Design
LLM providers, database operations - all use interfaces for flexibility.

### 5. Incremental Processing
Ingest cursors track progress so re-runs only process new commits.

---

## Development Workflow

```bash
# Build
make build

# Run directly
go run ./cmd/devlog <command>

# Format code
make fmt

# Run linter
make lint

# Install dev tools
make tools
```

---

## Adding a New Command

1. Create `internal/cli/mycommand.go`
2. Define the cobra command
3. Register it in `init()` with `rootCmd.AddCommand()`
4. Implement the `RunE` function

```go
package cli

var myCmd = &cobra.Command{
    Use:   "mycommand",
    Short: "Does something cool",
    RunE:  runMyCommand,
}

func init() {
    rootCmd.AddCommand(myCmd)
}

func runMyCommand(cmd *cobra.Command, args []string) error {
    ctx := context.Background()
    dbRepo, err := db.GetRepository()
    if err != nil {
        return err
    }
    
    // Your logic here...
    
    return nil
}
```

---

## Adding a New LLM Provider

1. Create `internal/llm/myprovider.go`
2. Implement the `Client` interface
3. Add provider constant to `client.go`
4. Add case to `NewClient()` factory function

```go
type MyProviderClient struct {
    apiKey string
    model  string
}

func (c *MyProviderClient) Complete(ctx context.Context, prompt string) (string, error) {
    // Make API call...
}

func (c *MyProviderClient) ChatComplete(ctx context.Context, messages []Message) (string, error) {
    // Make API call...
}
```

---

## Deep Dive: Git Crawl & Insights

### The Git Walker: Parallel Commit Processing

When you run `devlog ingest`, the git walker uses a **worker pool pattern** to process commits in parallel:

```
                    ┌─────────────┐
                    │ Git History │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │   Collect   │  Gather all commit hashes
                    │   Commits   │  (stop at cursor or date)
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │ Worker Pool │  CPU_COUNT workers
                    │  (Parallel) │  process commits
                    └──────┬──────┘
                           │
            ┌──────────────┼──────────────┐
            │              │              │
      ┌─────▼─────┐  ┌─────▼─────┐  ┌─────▼─────┐
      │ Worker 1  │  │ Worker 2  │  │ Worker N  │
      │  Process  │  │  Process  │  │  Process  │
      │  Commit   │  │  Commit   │  │  Commit   │
      └─────┬─────┘  └─────┬─────┘  └─────┬─────┘
            │              │              │
            └──────────────┼──────────────┘
                           │
                    ┌──────▼──────┐
                    │   Results   │
                    │   Channel   │
                    └─────────────┘
```

#### Key Code (`git/walker.go`):

```go
// Start N workers (default: number of CPUs)
for i := 0; i < opts.Workers; i++ {
    go func() {
        for commit := range jobs {
            info, err := processCommit(commit)
            results <- info
        }
    }()
}

// Feed commits to workers
for _, commit := range commits {
    jobs <- commit
}
```

**Why parallel?** Processing 1000+ commits sequentially is slow. Parallel processing can speed this up by 4-8x on modern machines.

### What Insights Do We Extract?

For each commit, we extract rich metadata:

#### 1. Basic Metadata
```go
CommitInfo {
    Hash:        "a1b2c3d4..."
    Message:     "Add user authentication"
    AuthorName:  "John Doe"
    AuthorEmail: "john@example.com"
    CommittedAt: 2024-01-15T10:30:00Z
}
```

#### 2. Commit Statistics
```go
CommitStats {
    TotalAdditions: 245,     // Lines added
    TotalDeletions: 82,      // Lines removed
    FilesChanged:   15,      // Number of files touched
}
```

#### 3. File-Level Changes
For each file in the commit:
```go
FileChangeInfo {
    FilePath:   "src/auth/login.go"
    ChangeType: "modify"           // add, delete, rename, modify
    Additions:  42                 // Lines added in this file
    Deletions:  18                 // Lines removed
    Patch:      "diff --git..."   // Full diff (truncated if >10KB)
}
```

**Change Type Detection:**
```go
switch {
case change.From.Name == "":
    ChangeType = "add"      // New file
case change.To.Name == "":
    ChangeType = "delete"   // File removed
case change.From.Name != change.To.Name:
    ChangeType = "rename"   // File moved/renamed
default:
    ChangeType = "modify"   // File edited
}
```

#### 4. LLM-Generated Summaries

For **your commits** (identified by email or GitHub username), DevLog can generate natural language summaries:

```
Commit: "fix auth bug + update tests"
↓ (LLM analyzes message + file changes)
Summary: "Fixed authentication token expiration bug and updated related unit tests"
```

This happens in `ingestBranch()`:
```go
if isUserCommit && llmClient != nil {
    summary, _ := generateCommitSummary(llmClient, message, fileChanges)
}
```

---

### Blame Analysis: Understanding Code Ownership

DevLog uses **git blame** to categorize commits as new features, refactors, or fixes.

#### The Heuristic (`git/blame.go`):

```go
func categorizeChange(previousAuthors map[string]int, currentAuthor string) string {
    totalLines := sum(previousAuthors)
    selfLines := previousAuthors[currentAuthor]
    selfRatio := selfLines / totalLines

    if selfRatio > 0.7:
        return "refactor"  // Mostly your own code
    if selfRatio < 0.3:
        return "fix"       // Mostly others' code
    return "refactor"
}
```

**Example:**

File: `auth.go` (200 lines)
- Alice wrote 180 lines
- Bob wrote 20 lines

When Alice makes a commit changing 50 lines in `auth.go`:
- Self-ratio: 180/200 = 0.9 (90%)
- **Category: refactor** (working on her own code)

When Bob makes a commit changing 50 lines in `auth.go`:
- Self-ratio: 20/200 = 0.1 (10%)
- **Category: fix** (working on others' code)

**Why this matters:**
- "refactor" commits → improving existing features
- "fix" commits → debugging/maintaining others' code
- "new_feature" commits → no previous blame data (new file)

---

### Incremental Ingestion: Cursors & Smart Updates

DevLog doesn't re-process commits you've already ingested. It uses **ingest cursors**.

#### How Cursors Work:

```
First run:
  Ingest commits A → B → C → D
  Save cursor: D

Second run:
  Check cursor: D (already have A, B, C, D)
  Find new commits: E → F → G
  Ingest only E, F, G
  Update cursor: G
```

**Database:**
```sql
CREATE TABLE ingest_cursors (
    codebase_id VARCHAR,
    branch_name VARCHAR,
    last_commit_hash VARCHAR,  -- Cursor: last processed commit
    updated_at TIMESTAMP
)
```

**Code logic:**
```go
lastHash := dbRepo.GetBranchCursor(ctx, codebaseID, branchName)

for _, hash := range commitHashes {
    if hash == lastHash {
        break  // Stop at cursor
    }
    // Process new commit...
}

dbRepo.UpdateBranchCursor(ctx, codebaseID, branchName, latestHash)
```

#### Branch-Aware Ingestion

DevLog handles branches intelligently:

- **Main branch:** Ingests all commits
- **Feature branches:** Only ingests commits **unique to that branch** (not on main)

```go
if isDefault {
    commits = repo.GetCommitsOnBranch(branchName, "")
} else {
    commits = repo.GetCommitsOnBranch(branchName, baseBranch)
}
```

This uses `git log main..feature-branch` internally to find divergent commits.

---

## Deep Dive: Codebase Indexing

### The Indexing Pipeline

```
1. SCAN
   ├─ Walk directory tree
   ├─ Skip ignored dirs (.git, node_modules, etc.)
   ├─ Detect file language by extension
   ├─ Read file content (if <100KB)
   └─ Calculate SHA256 hash

2. CHANGE DETECTION
   ├─ Compare hashes with existing indexes
   ├─ Categorize: new, changed, unchanged, deleted
   └─ Skip unchanged files (incremental)

3. SUMMARIZATION (LLM)
   ├─ Generate file summaries (parallel)
   ├─ Generate folder summaries
   └─ Generate codebase overview

4. EMBEDDING GENERATION (optional)
   ├─ Convert summaries to vectors
   └─ Enable semantic search
```

### File Scanning: What We Look For

#### Language Detection (40+ languages):
```go
var languageMap = map[string]string{
    ".go":   "Go",
    ".py":   "Python",
    ".js":   "JavaScript",
    ".ts":   "TypeScript",
    ".rs":   "Rust",
    // ... 40+ more
}
```

#### Ignored Directories:
```go
var ignoredDirs = {
    ".git", "node_modules", "vendor",
    ".venv", "__pycache__", "dist",
    "build", "target", ".next",
    // ... prevents indexing build artifacts
}
```

#### Ignored File Types:
```go
var ignoredExtensions = {
    ".png", ".jpg", ".pdf", ".zip",
    ".mp3", ".exe", ".dll",
    // ... skip binary/media files
}
```

### Content Hashing: Incremental Intelligence

DevLog uses **SHA256 hashing** to detect file changes:

```go
// First index
file: "auth.go", hash: "a1b2c3..."  → Generate summary

// Second index (unchanged)
file: "auth.go", hash: "a1b2c3..."  → Skip (hash matches)

// Third index (modified)
file: "auth.go", hash: "d4e5f6..."  → Re-generate summary
```

**Benefits:**
- Skip unchanged files (saves time & LLM costs)
- Detect renames (same hash, different path)
- Track content evolution

### LLM Summarization: Three Levels

#### 1. File-Level Summaries

**Prompt:**
```
Analyze this code file:
File: src/auth/login.go
Language: Go
Content: (first 2000 chars)

Respond with:
1. SUMMARY: One-sentence description
2. PURPOSE: Main purpose (e.g., "API handler")
3. EXPORTS: Key functions (comma-separated)
```

**Output:**
```
SUMMARY: Handles user login and JWT token generation
PURPOSE: Authentication handler
EXPORTS: HandleLogin, ValidateCredentials, GenerateToken
```

Stored in `file_indexes` table:
```sql
INSERT INTO file_indexes (
    path, summary, purpose, key_exports, ...
)
```

#### 2. Folder-Level Summaries

**Prompt:**
```
Folder: src/auth/
Files: login.go, logout.go, middleware.go
Subfolders: providers/, tokens/

Respond with:
1. SUMMARY: What this folder contains
2. PURPOSE: Main purpose
```

**Output:**
```
SUMMARY: Contains authentication handlers and middleware
PURPOSE: Authentication system
```

#### 3. Codebase-Level Summary

**Prompt:**
```
Name: myapp
Tech Stack: Go (45%), JavaScript (30%), Python (25%)
Main Folders: cmd/, internal/, web/, scripts/
Total Files: 234

Respond with:
1. SUMMARY: 2-3 sentences about the project
2. TECH: Primary technologies
```

**Output:**
```
SUMMARY: A REST API service for task management built with Go.
         Uses PostgreSQL for storage and React for the frontend.
         Follows clean architecture with domain-driven design.
TECH: Go, PostgreSQL, React, Docker, GitHub Actions
```

### Tech Stack Detection

DevLog automatically detects your stack by analyzing files:

```go
func DetectTechStack(files []FileInfo) map[string]int {
    stack := {}
    
    // By extension
    for file in files:
        if file.extension == ".go":
            stack["Go"]++
        if file.extension == ".py":
            stack["Python"]++
    
    // By filename
    if file.name == "package.json":
        stack["Node.js"]++
    if file.name == "Dockerfile":
        stack["Docker"]++
    if file.name == "go.mod":
        stack["Go"]++
    
    return stack
}
```

**Result:**
```
{
    "Go": 45,
    "Python": 12,
    "Docker": 2,
    "Node.js": 1
}
```

### Semantic Search: Vector Embeddings

When you enable embeddings (via `--skip-embeddings=false`), DevLog:

1. **Generates embeddings** for file summaries using LLM:
   ```
   Summary: "Handles user login"
   ↓ (embedding model)
   Vector: [0.123, -0.456, 0.789, ...]  (768 dimensions)
   ```

2. **Stores vectors** in the database

3. **Enables semantic search**:
   ```bash
   devlog search "authentication logic"
   ```
   
   Converts your query to a vector, finds nearest neighbors:
   ```
   Query vector: [0.145, -0.423, 0.801, ...]
   ↓ (cosine similarity)
   Matches: login.go (0.92), auth.go (0.88), middleware.go (0.85)
   ```

**Why this matters:**
- "authentication" matches "login", "security", "credentials"
- Searches by **meaning**, not just keywords
- Works across languages

---

## Performance Optimizations

### 1. Parallel Processing
- Git commits: N workers (default: CPU count)
- File summarization: Concurrent goroutines
- **Speedup:** 4-8x faster than sequential

### 2. Incremental Updates
- Commit cursors: Skip already-processed commits
- Content hashing: Skip unchanged files
- **Speedup:** Second runs are 10-100x faster

### 3. Smart Batching
- Database inserts use transactions
- LLM calls are batched where possible
- **Result:** Fewer network round-trips

### 4. Selective Summarization
- Only summarize **your commits** by default
- Option to skip summaries (`--skip-summaries`)
- Option to skip embeddings (`--skip-embeddings`)
- **Result:** Lower LLM costs

### 5. Content Limits
- File content: First 2000 chars for summaries
- Patches: Truncated at 10KB
- Large files: Skipped entirely (configurable)
- **Result:** Bounded memory usage

---

## Questions?

- Check `guides/` for user documentation
- Read the README.md for usage examples
- Look at existing commands in `internal/cli/` for patterns

Happy coding!
