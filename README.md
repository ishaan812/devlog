<div align="center">

```
     _            _
  __| | _____   _| | ___   __ _
 / _` |/ _ \ \ / / |/ _ \ / _` |
| (_| |  __/\ V /| | (_) | (_| |
 \__,_|\___| \_/ |_|\___/ \__, |
                           |___/
```

### Stop forgetting what you shipped.

Open-source, local-first AI work logging for developers who juggle<br>too many repos, too many branches, and too many standups.

**[devlog.ishaan812.com](https://devlog.ishaan812.com)**

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.21-blue.svg)](https://go.dev)
[![Open Source](https://img.shields.io/badge/open%20source-%E2%9C%93-brightgreen.svg)](https://github.com/ishaan812/devlog)
[![npm](https://img.shields.io/badge/npm-%40ishaan812%2Fdevlog-red.svg)](https://www.npmjs.com/package/@ishaan812/devlog)

</div>

You ship code across 5 repos, 12 branches, and 3 teams. Monday morning standup hits and you're scrolling through `git log` trying to remember what you did last Thursday.

**DevLog fixes that.** It ingests your git history across every repo and branch you work on, and turns it into smart, structured work logs — automatically. No more "I think I worked on the auth thing?" Get your work summary in seconds.

---

### The 30-second pitch

```bash
npm install -g @ishaan812/devlog  # Install
devlog onboard              # Set up (works with free local Ollama)
devlog ingest               # Point it at your repos
devlog worklog --days 7     # Get your week's work, organized by branch
devlog console              # Browse worklogs in an interactive TUI
```

That's it. Professional markdown work logs, generated from your actual commits. Multi-repo, multi-branch, zero effort.

---

## Why Developers Love DevLog

### You context-switch constantly
You're on `feature/payments` in the morning, hotfixing `prod` after lunch, reviewing PRs on a backend service, then back to payments. DevLog tracks all of it — across repos and branches — so you don't have to.

### Your standups are painful
"What did I do yesterday?" shouldn't require 10 minutes of archaeology through git logs in 4 different terminals. DevLog generates a clean, branch-organized summary in one command.

### You work across multiple repos
Frontend, backend, shared libraries, infrastructure — DevLog ingests them all into one unified timeline. One command to see everything you shipped.

### You care about privacy (and your wallet)
DevLog is **local-first**. Run it with [Ollama](https://ollama.ai) and your data never leaves your machine. No subscriptions, no telemetry, no cloud dependency. Need better summaries? Plug in Anthropic, OpenAI, or OpenRouter for pennies per query.

---

## Features

| Feature | Description |
|---------|-------------|
| **Smart Work Logs** | Auto-generated markdown summaries organized by branch, date, and repo — ready for standups, PRs, or performance reviews |
| **Interactive Console** | Full-screen terminal UI to browse repos and navigate through your cached worklogs day-by-day |
| **Multi-Repo Ingestion** | Ingest as many repos as you want into a single profile. See your full picture. |
| **Multi-Branch Tracking** | Branch-aware ingestion remembers your selections per repo. Track `main`, `develop`, and every feature branch. |
| **Local-First AI** | Works completely offline with Ollama. Your code and history stay on your machine. |
| **Cheap Cloud Fallback** | Optionally use Anthropic, OpenAI, OpenRouter, or AWS Bedrock — most queries are very lightweight and would cost fractions of a cent |
| **Profile System** | Isolated databases for work vs. personal, or per-client contexts |
| **Incremental Updates** | Re-runs only process new commits. Fast even on large repos. |

## Quick Start

```bash
# 1. Install DevLog
npm install -g @ishaan812/devlog
# or: go install github.com/ishaan812/devlog/cmd/devlog@latest

# 2. Run the setup wizard
devlog onboard

# 3. Ingest your repositories
devlog ingest ~/projects/frontend
devlog ingest ~/projects/backend
devlog ingest ~/projects/shared-lib

# 4. See what you actually shipped
devlog worklog --days 7
```

## Installation

### Using npm (Easiest)

```bash
npm install -g @ishaan812/devlog
```

### Using Go Install

```bash
go install github.com/ishaan812/devlog/cmd/devlog@latest
```

**Note:** Make sure `$HOME/go/bin` (or `$GOPATH/bin`) is in your PATH. Add this to your shell config if needed:

```bash
# For bash (~/.bashrc) or zsh (~/.zshrc)
export PATH="$HOME/go/bin:$PATH"

# For fish (~/.config/fish/config.fish)
set -gx PATH $HOME/go/bin $PATH
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

One command. Every repo. Every branch. Organized and summarized:

```markdown
# Work Log - yourname

*Generated on February 4, 2026*

# Monday, February 3, 2026

## repo: frontend | Branch: feature/payments

### Updates
- Built the checkout flow with Stripe integration
- Added client-side form validation for card inputs

### Commits
- **16:45** `f8a9b0c` Add Stripe checkout component (+320/-15)
- **14:20** `c3d4e5f` Add card validation helpers (+95/-10)

## repo: backend | Branch: feature/auth

### Updates
- Implemented JWT-based authentication with refresh tokens
- Added role-based access control middleware

### Commits
- **11:30** `a1b2c3d` Add JWT authentication (+250/-30)
- **09:15** `d4e5f6g` Implement RBAC middleware (+180/-20)
```

That's your standup, done. Copy-paste it, email it, or drop it in Slack.

## Commands Reference

### `devlog ingest`

Ingest git history from your repositories.

```bash
devlog ingest                      # Current directory
devlog ingest ~/projects/myapp     # Specific path
devlog ingest --days 90            # Last 90 days (default: 30)
devlog ingest --all                # Full git history
devlog ingest --reselect-branches  # Re-select branches
devlog ingest --all-branches       # Ingest all branches
devlog ingest --fill-summaries     # Generate missing commit summaries
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

### `devlog console`

Interactive terminal UI to browse repositories and worklogs.

```bash
devlog console                     # Launch full-screen TUI
```

Features:
- Browse through all your ingested repositories
- Navigate day-by-day worklogs
- View formatted markdown content in the terminal
- Keyboard shortcuts for quick navigation (arrow keys, j/k, tab)

Requires at least one prior `devlog worklog` run to populate the cache.

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
devlog --profile personal worklog --days 7
```

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

## LLM Providers — Use What You Already Have

DevLog doesn't lock you into an expensive API. Start free and local, upgrade if you want.

| Provider | Privacy | Cost | Best For |
|----------|---------|------|----------|
| **Ollama** | 🟢 Fully local | Free | Default. Your data never leaves your machine. |
| **OpenRouter** | 🟡 Cloud | ~$0.001/query | Cheap access to dozens of models |
| **Anthropic** | 🟡 Cloud | Paid | Best quality summaries |
| **OpenAI** | 🟡 Cloud | Paid | Wide model selection |
| **Bedrock** | 🟡 Cloud | Paid | AWS-integrated workflows |

### Setting Up Ollama (Recommended — Free & Private)

```bash
# 1. Install Ollama (ollama.ai)
# 2. Pull a model
ollama pull llama3.2

# 3. Start the server
ollama serve

# 4. DevLog will detect it automatically
devlog onboard
```

No API keys. No accounts. No usage limits. Just your machine.

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

### Multiple Repos, One Timeline

Ingest every repo you touch — DevLog unifies them:
```bash
devlog ingest ~/projects/frontend
devlog ingest ~/projects/backend
devlog ingest ~/projects/shared-lib
devlog worklog --days 7  # See everything in one report
```

### Work vs Personal — Completely Isolated

Profiles give you separate databases. No bleed between contexts:
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

## Who Is This For?

- **Individual developers** who work across multiple repos and branches and need clean work logs for standups, weekly reports, or performance reviews
- **Freelancers & contractors** who bill by the hour and need to show clients what was delivered
- **Open source maintainers** who want to generate changelogs and activity summaries
- **Anyone tired of `git log --oneline | head -50`** as a standup prep strategy

## Contributing

Contributions are welcome! DevLog is open source and we'd love your help making it better.

**Ways to contribute:**
- 🐛 Report bugs or issues
- 💡 Suggest new features or improvements
- 📝 Improve documentation
- 🔧 Submit pull requests for bug fixes or features
- ⭐ Star the repo if you find it useful

**Getting started:**
1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes and commit (`git commit -m 'feat: add amazing feature'`)
4. Push to your branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

See the [Development](#development) section for build instructions.

## License

MIT License - see [LICENSE](LICENSE) for details.

---

**Built for developers who ship faster than they can remember.** Star the repo if DevLog saves your next standup.
