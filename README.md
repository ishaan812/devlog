# DevLog

Track and query your development activity using natural language.

DevLog analyzes your git history and codebase, letting you ask questions like "What did I work on this week?" or search your code with natural language queries.

## Features

- **Natural Language Queries**: Ask questions about your git history in plain English
- **Semantic Code Search**: Find code by describing what it does, not just keyword matching
- **Profiles**: Maintain separate databases for personal and work projects
- **Work Logs**: Generate markdown summaries of your development activity
- **Multiple LLM Providers**: Ollama (local), Anthropic, OpenAI, or AWS Bedrock
- **Incremental Ingestion**: Fast re-runs that only process new commits

## Quick Start

```bash
# Install
go install github.com/ishaan812/devlog/cmd/devlog@latest

# Run setup wizard
devlog onboard

# Ingest a repository
cd ~/projects/myapp
devlog ingest

# Ask questions
devlog ask "What did I work on this week?"

# Search code
devlog search "authentication"
```

## Installation

### From Source

```bash
git clone https://github.com/ishaan812/devlog
cd devlog
go install ./cmd/devlog
```

### Using Go Install

```bash
go install github.com/ishaan812/devlog/cmd/devlog@latest
```

## Configuration

DevLog stores configuration and data in `~/.devlog/`:

```
~/.devlog/
├── config.json              # Global config + profiles
└── profiles/
    ├── default/
    │   └── devlog.db        # Default profile database
    └── work/
        └── devlog.db        # Work profile database
```

## Commands

### Ingest

Ingest git history and index codebase for a repository:

```bash
devlog ingest [path]         # Full ingest (git + codebase index)
devlog ingest --git-only     # Only git history
devlog ingest --index-only   # Only codebase indexing
devlog ingest --all          # Full git history (not just last 30 days)
devlog ingest --days 90      # Last 90 days
devlog ingest --skip-summaries  # Skip LLM summaries (faster)
```

### Ask

Query your git history using natural language:

```bash
devlog ask "What did I work on this week?"
devlog ask "Which files have I changed the most?"
devlog ask "Show commits about authentication"
devlog ask --provider anthropic "..."  # Use specific provider
```

### Search

Search indexed codebase using natural language:

```bash
devlog search "authentication logic"
devlog search "database connection"
devlog search -n 20 "error handling"  # More results
```

### Profile Management

Profiles isolate data between different work contexts:

```bash
devlog profile              # Show current profile
devlog profile list         # List all profiles
devlog profile create work "Work projects"
devlog profile use work     # Switch to work profile
devlog profile repos        # List repos in current profile
devlog profile delete old   # Delete a profile
```

Use `--profile` flag to temporarily use a different profile:

```bash
devlog --profile work ingest ~/work/project
devlog --profile personal ask "What did I do last weekend?"
```

### Work Logs

Generate markdown summaries of development activity:

```bash
devlog worklog --days 7              # Last week
devlog worklog --days 30 -o log.md   # Export to file
devlog worklog --no-llm              # Skip LLM summaries
```

### Graph

Visualize codebase structure:

```bash
devlog graph                         # Structure diagram
devlog graph --type commits          # Commit activity
devlog graph --type collab           # Developer collaboration
devlog graph -o graph.md             # Export to file
```

## LLM Providers

DevLog supports multiple LLM providers:

| Provider | Local | Cost | Setup |
|----------|-------|------|-------|
| Ollama | Yes | Free | Install Ollama, pull a model |
| Anthropic | No | Paid | Get API key from console.anthropic.com |
| OpenAI | No | Paid | Get API key from platform.openai.com |
| Bedrock | No | Paid | AWS credentials with Bedrock access |

### Ollama Setup (Recommended for Privacy)

```bash
# Install Ollama (https://ollama.ai)
# Start the server
ollama serve

# Pull a model
ollama pull llama3.2

# DevLog will use Ollama by default
devlog onboard  # Select Ollama
```

## Global Flags

| Flag | Description |
|------|-------------|
| `--profile` | Use specific profile for this command |
| `--verbose, -v` | Enable debug output |
| `--db` | Custom database path (overrides profile) |

## Documentation

- [Quick Start Guide](guides/quickstart.md)
- [LLM Provider Setup](guides/providers.md)
- [Ollama Setup](guides/ollama-setup.md)

## Development

```bash
# Build
make build

# Run tests
make test

# Install locally
make install
```

## License

MIT
