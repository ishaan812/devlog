# DevLog Quick Start

Get up and running with DevLog in under 5 minutes.

## 1. Install

```bash
go install github.com/ishaan812/devlog/cmd/devlog@latest
```

Or build from source:
```bash
git clone https://github.com/ishaan812/devlog
cd devlog
make install
```

## 2. Run Setup Wizard

```bash
devlog onboard
```

The interactive wizard will guide you through:
- Creating your first profile
- Choosing an LLM provider
- Configuring API keys (if needed)
- Setting up your user info

## 3. Ingest Your First Repository

```bash
cd ~/projects/myapp
devlog ingest
```

This command:
1. Scans git history (last 30 days by default)
2. Indexes the codebase for semantic search
3. Adds the repo to your active profile

### Ingest Options

```bash
# Full git history
devlog ingest --all

# Last 90 days of history
devlog ingest --days 90

# Skip LLM summaries (faster)
devlog ingest --skip-summaries

# Only git history, no codebase indexing
devlog ingest --git-only

# Only codebase indexing, no git history
devlog ingest --index-only
```

## 4. Ask Questions

```bash
devlog ask "What did I work on this week?"
devlog ask "Which files have I changed the most?"
devlog ask "Show commits about authentication"
```

## 5. Search Code

```bash
devlog search "authentication logic"
devlog search "database connection"
```

## 6. Generate Work Logs

```bash
# Last 7 days
devlog worklog --days 7

# Export to markdown file
devlog worklog --days 30 --output worklog.md
```

## Profile Management

Profiles keep your data organized across different work contexts.

```bash
# Show current profile
devlog profile

# List all profiles
devlog profile list

# Create a new profile
devlog profile create work "Work projects"

# Switch to a profile
devlog profile use work

# See repos in active profile
devlog profile repos

# Delete a profile (--data to also delete database)
devlog profile delete old-profile --data
```

### Using Profiles

```bash
# Ingest to specific profile
devlog --profile work ingest ~/work/project

# Query specific profile
devlog --profile personal ask "What did I do last weekend?"
```

## Commands Reference

| Command | Description |
|---------|-------------|
| `devlog onboard` | Interactive setup wizard |
| `devlog ingest [path]` | Ingest git history and index codebase |
| `devlog cron [path]` | Set up daily ingest + worklog cron job |
| `devlog ask "question"` | Query your git history |
| `devlog search "query"` | Search indexed codebase |
| `devlog worklog` | Generate work summary |
| `devlog profile` | Manage profiles |
| `devlog graph` | Visualize codebase structure |

## Global Flags

| Flag | Description |
|------|-------------|
| `--profile` | Use specific profile |
| `--verbose, -v` | Show debug output |
| `--db` | Custom database path (overrides profile) |

## Next Steps

- See [providers.md](providers.md) for LLM provider setup
- See [ollama-setup.md](ollama-setup.md) for local Ollama setup
