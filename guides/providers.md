# LLM Provider Setup

DevLog supports multiple LLM providers. Choose based on your needs:

| Provider | Cost | Privacy | Quality | Setup |
|----------|------|---------|---------|-------|
| Ollama | Free | Local | Good | Easy |
| Anthropic | Paid | Cloud | Excellent | Easy |
| OpenAI | Paid | Cloud | Excellent | Easy |
| Bedrock | Paid | AWS | Excellent | Medium |

## Ollama (Recommended for Privacy)

See [ollama-setup.md](./ollama-setup.md)

## Anthropic Claude

1. Get API key from [console.anthropic.com](https://console.anthropic.com)
2. Run `devlog onboard` and enter your key
3. Or set environment variable:
   ```bash
   export ANTHROPIC_API_KEY=sk-ant-...
   ```

Models: `claude-3-5-sonnet-20241022`, `claude-3-opus-20240229`, `claude-3-haiku-20240307`

## OpenAI

1. Get API key from [platform.openai.com](https://platform.openai.com)
2. Run `devlog onboard` and enter your key
3. Or set environment variable:
   ```bash
   export OPENAI_API_KEY=sk-...
   ```

Models: `gpt-4o`, `gpt-4o-mini`, `gpt-4-turbo`

## AWS Bedrock

1. Enable Bedrock in AWS Console
2. Create IAM user with Bedrock access
3. Run `devlog onboard` and enter credentials
4. Or set environment variables:
   ```bash
   export AWS_ACCESS_KEY_ID=AKIA...
   export AWS_SECRET_ACCESS_KEY=...
   export AWS_REGION=us-east-1
   ```

Models: `anthropic.claude-3-5-sonnet-20241022-v2:0`, `anthropic.claude-3-haiku-20240307-v1:0`

## Switching Providers

```bash
# Use a specific provider
devlog ask --provider anthropic "What did I do today?"

# Set default provider
devlog onboard
```
