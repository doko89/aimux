# aimux — AI API Gateway

Anthropic ↔ OpenAI protocol translation gateway with multi-provider routing and model aggregation.

## Features

- **Protocol bridge** — Accepts Anthropic Messages API, converts to OpenAI Chat Completions
- **Multi-provider** — Routes to any OpenAI-compatible backend (OpenAI, DeepSeek, Mimo, OpenRouter, etc.)
- **Model aggregation** — Virtual models that distribute traffic across providers via weighted, fallback, or round-robin strategies
- **Circuit breaker** — Automatic failover with health checks and cooldown
- **Thinking/Reasoning** — Bidirectional conversion between Anthropic thinking blocks and OpenAI reasoning_content
- **Streaming** — Full SSE streaming support with real-time format conversion
- **Response caching** — `count_tokens` endpoint with cached response support

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

## Configuration

Providers and routing are configured via environment variables or `config.yaml`. Model aggregations are defined in a separate YAML file (default: `aggregation.yaml`, overridable via `CONFIG_FILE`).

See `.env.example` for all available options and `aggregation.example.yaml` for aggregation setup.
