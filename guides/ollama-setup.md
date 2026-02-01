# Ollama Setup Guide

## Install Ollama

**macOS:**
```bash
brew install ollama
```

**Linux:**
```bash
curl -fsSL https://ollama.com/install.sh | sh
```

**Windows:**
Download from [ollama.com/download](https://ollama.com/download)

## Start Ollama

```bash
ollama serve
```

## Pull a Model

```bash
ollama pull llama3.2
```

Other recommended models:
- `llama3.2` - Fast, good for most tasks (default)
- `llama3.1` - More capable, slower
- `codellama` - Optimized for code
- `mistral` - Good balance of speed/quality

## Verify Setup

```bash
ollama list
```

## Use with DevLog

```bash
devlog ask "What did I work on today?"
```

Or explicitly:
```bash
devlog ask --provider ollama --model llama3.2 "What did I work on today?"
```

## Configuration

Set default model in config:
```bash
devlog onboard
```

Or manually edit `~/.devlog/config.json`:
```json
{
  "default_provider": "ollama",
  "ollama_base_url": "http://localhost:11434",
  "ollama_model": "llama3.2"
}
```

## Troubleshooting

**Connection refused:**
- Make sure `ollama serve` is running
- Check if port 11434 is available

**Model not found:**
- Run `ollama pull <model-name>`
- Check available models with `ollama list`

**Slow responses:**
- Try a smaller model like `llama3.2`
- Ensure you have enough RAM (8GB+ recommended)
