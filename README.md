# aimux — AI API Gateway

Anthropic ↔ OpenAI protocol translation gateway with multi-provider routing and model aggregation.

## Features

- **Protocol bridge** — Accepts Anthropic Messages API, converts to OpenAI Chat Completions
- **Multi-provider** — Routes to any OpenAI-compatible backend (OpenAI, DeepSeek, Mimo, OpenRouter, etc.)
- **ChatGPT/Codex provider** — Use your ChatGPT Plus/Pro subscription via OAuth device flow
- **Model aggregation** — Virtual models that distribute traffic across providers via weighted, fallback, or round-robin strategies
- **Circuit breaker** — Automatic failover with health checks and cooldown
- **Thinking/Reasoning** — Bidirectional conversion between Anthropic thinking blocks and OpenAI reasoning_content
- **Streaming** — Full SSE streaming support with real-time format conversion
- **CLI subcommands** — `aimux login`, `aimux version`, `aimux help`

## CLI

```bash
aimux                    # Start the API gateway server
aimux login chatgpt      # Login with ChatGPT account (OAuth device flow)
aimux login chatgpt -f   # Force re-login
aimux version            # Show version
aimux help               # Show help
```

### ChatGPT/Codex Login

```bash
aimux login chatgpt
```

Opens a device code flow — you'll see a URL and code to enter in your browser.
After authorization, tokens are saved to `~/.aimux/chatgpt-auth.json` and auto-refreshed.

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/messages` | Anthropic Messages API (converted to OpenAI) |
| POST | `/v1/messages/count_tokens` | Token counting |
| POST | `/v1/chat/completions` | OpenAI Chat Completions API |
| POST | `/v1/responses` | OpenAI Responses API v2 (non-streaming) |
| GET | `/v1/models` | List available models (aggregations only) |
| GET | `/health` | Health check |
| GET | `/health/providers` | Per-provider status |
| GET | `/admin/stats` | Admin statistics |

## Quick Start

```bash
cp .env.example .env    # edit with your API keys
cp aggregation.example.yaml aggregation.yaml  # configure aggregations
go build -o aimux .
./aimux
```

### With ChatGPT/Codex

```bash
./aimux login chatgpt      # Login with your ChatGPT account
./aimux                     # Start server — codex provider auto-detected
```

## Providers

### Built-in Providers

| Provider | Auth | Env Vars |
|----------|------|----------|
| OpenAI | API key | `OPENAI_API_KEY` |
| Anthropic | API key | `ANTHROPIC_API_KEY` |
| DeepSeek | API key | `DEEPSEEK_API_KEY` |
| OpenRouter | API key | `OPENROUTER_API_KEY` |
| Ollama | None | `OLLAMA_BASE_URL` |
| **Codex (ChatGPT)** | **OAuth** | **`aimux login chatgpt`** |
| Generic OpenAI-compatible | API key | `<NAME>_BASE_URL`, `<NAME>_API_KEY` |

### Codex Provider

Uses your ChatGPT Plus/Pro subscription. After `aimux login chatgpt`, the provider is auto-detected and configured.

- Endpoint: `https://chatgpt.com/backend-api/codex/responses`
- Model: `gpt-5.5` (default, configurable via `CODEX_MODEL`)
- Auth: OAuth tokens stored in `~/.aimux/chatgpt-auth.json`
- Token refresh: automatic via refresh token

## Model Aggregations

Virtual models that route across multiple providers. Define in `aggregation.yaml`:

```yaml
model_aggregations:
  - name: codex
    strategy: weighted
    models:
      - provider: codex
        model: gpt-5.5
        weight: 100

  - name: flash
    strategy: weighted
    models:
      - provider: mimo
        model: mimo-v2.5
        weight: 50
      - provider: opencode
        model: deepseek-v4-flash-free
        weight: 25
```

Strategies: `weighted`, `fallback`, `round_robin`

## Configuration

Providers and routing are configured via environment variables or `config.yaml`. Model aggregations are defined in a separate YAML file (default: `aggregation.yaml`, overridable via `CONFIG_FILE`).

See `.env.example` for all available options and `aggregation.example.yaml` for aggregation setup.

### Codex-Specific Env Vars

| Variable | Default | Description |
|----------|---------|-------------|
| `CODEX_ENABLED` | `true` | Enable codex provider (auto-detected from auth file) |
| `CODEX_MODEL` | `gpt-5.5` | Model to use |
| `CODEX_WEIGHT` | `40` | Provider weight for routing |
| `CODEX_PRIORITY` | `1` | Provider priority |
| `CODEX_TIMEOUT` | `120` | Request timeout in seconds |

## Deploy (Linux ARM64)

```bash
GOOS=linux GOARCH=arm64 go build -o aimux-linux-arm64 .
scp aimux-linux-arm64 user@server:/apps/ai-router/aimux
ssh user@server "sudo systemctl restart ai-router"
```

## Architecture

```
Client (Anthropic API)
    ↓ POST /v1/messages
aimux gateway
    ├─ Anthropic → OpenAI format conversion
    ├─ Model aggregation routing
    ├─ Circuit breaker failover
    └─ OpenAI → Anthropic format conversion
    ↓
Providers (OpenAI, Codex, DeepSeek, Mimo, etc.)
```
