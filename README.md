# DevLog

> Your AI-powered development activity tracker and code intelligence tool.

DevLog transforms your git history and codebase into a queryable knowledge base. Automaically generate professional work logs, Ask questions in natural language,  and search your code semantically—all from the command line.

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/go-%3E%3D1.21-blue.svg)

## Why DevLog?

- **"What did I work on last week?"** — Get instant summaries without digging through git logs or having to traverse through different repos and branches
- **"Where is the authentication logic?"** — Semantic search finds code by meaning, not just keywords
- **Generate standup reports** — Create professional markdown work logs in seconds
- **Privacy-first** — Run locally with Ollama, or use cloud providers
- **Multi-repo Multi-branch support** — Profiles keep work and personal projects separate

## Features

| Feature | Description |
|---------|-------------|
| **Natural Language Queries** | Ask questions about your git history in plain English |
| **Semantic Code Search** | Find code by describing what it does |
| **Smart Work Logs** | Auto-generated markdown summaries organized by branch and date |
| **Multiple LLM Providers** | Ollama (local), Anthropic, OpenAI, or AWS Bedrock |
| **Profile System** | Isolated databases for different work contexts |
| **Branch-aware Ingestion** | Remembers your branch selections per repo |
| **Incremental Updates** | Fast re-runs that only process new commits |
| **Codebase Visualization** | Generate structure diagrams and collaboration graphs |

## Quick Start

```bash
# 1. Install DevLog
go install github.com/ishaan812/devlog/cmd/devlog@latest

# 2. Run the setup wizard
devlog onboard

# 3. Ingest your repository
cd ~/projects/myapp
devlog ingest

# 4. Start asking questions!
devlog worklog --days 7
devlog ask "What did I work on this week?"
devlog search "error handling"
```

## Installation

### Using Go Install (Recommended)

```bash
go install github.com/ishaan812/devlog/cmd/devlog@latest
```

### From Source

```bash
git clone https://github.com/ishaan812/devlog
cd devlog
make install
```

### Verify Installation

```bash
devlog --help
```

## Getting Started

### 1. Initial Setup

Run the onboarding wizard to configure your LLM provider:

```bash
devlog onboard
```

This will:
- Set up your preferred LLM provider (Ollama recommended for privacy)
- Configure your user identity for commit tracking
- Create your default profile

### 2. Ingest a Repository

Navigate to your project and run:

```bash
cd ~/projects/myapp
devlog ingest
```

On first run, you'll select which branches to track. DevLog remembers your selection:
```
  Saved branch selection:
    Main: main
    Branches: main, develop, feature/auth

  [Enter] Use current  [m] Modify  [r] Reselect all: 
```

### 3. Generate Work Logs

```bash
devlog worklog --days 7
```

Generates a professional markdown work log:

### 4. Query Your Activity

```bash
# Ask natural language questions
devlog ask "What features did I add this month?"
devlog ask "Show me commits related to the payment system"
devlog ask "Which files have I changed the most?"

# Semantic code search
devlog search "database connection pooling"
devlog search "user authentication flow"
```

```markdown
# Work Log - yourname

*Generated on February 4, 2026*

# Monday, February 3, 2026

## Branch: feature/auth

### Updates
- Implemented JWT-based authentication with refresh tokens
- Added role-based access control middleware

### Commits
- **14:30** `a1b2c3d` Add JWT authentication (+250/-30)
- **11:15** `d4e5f6g` Implement RBAC middleware (+180/-20)
```

## Commands Reference

### `devlog ingest`

Ingest git history and index your codebase for search.

```bash
devlog ingest                      # Current directory
devlog ingest ~/projects/myapp     # Specific path
devlog ingest --days 90            # Last 90 days (default: 30)
devlog ingest --all                # Full git history
devlog ingest --git-only           # Skip codebase indexing
devlog ingest --index-only         # Skip git history
devlog ingest --reselect-branches  # Re-select branches
devlog ingest --all-branches       # Ingest all branches
devlog ingest --fill-summaries     # Generate missing commit summaries
```

### `devlog ask`

Query your development activity using natural language.

```bash
devlog ask "What did I work on this week?"
devlog ask "Show commits about authentication"
devlog ask "Which files changed the most in January?"
devlog ask --provider anthropic "Summarize my recent work"
```

### `devlog search`

Semantic search across your indexed codebase.

```bash
devlog search "authentication logic"
devlog search "database queries"
devlog search -n 20 "error handling"    # More results
```

### `devlog worklog`

Generate formatted work logs from your commit history.

```bash
devlog worklog                     # Default: last 7 days
devlog worklog --days 30           # Last 30 days
devlog worklog -o report.md        # Custom output file
devlog worklog --no-llm            # Skip AI summaries
devlog worklog --group-by date     # Group by date instead of branch
```

### `devlog profile`

Manage isolated profiles for different work contexts.

```bash
devlog profile                     # Show current profile
devlog profile list                # List all profiles
devlog profile create work "Work projects"
devlog profile use work            # Switch profiles
devlog profile repos               # List repos in profile
devlog profile delete old          # Delete a profile
```

Use a profile temporarily:
```bash
devlog --profile work ingest ~/work/project
devlog --profile personal ask "What did I do this weekend?"
```

### `devlog graph`

Visualize your codebase structure.

```bash
devlog graph                       # Folder structure
devlog graph --type commits        # Commit activity
devlog graph --type files          # File change heatmap
devlog graph --type collab         # Developer collaboration
devlog graph -o diagram.md         # Export as Mermaid markdown
```

### `devlog list`

List profiles and repositories.

```bash
devlog list                        # All profiles with stats
devlog list repos                  # Repos in current profile
```

### `devlog clear`

Clear database data for the current profile.

```bash
devlog clear                       # Interactive confirmation
devlog clear --force               # Skip confirmation
```

## LLM Providers

DevLog supports multiple LLM providers. Choose based on your needs:

| Provider | Privacy | Cost | Best For |
|----------|---------|------|----------|
| **Ollama** | 🟢 Local | Free | Privacy-conscious users |
| **Anthropic** | 🟡 Cloud | Paid | Best quality responses |
| **OpenAI** | 🟡 Cloud | Paid | Wide model selection |
| **Bedrock** | 🟡 Cloud | Paid | AWS-integrated workflows |

### Setting Up Ollama (Recommended)

1. Install Ollama from [ollama.ai](https://ollama.ai)
2. Pull a model:
   ```bash
   ollama pull llama3.2
   ```
3. Start the server:
   ```bash
   ollama serve
   ```
4. Run DevLog setup:
   ```bash
   devlog onboard
   ```

### Using Cloud Providers

Set your API key during onboarding or in `~/.devlog/config.json`:

```json
{
  "default_provider": "anthropic",
  "anthropic_api_key": "sk-ant-..."
}
```

Or use environment variables:
```bash
export ANTHROPIC_API_KEY="sk-ant-..."
export OPENAI_API_KEY="sk-..."
```

## Configuration

DevLog stores all data in `~/.devlog/`:

```
~/.devlog/
├── config.json              # Settings, profiles, API keys
└── profiles/
    ├── default/
    │   └── devlog.db        # SQLite database
    └── work/
        └── devlog.db
```

### Config Options

| Setting | Description | Default |
|---------|-------------|---------|
| `default_provider` | LLM provider | `ollama` |
| `ollama_model` | Model for Ollama | `llama3.2` |
| `ollama_base_url` | Ollama server URL | `http://localhost:11434` |
| `user_email` | Your git email | Auto-detected |
| `github_username` | GitHub username | Optional |

## Tips & Tricks

### Faster Ingestion

Skip AI summaries for quick ingestion:
```bash
devlog ingest --skip-summaries --skip-commit-summaries
```

### Generate Summaries Later

Fill in missing summaries after fast ingestion:
```bash
devlog ingest --fill-summaries
```

### Multiple Repos

Ingest multiple repos into one profile:
```bash
devlog ingest ~/projects/frontend
devlog ingest ~/projects/backend
devlog ingest ~/projects/shared-lib
```

### Work vs Personal

Keep work and personal projects separate:
```bash
devlog profile create work "Work projects"
devlog profile use work
devlog ingest ~/work/project1

devlog profile use default  # Back to personal
```

## Shell Completion

Enable tab completion for your shell:

```bash
# Bash
devlog completion bash > /etc/bash_completion.d/devlog

# Zsh
devlog completion zsh > "${fpath[1]}/_devlog"

# Fish
devlog completion fish > ~/.config/fish/completions/devlog.fish
```

## Development

```bash
# Clone the repo
git clone https://github.com/ishaan812/devlog
cd devlog

# Build
make build

# Run tests
make test

# Install locally
make install

# Run development commands
make run-ingest
make run-ask
make run-worklog
```

## Documentation

- [Quick Start Guide](guides/quickstart.md)
- [LLM Provider Setup](guides/providers.md)
- [Ollama Setup Guide](guides/ollama-setup.md)

## Troubleshooting

### "No commits found"
Make sure you've run `devlog ingest` first:
```bash
devlog ingest
```

### "LLM not available"
Check your provider is configured:
```bash
devlog onboard  # Re-run setup
```

For Ollama, ensure the server is running:
```bash
ollama serve
```

### Slow ingestion
Skip AI summaries for faster initial ingestion:
```bash
devlog ingest --skip-summaries
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

MIT License - see [LICENSE](LICENSE) for details.

---

**Made with ❤️ for developers who want to understand their work better.**
