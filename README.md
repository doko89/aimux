# aimux — AI API Gateway

Anthropic ↔ OpenAI protocol translation gateway with multi-provider routing and model aggregation.

## Features

- **Protocol bridge** — Accepts Anthropic Messages API, converts to OpenAI Chat Completions
- **Multi-provider** — Routes to any OpenAI-compatible backend (OpenAI, DeepSeek, Mimo, OpenRouter, etc.)
- **ChatGPT/Codex provider** — Use your ChatGPT Plus/Pro subscription via OAuth device flow
- **Interactive setup** — `aimux setup` TUI to configure all providers, aggregations, and settings
- **Model aggregation** — Virtual models that distribute traffic across providers via weighted, fallback, or round-robin strategies
- **Circuit breaker** — Automatic failover with health checks and cooldown
- **Thinking/Reasoning** — Bidirectional conversion between Anthropic thinking blocks and OpenAI reasoning_content
- **Streaming** — Full SSE streaming support with real-time format conversion

## CLI

```bash
aimux setup               # Interactive configuration TUI (bubbletea)
aimux server              # Start the API gateway server
aimux login chatgpt       # Login with ChatGPT account (OAuth device flow)
aimux login chatgpt -f    # Force re-login
aimux version             # Show version
aimux help                # Show help
```

### Setup Wizard

```bash
aimux setup
```

Interactive TUI to configure everything:

- **Gateway Settings** — host, port, debug mode
- **Providers** — add/edit/remove providers (OpenAI, Anthropic, DeepSeek, Codex, custom, etc.)
- **Model Aggregations** — create virtual models with weighted/fallback/round-robin strategies
- **Routing** — strategy, fallback on error, max retries
- **Circuit Breaker** — failure threshold, cooldown, health check interval
- **Rate Limiting** — enabled, RPM, burst
- **Auth (API Keys)** — manage client API keys
- **Login (ChatGPT)** — trigger OAuth device flow

Generates `.env` and `aggregation.yaml` files.

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
# Option 1: Interactive setup
go build -o aimux .
./aimux setup

# Option 2: Manual config
cp .env.example .env
cp aggregation.example.yaml aggregation.yaml
go build -o aimux .
./aimux server
```

### With ChatGPT/Codex

```bash
./aimux login chatgpt      # Login with your ChatGPT account
./aimux server             # Start server — codex provider auto-detected
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

Use `aimux setup` for interactive configuration, or see `.env.example` and `aggregation.example.yaml` for manual setup.

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
