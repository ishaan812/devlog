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
ollama pull gemma3:4b
```

Other recommended models:
- `gemma3:4b` - Fast, lightweight (default)
- `gemma3` - Fast, good for most tasks
- `codellama` - Optimized for code
- `mistral` - Good balance of speed/quality

## Verify Setup

```bash
ollama list
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
  "ollama_model": "gemma3"
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
- Try a smaller model like `gemma3`
- Ensure you have enough RAM (8GB+ recommended)
