Here is the comprehensive documentation:

---

# 📖 AI API Gateway — Dokumentasi Pembangunan

## Arsitektur: Claude Code → AI API (Anthropic ↔ OpenAI) → Multi AI Provider

---

## 📋 Daftar Isi

1. [Overview & Arsitektur](#1-overview--arsitektur)
2. [Alur Request (Request Flow)](#2-alur-request-request-flow)
3. [Komponen Utama](#3-komponen-utama)
4. [Routing Strategies](#4-routing-strategies)
5. [Converter: Anthropic ↔ OpenAI](#5-converter-anthropic--openai)
6. [Multi-Provider Support](#6-multi-provider-support)
7. [Circuit Breaker & Health Check](#7-circuit-breaker--health-check)
8. [Project Structure](#8-project-structure)
9. [API Reference](#9-api-reference)
10. [Konfigurasi & Environment Variables](#10-konfigurasi--environment-variables)
11. [Deployment](#11-deployment)
12. [Testing](#12-testing)

---

## 1. Overview & Arsitektur

### Apa ini?

AI API Gateway adalah **proxy server** yang menerima request dalam format **Anthropic Messages API** (dari Claude Code) dan menerjemahkannya ke format **OpenAI Chat Completions API** untuk dikirim ke berbagai AI provider. Gateway ini mendukung **multiple provider** dengan strategi routing seperti **Round Robin**, **Fallback**, dan **Weighted**.

### Mengapa dibutuhkan?

| Problem | Solusi |
|---------|--------|
| Claude Code hanya bisa bicara format Anthropic | Gateway translate Anthropic ↔ OpenAI |
| Vendor lock-in ke satu provider | Multi-provider support |
| Single point of failure | Automatic failover & circuit breaker |
| Rate limit dari satu provider | Load balancing ke beberapa provider |
| Biaya tinggi jika pakai model premium terus | Weighted routing (prioritaskan model murah) |

### Diagram Arsitektur

![Architecture Diagram](assets/architecture-diagram-showing-the-ai-api-system-flo-1783410235005.svg)

### Ringkasan Alur

```
┌──────────────┐     Anthropic Format      ┌──────────────────────────────┐
│              │    POST /v1/messages       │        AI API Gateway        │
│ Claude Code  │ ─────────────────────────► │                              │
│   (Client)   │                            │  ┌────────────────────────┐  │
│              │ ◄───────────────────────── │  │  Request Converter     │  │
└──────────────┘    SSE Stream Response     │  │  Anthropic → OpenAI    │  │
                                           │  └───────────┬────────────┘  │
                                           │              │               │
                                           │  ┌───────────▼────────────┐  │
                                           │  │    Router Engine       │  │
                                           │  │  ┌──────┬──────┬─────┐│  │
                                           │  │  │Round │Fall- │Wgh- ││  │
                                           │  │  │Robin │back  │ted  ││  │
                                           │  │  └──────┴──────┴─────┘│  │
                                           │  └───────────┬────────────┘  │
                                           │              │               │
                                           │  ┌───────────▼────────────┐  │
                                           │  │  Response Converter    │  │
                                           │  │  OpenAI → Anthropic    │  │
                                           │  └────────────────────────┘  │
                                           └──────────────┬───────────────┘
                                                          │
                              ┌───────────────────────────┼────────────────────────┐
                              │                           │                        │
                              ▼                           ▼                        ▼
                    ┌──────────────┐          ┌──────────────┐          ┌──────────────┐
                    │   OpenAI     │          │  DeepSeek    │          │  OpenRouter  │
                    │   GPT-4o     │          │  DeepSeek-V3 │          │  100+ Models │
                    └──────────────┘          └──────────────┘          └──────────────┘
```

---

## 2. Alur Request (Request Flow)

### 2.1 Detailed Flow Diagram

```
Claude Code
    │
    │ ① POST /v1/messages (Anthropic format)
    │    Headers: x-api-key, anthropic-version
    │    Body: { model, messages, system, tools, stream, ... }
    ▼
┌─────────────────────────────────────────────────────────┐
│                    AI API GATEWAY                        │
│                                                         │
│  ② Auth Middleware                                       │
│     - Validate API key                                  │
│     - Rate limiting per client                          │
│                                                         │
│  ③ Request Converter (Anthropic → OpenAI)               │
│     - Map system prompt → system message                │
│     - Convert messages[] format                         │
│     - Translate tool_use → function calling             │
│     - Map model name (claude-sonnet → gpt-4o)          │
│     - Convert thinking → reasoning_content              │
│     - Strip cache_control fields                        │
│                                                         │
│  ④ Router Engine                                        │
│     - Select provider based on strategy                 │
│     - Check circuit breaker state                       │
│     - Apply weights / priority                          │
│     - Track latency per provider                        │
│                                                         │
│  ⑤ Provider Adapter                                     │
│     - Send to selected OpenAI-compatible endpoint       │
│     - Handle streaming SSE                              │
│                                                         │
│  ⑥ Response Converter (OpenAI → Anthropic)              │
│     - Convert chat completion → messages format         │
│     - Map tool_calls → tool_use blocks                  │
│     - Translate reasoning_content → thinking blocks     │
│     - Format SSE events for Anthropic stream            │
│     - Map finish_reason → stop_reason                   │
│                                                         │
│  ⑦ Fallback Handler (if error)                          │
│     - On 429/5xx/timeout → try next provider            │
│     - Update circuit breaker state                      │
│     - Log failover event                                │
└─────────────────────────────────────────────────────────┘
    │
    │ ⑧ Response to Claude Code
    │    Format: Anthropic Messages API
    ▼
Claude Code (receives response)
```

### 2.2 Streaming Event Flow

Saat streaming (`stream: true`), events diterjemahkan secara real-time:

```
OpenAI SSE Events                  →    Anthropic SSE Events
─────────────────                       ──────────────────────
                                        event: message_start
                                        data: {type, message}
                                        
                                        event: content_block_start
                                        data: {type:"text", ...}

data: {"choices":[{"delta":             event: content_block_delta
  {"content":"Hello"}}]}                data: {type:"text_delta", ...}

data: {"choices":[{"delta":             event: content_block_stop
  {}, "finish_reason":"stop"}]}

                                        event: message_delta
                                        data: {stop_reason, usage}
                                        
                                        event: message_stop
```

---

## 3. Komponen Utama

### 3.1 Request Converter (`anthropic_to_openai.py`)

Bertanggung jawab mengubah format request Anthropic ke OpenAI.

#### Mapping Tabel

| Anthropic Field | OpenAI Field | Catatan |
|----------------|-------------|---------|
| `messages[].role: "user"` | `messages[].role: "user"` | Sama |
| `messages[].role: "assistant"` | `messages[].role: "assistant"` | Sama |
| `system` (string/array) | `messages[].role: "system"` | Dipindah ke messages array |
| `messages[].content[].type: "tool_result"` | `messages[].role: "tool"` | Format berubah |
| `messages[].content[].type: "tool_use"` | `tool_calls[]` | Format berubah |
| `tools[].input_schema` | `tools[].function.parameters` | Schema mapping |
| `tool_choice: {type:"auto"}` | `tool_choice: "auto"` | Simplified |
| `tool_choice: {type:"any"}` | `tool_choice: "required"` | Mapped |
| `tool_choice: {type:"tool", name:"x"}` | `tool_choice: {type:"function", function:{name:"x"}}` | Nested |
| `max_tokens` | `max_tokens` / `max_completion_tokens` | Adaptive per model |
| `temperature`, `top_p` | `temperature`, `top_p` | Sama |
| `top_k` | Tidak ada | Dihapus/diabaikan |
| `metadata` | Tidak ada | Dihapus |
| `stream: true` | `stream: true` | Sama |
| `thinking` block | `reasoning_content` | Perlu konversi |
| `cache_control` | Tidak ada | Strip sebelum kirim |

#### Model Mapping

| Claude Model | → | Target Model | Config Variable |
|-------------|---|-------------|-----------------|
| `claude-haiku-4-5` | → | `gpt-4o-mini` | `SMALL_MODEL` |
| `claude-sonnet-4-6` | → | `gpt-4o` | `MIDDLE_MODEL` |
| `claude-opus-4` | → | `gpt-4o` | `BIG_MODEL` |
| Model dengan prefix provider | → | Pass through | Tidak di-remap |

#### Code Example: Request Converter

```python
# src/converters/anthropic_to_openai.py

from typing import Any
from ..models.anthropic_schemas import AnthropicMessageRequest
from ..models.openai_schemas import OpenAIChatRequest

class AnthropicToOpenAIConverter:
    """Convert Anthropic Messages API format to OpenAI Chat Completions format."""
    
    def __init__(self, model_mapping: dict[str, str]):
        self.model_mapping = model_mapping
    
    def convert(self, request: AnthropicMessageRequest) -> OpenAIChatRequest:
        messages = []
        
        # 1. Convert system prompt
        if request.system:
            system_content = self._extract_system_content(request.system)
            messages.append({
                "role": "system",
                "content": system_content
            })
        
        # 2. Convert messages
        for msg in request.messages:
            converted = self._convert_message(msg)
            if isinstance(converted, list):
                messages.extend(converted)
            else:
                messages.append(converted)
        
        # 3. Map model name
        model = self._map_model(request.model)
        
        # 4. Build tools
        tools = self._convert_tools(request.tools) if request.tools else None
        
        # 5. Convert tool_choice
        tool_choice = self._convert_tool_choice(request.tool_choice) if request.tool_choice else None
        
        return OpenAIChatRequest(
            model=model,
            messages=messages,
            max_tokens=request.max_tokens,
            temperature=request.temperature,
            top_p=request.top_p,
            stream=request.stream or False,
            tools=tools,
            tool_choice=tool_choice,
            stop=request.stop_sequences,
        )
    
    def _extract_system_content(self, system) -> str:
        """Extract system prompt from string or content blocks."""
        if isinstance(system, str):
            return system
        if isinstance(system, list):
            parts = []
            for block in system:
                if isinstance(block, dict) and block.get("type") == "text":
                    parts.append(block["text"])
                elif isinstance(block, str):
                    parts.append(block)
            return "\n".join(parts)
        return str(system)
    
    def _convert_message(self, msg: dict) -> list[dict]:
        """Convert a single Anthropic message to OpenAI format."""
        role = msg.get("role")
        content = msg.get("content")
        
        if role == "assistant":
            return self._convert_assistant_message(content)
        elif role == "user":
            return self._convert_user_message(content)
        
        return [{"role": role, "content": content}]
    
    def _convert_assistant_message(self, content) -> list[dict]:
        """Convert assistant message, extracting tool_use blocks."""
        messages = []
        text_parts = []
        tool_calls = []
        
        if isinstance(content, str):
            return [{"role": "assistant", "content": content}]
        
        if isinstance(content, list):
            for block in content:
                block_type = block.get("type")
                
                if block_type == "text":
                    text_parts.append(block["text"])
                
                elif block_type == "thinking":
                    # Convert thinking to reasoning (for compatible models)
                    # Will be handled specially based on model
                    pass
                
                elif block_type == "tool_use":
                    tool_calls.append({
                        "id": block["id"],
                        "type": "function",
                        "function": {
                            "name": block["name"],
                            "arguments": self._serialize_json(block["input"])
                        }
                    })
        
        assistant_msg = {"role": "assistant"}
        if text_parts:
            assistant_msg["content"] = "\n".join(text_parts)
        else:
            assistant_msg["content"] = None
        
        if tool_calls:
            assistant_msg["tool_calls"] = tool_calls
        
        messages.append(assistant_msg)
        return messages
    
    def _convert_user_message(self, content) -> list[dict]:
        """Convert user message, handling tool_result blocks."""
        messages = []
        
        if isinstance(content, str):
            return [{"role": "user", "content": content}]
        
        if isinstance(content, list):
            tool_results = []
            text_parts = []
            image_parts = []
            
            for block in content:
                block_type = block.get("type")
                
                if block_type == "text":
                    text_parts.append(block["text"])
                
                elif block_type == "image":
                    image_parts.append(self._convert_image_block(block))
                
                elif block_type == "tool_result":
                    tool_results.append(block)
            
            # If there are tool results, emit them as "tool" role messages
            for result in tool_results:
                tool_msg = {
                    "role": "tool",
                    "tool_call_id": result["tool_use_id"],
                    "content": self._extract_tool_result_content(result)
                }
                messages.append(tool_msg)
            
            # If there's remaining text/image content
            if text_parts or image_parts:
                user_content = []
                for text in text_parts:
                    user_content.append({"type": "text", "text": text})
                for img in image_parts:
                    user_content.append(img)
                
                if len(user_content) == 1 and user_content[0].get("type") == "text":
                    messages.append({"role": "user", "content": user_content[0]["text"]})
                else:
                    messages.append({"role": "user", "content": user_content})
        
        return messages
    
    def _convert_tools(self, tools: list) -> list:
        """Convert Anthropic tools to OpenAI function format."""
        openai_tools = []
        for tool in tools:
            openai_tools.append({
                "type": "function",
                "function": {
                    "name": tool["name"],
                    "description": tool.get("description", ""),
                    "parameters": tool.get("input_schema", {})
                }
            })
        return openai_tools
    
    def _convert_tool_choice(self, tool_choice) -> Any:
        """Convert Anthropic tool_choice to OpenAI format."""
        if not tool_choice:
            return None
        
        choice_type = tool_choice.get("type", "auto")
        
        if choice_type == "auto":
            return "auto"
        elif choice_type == "any":
            return "required"
        elif choice_type == "none":
            return "none"
        elif choice_type == "tool":
            return {
                "type": "function",
                "function": {"name": tool_choice["name"]}
            }
        
        return "auto"
    
    def _map_model(self, model: str) -> str:
        """Map Claude model name to target provider model."""
        # Check exact match first
        if model in self.model_mapping:
            return self.model_mapping[model]
        
        # Check prefix match
        for prefix, target in self.model_mapping.items():
            if model.startswith(prefix):
                return target
        
        # Passthrough for provider-prefixed models
        if "/" in model:
            return model
        
        # Default: return as-is or mapped
        return self.model_mapping.get("default", model)
    
    def _convert_image_block(self, block: dict) -> dict:
        """Convert Anthropic image block to OpenAI image_url format."""
        source = block.get("source", {})
        
        if source.get("type") == "base64":
            media_type = source.get("media_type", "image/png")
            data = source.get("data", "")
            return {
                "type": "image_url",
                "image_url": {
                    "url": f"data:{media_type};base64,{data}"
                }
            }
        elif source.get("type") == "url":
            return {
                "type": "image_url",
                "image_url": {"url": source["url"]}
            }
        
        return {"type": "text", "text": "[image]"}
    
    def _extract_tool_result_content(self, result: dict) -> str:
        """Extract text content from a tool_result block."""
        content = result.get("content", "")
        if isinstance(content, str):
            return content
        if isinstance(content, list):
            parts = []
            for block in content:
                if isinstance(block, dict) and block.get("type") == "text":
                    parts.append(block["text"])
            return "\n".join(parts)
        return str(content)
    
    @staticmethod
    def _serialize_json(obj: Any) -> str:
        """Serialize object to JSON string."""
        import json
        return json.dumps(obj, ensure_ascii=False)
```

### 3.2 Response Converter (`openai_to_anthropic.py`)

```python
# src/converters/openai_to_anthropic.py

import json
import uuid
from typing import AsyncIterator

class OpenAIToAnthropicConverter:
    """Convert OpenAI Chat Completions response to Anthropic Messages format."""
    
    def convert_non_stream(self, openai_response: dict, model: str) -> dict:
        """Convert a non-streaming OpenAI response to Anthropic format."""
        choice = openai_response["choices"][0]
        message = choice["message"]
        usage = openai_response.get("usage", {})
        
        # Build content blocks
        content = []
        
        # Text content
        if message.get("content"):
            content.append({
                "type": "text",
                "text": message["content"]
            })
        
        # Tool calls
        if message.get("tool_calls"):
            for tc in message["tool_calls"]:
                content.append({
                    "type": "tool_use",
                    "id": tc["id"],
                    "name": tc["function"]["name"],
                    "input": json.loads(tc["function"]["arguments"])
                })
        
        # Map finish_reason → stop_reason
        stop_reason = self._map_stop_reason(choice.get("finish_reason", "stop"))
        
        return {
            "id": f"msg_{uuid.uuid4().hex[:24]}",
            "type": "message",
            "role": "assistant",
            "content": content,
            "model": model,
            "stop_reason": stop_reason,
            "stop_sequence": None,
            "usage": {
                "input_tokens": usage.get("prompt_tokens", 0),
                "output_tokens": usage.get("completion_tokens", 0),
                "cache_creation_input_tokens": 0,
                "cache_read_input_tokens": 0
            }
        }
    
    async def convert_stream(
        self, 
        openai_stream: AsyncIterator[dict], 
        model: str,
        request_id: str = None
    ) -> AsyncIterator[str]:
        """Convert streaming OpenAI SSE events to Anthropic SSE format."""
        
        request_id = request_id or f"msg_{uuid.uuid4().hex[:24]}"
        content_block_index = 0
        total_input_tokens = 0
        total_output_tokens = 0
        tool_calls_buffer = {}  # id → {index, name, arguments}
        
        # ① Send message_start
        yield self._sse_event("message_start", {
            "type": "message_start",
            "message": {
                "id": request_id,
                "type": "message",
                "role": "assistant",
                "content": [],
                "model": model,
                "stop_reason": None,
                "stop_sequence": None,
                "usage": {"input_tokens": 0, "output_tokens": 0}
            }
        })
        
        text_started = False
        
        async for chunk in openai_stream:
            if not chunk.get("choices"):
                # Might contain usage info
                if chunk.get("usage"):
                    total_input_tokens = chunk["usage"].get("prompt_tokens", 0)
                    total_output_tokens = chunk["usage"].get("completion_tokens", 0)
                continue
            
            choice = chunk["choices"][0]
            delta = choice.get("delta", {})
            finish_reason = choice.get("finish_reason")
            
            # Handle text content
            if delta.get("content"):
                if not text_started:
                    # Start text content block
                    yield self._sse_event("content_block_start", {
                        "type": "content_block_start",
                        "index": content_block_index,
                        "content_block": {"type": "text", "text": ""}
                    })
                    text_started = True
                
                yield self._sse_event("content_block_delta", {
                    "type": "content_block_delta",
                    "index": content_block_index,
                    "delta": {
                        "type": "text_delta",
                        "text": delta["content"]
                    }
                })
            
            # Handle tool calls (streamed incrementally)
            if delta.get("tool_calls"):
                for tc_delta in delta["tool_calls"]:
                    tc_index = tc_delta.get("index", 0)
                    
                    if tc_delta.get("id"):
                        # New tool call starting
                        tool_calls_buffer[tc_index] = {
                            "id": tc_delta["id"],
                            "name": "",
                            "arguments": ""
                        }
                        
                        # Close text block if open
                        if text_started:
                            yield self._sse_event("content_block_stop", {
                                "type": "content_block_stop",
                                "index": content_block_index
                            })
                            content_block_index += 1
                            text_started = False
                        
                        # Start tool_use block
                        yield self._sse_event("content_block_start", {
                            "type": "content_block_start",
                            "index": content_block_index,
                            "content_block": {
                                "type": "tool_use",
                                "id": tc_delta["id"],
                                "name": tc_delta.get("function", {}).get("name", "")
                            }
                        })
                        
                        if tc_delta.get("function", {}).get("name"):
                            tool_calls_buffer[tc_index]["name"] = tc_delta["function"]["name"]
                    
                    # Accumulate arguments
                    if tc_delta.get("function", {}).get("arguments"):
                        tool_calls_buffer[tc_index]["arguments"] += tc_delta["function"]["arguments"]
                        
                        yield self._sse_event("content_block_delta", {
                            "type": "content_block_delta",
                            "index": content_block_index,
                            "delta": {
                                "type": "input_json_delta",
                                "partial_json": tc_delta["function"]["arguments"]
                            }
                        })
            
            # Handle reasoning/thinking content
            if delta.get("reasoning_content"):
                # Emit as thinking block (if supported)
                pass  # Handle based on model support
            
            # Handle finish
            if finish_reason:
                # Close open content block
                if text_started:
                    yield self._sse_event("content_block_stop", {
                        "type": "content_block_stop",
                        "index": content_block_index
                    })
                    content_block_index += 1
                
                # Close any open tool_use blocks
                for idx in tool_calls_buffer:
                    yield self._sse_event("content_block_stop", {
                        "type": "content_block_stop",
                        "index": content_block_index
                    })
                    content_block_index += 1
        
        # ② Send message_delta with stop_reason and usage
        stop_reason = self._map_stop_reason(
            finish_reason if finish_reason else "stop"
        )
        
        yield self._sse_event("message_delta", {
            "type": "message_delta",
            "delta": {
                "stop_reason": stop_reason,
                "stop_sequence": None
            },
            "usage": {
                "output_tokens": total_output_tokens
            }
        })
        
        # ③ Send message_stop
        yield self._sse_event("message_stop", {
            "type": "message_stop"
        })
    
    def _map_stop_reason(self, openai_reason: str) -> str:
        """Map OpenAI finish_reason to Anthropic stop_reason."""
        mapping = {
            "stop": "end_turn",
            "length": "max_tokens",
            "tool_calls": "tool_use",
            "content_filter": "end_turn",
            "function_call": "tool_use",
        }
        return mapping.get(openai_reason, "end_turn")
    
    def _sse_event(self, event_type: str, data: dict) -> str:
        """Format a Server-Sent Event string."""
        return f"event: {event_type}\ndata: {json.dumps(data, ensure_ascii=False)}\n\n"
```

### 3.3 Router Engine

```python
# src/router/engine.py

import asyncio
import time
from enum import Enum
from dataclasses import dataclass, field
from typing import Optional

class RoutingStrategy(str, Enum):
    ROUND_ROBIN = "round_robin"
    FALLBACK = "fallback"
    WEIGHTED = "weighted"
    LEAST_LATENCY = "least_latency"

class CircuitState(str, Enum):
    CLOSED = "closed"       # Healthy - normal traffic
    OPEN = "open"           # Unhealthy - block traffic
    HALF_OPEN = "half_open" # Testing - allow probe request

@dataclass
class ProviderInstance:
    """Represents a single AI provider configuration."""
    name: str
    base_url: str
    api_key: str
    model: str
    weight: int = 1
    priority: int = 0           # Lower = higher priority
    max_rpm: int = 0            # 0 = unlimited
    
    # Health tracking
    circuit_state: CircuitState = CircuitState.CLOSED
    consecutive_failures: int = 0
    last_failure_time: float = 0
    total_requests: int = 0
    total_failures: int = 0
    avg_latency_ms: float = 0
    last_used_time: float = 0
    
    # Cooldown
    cooldown_seconds: float = 60
    failure_threshold: int = 5

class RouterEngine:
    """Multi-provider routing engine with strategy support."""
    
    def __init__(
        self, 
        providers: list[ProviderInstance],
        strategy: RoutingStrategy = RoutingStrategy.ROUND_ROBIN,
        fallback_providers: Optional[list[str]] = None,
        max_retries: int = 2
    ):
        self.providers = {p.name: p for p in providers}
        self.strategy = strategy
        self.fallback_providers = fallback_providers or []
        self.max_retries = max_retries
        self._rr_index = 0
        self._lock = asyncio.Lock()
    
    async def select_provider(
        self, 
        exclude: Optional[set[str]] = None
    ) -> ProviderInstance:
        """Select a provider based on the configured strategy."""
        exclude = exclude or set()
        
        available = [
            p for p in self.providers.values()
            if p.name not in exclude and self._is_available(p)
        ]
        
        if not available:
            # Reset circuit breakers and try again
            self._reset_half_open_providers()
            available = [
                p for p in self.providers.values()
                if p.name not in exclude
            ]
        
        if not available:
            raise NoProviderAvailableError("All providers are unavailable")
        
        if self.strategy == RoutingStrategy.ROUND_ROBIN:
            return await self._round_robin(available)
        elif self.strategy == RoutingStrategy.FALLBACK:
            return await self._fallback(available)
        elif self.strategy == RoutingStrategy.WEIGHTED:
            return await self._weighted(available)
        elif self.strategy == RoutingStrategy.LEAST_LATENCY:
            return await self._least_latency(available)
        
        return available[0]
    
    async def execute_with_fallback(
        self,
        request_fn,  # async callable that takes a ProviderInstance
        request_data: dict
    ) -> dict:
        """Execute a request with automatic fallback on failure."""
        attempted = set()
        last_error = None
        
        for attempt in range(self.max_retries + 1):
            try:
                provider = await self.select_provider(exclude=attempted)
                
                start_time = time.monotonic()
                result = await request_fn(provider, request_data)
                latency = (time.monotonic() - start_time) * 1000
                
                # Update success metrics
                self._record_success(provider, latency)
                return result
                
            except RateLimitError as e:
                self._record_failure(provider, "rate_limit")
                attempted.add(provider.name)
                last_error = e
                
            except ServerError as e:
                self._record_failure(provider, "server_error")
                attempted.add(provider.name)
                last_error = e
                
            except TimeoutError as e:
                self._record_failure(provider, "timeout")
                attempted.add(provider.name)
                last_error = e
        
        raise FallbackExhaustedError(
            f"All providers failed after {self.max_retries + 1} attempts"
        ) from last_error
    
    # ---- Strategy Implementations ----
    
    async def _round_robin(self, available: list[ProviderInstance]) -> ProviderInstance:
        """Distribute requests evenly across providers."""
        async with self._lock:
            provider = available[self._rr_index % len(available)]
            self._rr_index += 1
            return provider
    
    async def _fallback(self, available: list[ProviderInstance]) -> ProviderInstance:
        """Select provider by priority (lowest priority number first)."""
        return min(available, key=lambda p: p.priority)
    
    async def _weighted(self, available: list[ProviderInstance]) -> ProviderInstance:
        """Select provider based on weighted random distribution."""
        import random
        total_weight = sum(p.weight for p in available)
        if total_weight == 0:
            return available[0]
        
        r = random.uniform(0, total_weight)
        cumulative = 0
        for provider in available:
            cumulative += provider.weight
            if r <= cumulative:
                return provider
        
        return available[-1]
    
    async def _least_latency(self, available: list[ProviderInstance]) -> ProviderInstance:
        """Select the provider with lowest average latency."""
        return min(available, key=lambda p: p.avg_latency_ms or float('inf'))
    
    # ---- Circuit Breaker ----
    
    def _is_available(self, provider: ProviderInstance) -> bool:
        """Check if provider is available (circuit breaker check)."""
        if provider.circuit_state == CircuitState.CLOSED:
            return True
        
        if provider.circuit_state == CircuitState.OPEN:
            # Check if cooldown has passed
            elapsed = time.time() - provider.last_failure_time
            if elapsed > provider.cooldown_seconds:
                provider.circuit_state = CircuitState.HALF_OPEN
                return True
            return False
        
        if provider.circuit_state == CircuitState.HALF_OPEN:
            return True  # Allow probe request
        
        return False
    
    def _record_success(self, provider: ProviderInstance, latency_ms: float):
        """Record successful request and update metrics."""
        provider.total_requests += 1
        provider.consecutive_failures = 0
        provider.last_used_time = time.time()
        
        # Exponential moving average for latency
        alpha = 0.3
        if provider.avg_latency_ms == 0:
            provider.avg_latency_ms = latency_ms
        else:
            provider.avg_latency_ms = (
                alpha * latency_ms + (1 - alpha) * provider.avg_latency_ms
            )
        
        # Reset circuit breaker on success
        if provider.circuit_state == CircuitState.HALF_OPEN:
            provider.circuit_state = CircuitState.CLOSED
    
    def _record_failure(self, provider: ProviderInstance, error_type: str):
        """Record failure and potentially trip circuit breaker."""
        provider.total_requests += 1
        provider.total_failures += 1
        provider.consecutive_failures += 1
        provider.last_failure_time = time.time()
        
        if provider.consecutive_failures >= provider.failure_threshold:
            provider.circuit_state = CircuitState.OPEN
    
    def _reset_half_open_providers(self):
        """Reset open circuits to half-open for retry."""
        for provider in self.providers.values():
            if provider.circuit_state == CircuitState.OPEN:
                elapsed = time.time() - provider.last_failure_time
                if elapsed > provider.cooldown_seconds:
                    provider.circuit_state = CircuitState.HALF_OPEN


# ---- Custom Exceptions ----

class NoProviderAvailableError(Exception):
    pass

class RateLimitError(Exception):
    pass

class ServerError(Exception):
    pass

class FallbackExhaustedError(Exception):
    pass
```

### Routing Strategies Diagram

![Routing Strategies](assets/diagram-showing-the-routing-strategies-round-robin-1783410322217.svg)

---

## 4. Routing Strategies

### 4.1 Round Robin

Distribusi request merata ke semua provider secara bergantian.

```
Request 1 → Provider A
Request 2 → Provider B
Request 3 → Provider C
Request 4 → Provider A  (kembali ke awal)
```

**Kapan digunakan:**
- Semua provider memiliki kapasitas dan kemampuan yang mirip
- Ingin distribusi beban yang adil
- Rate limit per provider terbatas

**Konfigurasi:**
```yaml
routing:
  strategy: round_robin
  providers:
    - name: openai
      base_url: https://api.openai.com/v1
      model: gpt-4o
    - name: deepseek
      base_url: https://api.deepseek.com/v1
      model: deepseek-chat
```

---

### 4.2 Fallback (Priority-Based)

Provider dipilih berdasarkan prioritas. Jika provider utama gagal, otomatis pindah ke provider berikutnya.

```
Normal:     Request → Provider A (priority 1) ✓
A down:     Request → Provider A ✗ → Provider B (priority 2) ✓
A&B down:   Request → Provider A ✗ → Provider B ✗ → Provider C (priority 3) ✓
```

**Kapan digunakan:**
- Ada provider utama yang di-preferensikan
- Butuh high availability
- Ada perbedaan kualitas antar provider

**Konfigurasi:**
```yaml
routing:
  strategy: fallback
  max_retries: 2
  providers:
    - name: openai-primary
      base_url: https://api.openai.com/v1
      model: gpt-4o
      priority: 1          # Dipilih pertama
    - name: anthropic-fallback
      base_url: https://api.anthropic.com/v1
      model: claude-sonnet-4-6
      priority: 2          # Fallback jika OpenAI gagal
    - name: deepseek-cheap
      base_url: https://api.deepseek.com/v1
      model: deepseek-chat
      priority: 3          # Last resort
```

---

### 4.3 Weighted

Distribusi request berdasarkan bobot. Provider dengan bobot lebih tinggi menerima lebih banyak traffic.

```
Provider A (weight=70) → menerima ~70% traffic
Provider B (weight=20) → menerima ~20% traffic
Provider C (weight=10) → menerima ~10% traffic
```

**Kapan digunakan:**
- Cost optimization (banyak traffic ke model murah)
- A/B testing antar provider
- Migrasi bertahap antar provider

**Konfigurasi:**
```yaml
routing:
  strategy: weighted
  providers:
    - name: deepseek-cheap
      model: deepseek-chat
      weight: 70           # 70% traffic (murah)
    - name: openai-gpt4o
      model: gpt-4o
      weight: 20           # 20% traffic (mahal, kualitas tinggi)
    - name: anthropic-sonnet
      model: claude-sonnet-4-6
      weight: 10           # 10% traffic (testing)
```

---

### 4.4 Least Latency

Request dikirim ke provider dengan latency terendah.

**Kapan digunakan:**
- Aplikasi real-time yang sensitif terhadap latency
- Provider berada di region yang berbeda

---

### 4.5 Combined Strategies

Strategi bisa dikombinasikan:

```yaml
routing:
  strategy: weighted          # Primary strategy
  fallback_on_error: true     # Enable fallback
  max_retries: 2
  
  providers:
    - name: openai-primary
      model: gpt-4o
      weight: 50
      priority: 1             # Dipilih duluan dalam fallback
      failure_threshold: 5
      cooldown_seconds: 60
    
    - name: anthropic-secondary
      model: claude-sonnet-4-6
      weight: 30
      priority: 2
    
    - name: deepseek-tertiary
      model: deepseek-chat
      weight: 20
      priority: 3             # Last resort
```

---

## 5. Converter: Anthropic ↔ OpenAI

### 5.1 Tabel Konversi Lengkap

#### Messages

| Anthropic | OpenAI | Arah |
|-----------|--------|------|
| `system` (top-level field) | `messages[0].role: "system"` | Request |
| `messages[].role: "user"` | `messages[].role: "user"` | Request |
| `messages[].role: "assistant"` | `messages[].role: "assistant"` | Request |
| `messages[].content[].type: "tool_result"` | `messages[].role: "tool"` | Request |
| `messages[].content[].type: "tool_use"` | `tool_calls[]` di message assistant | Request |

#### Tools

| Anthropic | OpenAI | Arah |
|-----------|--------|------|
| `tools[].name` | `tools[].function.name` | Request |
| `tools[].description` | `tools[].function.description` | Request |
| `tools[].input_schema` | `tools[].function.parameters` | Request |
| `tool_use` (response) | `tool_calls[]` (response) | Response |
| `input_json_delta` (stream) | `function.arguments` (stream delta) | Response |

#### Tool Choice

| Anthropic | OpenAI | Catatan |
|-----------|--------|---------|
| `{type: "auto"}` | `"auto"` | Default |
| `{type: "any"}` | `"required"` | Force tool use |
| `{type: "none"}` | `"none"` | Disable tools |
| `{type: "tool", name: "x"}` | `{type:"function", function:{name:"x"}}` | Force specific tool |
| `{type: "auto", disable_parallel_tool_use: true}` | Tidak ada | Drop field |

#### Stop Reason

| Anthropic | OpenAI | Arah |
|-----------|--------|------|
| `end_turn` | `stop` | Response |
| `max_tokens` | `length` | Response |
| `tool_use` | `tool_calls` | Response |
| `stop_sequence` | `stop` (with content) | Response |

#### Thinking / Reasoning

| Anthropic | OpenAI | Catatan |
|-----------|--------|---------|
| `thinking` content block | `reasoning_content` field | Model-dependent |
| `thinking.type: "enabled"` | `reasoning_effort: "high"` | Parameter mapping |
| `signature_delta` | Dropped | Anthropic-specific |

### 5.2 Edge Cases & Sanitization

```python
# Fields yang perlu di-strip sebelum kirim ke provider non-Anthropic
STRIP_FIELDS = [
    "cache_control",        # Anthropic prompt caching
    "thinking",             # Perlu convert dulu
    "signature",            # Anthropic-specific
    "service_tier",         # Anthropic billing
    "metadata",             # Anthropic-specific
]

# Gemini schema cleaning (28+ fields)
GEMINI_STRIP_FIELDS = [
    "$ref", "$defs", "definitions",  # Unsupported
    "additionalProperties",
    "allOf", "anyOf", "oneOf",      # Complex schemas
    "patternProperties",
    "minProperties", "maxProperties",
]
```

---

## 6. Multi-Provider Support

### 6.1 Supported Providers

| Provider | Base URL | Auth Method | Contoh Model |
|----------|----------|-------------|--------------|
| **OpenAI** | `https://api.openai.com/v1` | Bearer token | `gpt-4o`, `o3-mini`, `o4-mini` |
| **Anthropic** | `https://api.anthropic.com/v1` | `x-api-key` header | `claude-sonnet-4-6` |
| **DeepSeek** | `https://api.deepseek.com/v1` | Bearer token | `deepseek-chat`, `deepseek-reasoner` |
| **OpenRouter** | `https://openrouter.ai/api/v1` | Bearer token | 100+ models via prefix |
| **Azure OpenAI** | Custom endpoint | API key | `gpt-4o`, `gpt-4` |
| **Groq** | `https://api.groq.com/openai/v1` | Bearer token | `llama-3.3-70b` |
| **Together AI** | `https://api.together.xyz/v1` | Bearer token | `meta-llama/Llama-3-70b` |
| **Ollama** | `http://localhost:11434/v1` | None | Any local model |
| **vLLM** | `http://localhost:8000/v1` | None | Any self-hosted model |
| **LM Studio** | `http://localhost:1234/v1` | None | Any loaded model |
| **AWS Bedrock** | Via proxy | AWS SigV4 | Claude, Llama, etc. |
| **Google Vertex** | Via proxy | OAuth | Gemini models |

### 6.2 Provider Adapter Pattern

```python
# src/providers/base.py
from abc import ABC, abstractmethod
from typing import AsyncIterator

class BaseProvider(ABC):
    """Base class for all AI providers."""
    
    @abstractmethod
    async def chat_completion(
        self, 
        request: dict, 
        stream: bool = False
    ) -> dict | AsyncIterator[dict]:
        """Send chat completion request to provider."""
        pass
    
    @abstractmethod
    async def health_check(self) -> bool:
        """Check if provider is healthy."""
        pass
    
    @abstractmethod
    def get_supported_models(self) -> list[str]:
        """Return list of supported model IDs."""
        pass


# src/providers/openai.py
import httpx
from .base import BaseProvider

class OpenAIProvider(BaseProvider):
    """OpenAI-compatible provider adapter."""
    
    def __init__(self, base_url: str, api_key: str, timeout: float = 120):
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.timeout = timeout
        self.client = httpx.AsyncClient(
            base_url=self.base_url,
            headers={
                "Authorization": f"Bearer {api_key}",
                "Content-Type": "application/json"
            },
            timeout=httpx.Timeout(timeout)
        )
    
    async def chat_completion(self, request: dict, stream: bool = False):
        if stream:
            return self._stream_completion(request)
        
        response = await self.client.post(
            "/chat/completions",
            json=request
        )
        response.raise_for_status()
        return response.json()
    
    async def _stream_completion(self, request: dict) -> AsyncIterator[dict]:
        request["stream"] = True
        
        async with self.client.stream(
            "POST", "/chat/completions", json=request
        ) as response:
            response.raise_for_status()
            async for line in response.aiter_lines():
                if line.startswith("data: "):
                    data = line[6:]
                    if data.strip() == "[DONE]":
                        break
                    import json
                    yield json.loads(data)
    
    async def health_check(self) -> bool:
        try:
            response = await self.client.get("/models", timeout=5)
            return response.status_code == 200
        except Exception:
            return False
    
    def get_supported_models(self) -> list[str]:
        return ["gpt-4o", "gpt-4o-mini", "o3-mini", "o4-mini"]
```

### 6.3 Adding a New Provider

1. Buat file baru di `src/providers/`:

```python
# src/providers/deepseek.py
from .openai import OpenAIProvider  # DeepSeek uses OpenAI-compatible API

class DeepSeekProvider(OpenAIProvider):
    """DeepSeek provider - uses OpenAI-compatible API."""
    
    def __init__(self, api_key: str):
        super().__init__(
            base_url="https://api.deepseek.com/v1",
            api_key=api_key
        )
    
    def get_supported_models(self) -> list[str]:
        return ["deepseek-chat", "deepseek-reasoner"]
```

2. Register di config:

```yaml
providers:
  deepseek:
    class: providers.deepseek.DeepSeekProvider
    api_key: ${DEEPSEEK_API_KEY}
    model: deepseek-chat
    weight: 30
    priority: 2
```

---

## 7. Circuit Breaker & Health Check

### 7.1 Circuit Breaker State Machine

```
         errors ≥ threshold
  CLOSED ──────────────────► OPEN
    ▲                          │
    │                          │ cooldown expires
    │    probe success         ▼
    └──────────────────── HALF_OPEN ◄──── probe request allowed
         probe fails               │
              └────────────────────┘
                    (back to OPEN)
```

### 7.2 Health Check Implementation

```python
# src/health/monitor.py

import asyncio
import time
from typing import Callable

class HealthMonitor:
    """Monitors provider health and manages circuit breakers."""
    
    def __init__(
        self,
        providers: dict,
        check_interval: float = 30,  # seconds
        failure_threshold: int = 5,
        recovery_timeout: float = 60,  # seconds
        on_state_change: Callable = None
    ):
        self.providers = providers
        self.check_interval = check_interval
        self.failure_threshold = failure_threshold
        self.recovery_timeout = recovery_timeout
        self.on_state_change = on_state_change
        self._running = False
        self._task = None
    
    async def start(self):
        """Start background health check loop."""
        self._running = True
        self._task = asyncio.create_task(self._check_loop())
    
    async def stop(self):
        """Stop health check loop."""
        self._running = False
        if self._task:
            self._task.cancel()
    
    async def _check_loop(self):
        """Periodically check all provider health."""
        while self._running:
            for name, provider in self.providers.items():
                try:
                    is_healthy = await asyncio.wait_for(
                        provider.health_check(),
                        timeout=10
                    )
                    
                    if is_healthy:
                        self._on_healthy(name, provider)
                    else:
                        self._on_unhealthy(name, provider)
                        
                except asyncio.TimeoutError:
                    self._on_unhealthy(name, provider)
                except Exception:
                    self._on_unhealthy(name, provider)
            
            await asyncio.sleep(self.check_interval)
    
    def _on_healthy(self, name: str, provider):
        """Handle healthy provider."""
        old_state = provider.circuit_state
        provider.consecutive_failures = 0
        
        if old_state == CircuitState.HALF_OPEN:
            provider.circuit_state = CircuitState.CLOSED
            if self.on_state_change:
                self.on_state_change(name, old_state, CircuitState.CLOSED)
    
    def _on_unhealthy(self, name: str, provider):
        """Handle unhealthy provider."""
        old_state = provider.circuit_state
        provider.consecutive_failures += 1
        provider.last_failure_time = time.time()
        
        if (provider.consecutive_failures >= self.failure_threshold 
            and old_state == CircuitState.CLOSED):
            provider.circuit_state = CircuitState.OPEN
            if self.on_state_change:
                self.on_state_change(name, old_state, CircuitState.OPEN)
```

### 7.3 Failover Triggers

| Trigger | HTTP Status | Action |
|---------|-------------|--------|
| Rate Limited | `429` | Increment failure, try next provider |
| Server Error | `500`, `502`, `503` | Increment failure, try next provider |
| Timeout | N/A | Increment failure, try next provider |
| Auth Error | `401`, `403` | Log error, try next provider (different key) |
| Content Filter | `400` (specific) | Return error to client (don't retry) |

---

## 8. Project Structure

![Project Structure](assets/diagram-showing-the-project-directory-structure-wi-1783410352643.svg)

```
ai-api-gateway/
│
├── main.py                          # FastAPI app entry point
├── config.py                        # Configuration loader
├── requirements.txt                 # Python dependencies
├── Dockerfile                       # Container definition
├── docker-compose.yml               # Multi-service setup
├── .env.example                     # Environment template
│
└── src/
    ├── __init__.py
    │
    ├── converters/                  # Format translation layer
    │   ├── __init__.py
    │   ├── anthropic_to_openai.py   # Request: Anthropic → OpenAI
    │   ├── openai_to_anthropic.py   # Response: OpenAI → Anthropic
    │   ├── stream_converter.py      # SSE stream translation
    │   └── tool_converter.py        # Tool/function call mapping
    │
    ├── router/                      # Routing engine
    │   ├── __init__.py
    │   ├── engine.py                # Main router logic
    │   ├── strategies/
    │   │   ├── __init__.py
    │   │   ├── round_robin.py       # Round robin strategy
    │   │   ├── fallback.py          # Priority fallback
    │   │   ├── weighted.py          # Weighted distribution
    │   │   └── least_latency.py     # Latency-based routing
    │   └── circuit_breaker.py       # Circuit breaker pattern
    │
    ├── providers/                   # Provider adapters
    │   ├── __init__.py
    │   ├── base.py                  # Abstract base provider
    │   ├── openai.py                # OpenAI adapter
    │   ├── anthropic.py             # Anthropic passthrough
    │   ├── deepseek.py              # DeepSeek adapter
    │   ├── openrouter.py            # OpenRouter adapter
    │   └── ollama.py                # Local Ollama adapter
    │
    ├── middleware/                   # Request middleware
    │   ├── __init__.py
    │   ├── auth.py                  # Authentication
    │   ├── rate_limiter.py          # Per-client rate limiting
    │   └── logging.py               # Request/response logging
    │
    ├── models/                      # Pydantic schemas
    │   ├── __init__.py
    │   ├── anthropic_schemas.py     # Anthropic API models
    │   ├── openai_schemas.py        # OpenAI API models
    │   └── provider_schemas.py      # Provider config models
    │
    └── health/                      # Health monitoring
        ├── __init__.py
        ├── monitor.py               # Health check loop
        └── healthcheck.py           # Endpoint handlers
```

---

## 9. API Reference

### 9.1 Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/messages` | Main chat endpoint (Anthropic format) |
| `POST` | `/v1/messages/count_tokens` | Token counting (estimate) |
| `GET` | `/v1/models` | List available models |
| `GET` | `/health` | Health check |
| `GET` | `/health/providers` | Provider status detail |
| `GET` | `/admin/stats` | Usage statistics |

### 9.2 POST /v1/messages

**Request (Anthropic format):**

```json
{
  "model": "claude-sonnet-4-6",
  "max_tokens": 4096,
  "system": "You are a helpful assistant.",
  "messages": [
    {
      "role": "user",
      "content": "Hello, what is 2+2?"
    }
  ],
  "stream": false,
  "temperature": 0.7,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get weather for a location",
      "input_schema": {
        "type": "object",
        "properties": {
          "location": {"type": "string"}
        },
        "required": ["location"]
      }
    }
  ],
  "tool_choice": {"type": "auto"}
}
```

**Response (Anthropic format):**

```json
{
  "id": "msg_abc123def456",
  "type": "message",
  "role": "assistant",
  "content": [
    {
      "type": "text",
      "text": "2 + 2 equals 4."
    }
  ],
  "model": "gpt-4o",
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "usage": {
    "input_tokens": 25,
    "output_tokens": 12,
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0
  }
}
```

**Response with Tool Use:**

```json
{
  "id": "msg_abc123def456",
  "type": "message",
  "role": "assistant",
  "content": [
    {
      "type": "text",
      "text": "Let me check the weather for you."
    },
    {
      "type": "tool_use",
      "id": "toolu_01ABC123",
      "name": "get_weather",
      "input": {
        "location": "Jakarta, Indonesia"
      }
    }
  ],
  "model": "gpt-4o",
  "stop_reason": "tool_use",
  "stop_sequence": null,
  "usage": {
    "input_tokens": 150,
    "output_tokens": 45,
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0
  }
}
```

**Streaming Response (SSE):**

```
event: message_start
data: {"type":"message_start","message":{"id":"msg_abc","type":"message","role":"assistant","content":[],"model":"gpt-4o","stop_reason":null,"usage":{"input_tokens":25,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"!"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}

event: message_stop
data: {"type":"message_stop"}
```

### 9.3 GET /health/providers

```json
{
  "status": "healthy",
  "providers": {
    "openai-primary": {
      "status": "healthy",
      "circuit_state": "closed",
      "avg_latency_ms": 850,
      "total_requests": 1520,
      "total_failures": 3,
      "failure_rate": "0.2%"
    },
    "anthropic-fallback": {
      "status": "healthy",
      "circuit_state": "closed",
      "avg_latency_ms": 920,
      "total_requests": 45,
      "total_failures": 0,
      "failure_rate": "0%"
    },
    "deepseek-cheap": {
      "status": "degraded",
      "circuit_state": "half_open",
      "avg_latency_ms": 2100,
      "total_requests": 300,
      "total_failures": 12,
      "failure_rate": "4%"
    }
  }
}
```

---

## 10. Konfigurasi & Environment Variables

### 10.1 .env.example

```bash
# ============================================================
# AI API Gateway Configuration
# ============================================================

# ---- Gateway Settings ----
GATEWAY_PORT=8080
GATEWAY_HOST=0.0.0.0
GATEWAY_SECRET=your-gateway-secret-key
DEBUG=false
LOG_LEVEL=INFO

# ---- Routing Strategy ----
# Options: round_robin, fallback, weighted, least_latency
ROUTING_STRATEGY=weighted
FALLBACK_ON_ERROR=true
MAX_RETRIES=2

# ---- Provider: OpenAI ----
OPENAI_ENABLED=true
OPENAI_API_KEY=sk-xxx
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_MODEL=gpt-4o
OPENAI_WEIGHT=50
OPENAI_PRIORITY=1
OPENAI_TIMEOUT=120
OPENAI_MAX_RPM=60

# ---- Provider: Anthropic (Passthrough) ----
ANTHROPIC_ENABLED=true
ANTHROPIC_API_KEY=sk-ant-xxx
ANTHROPIC_BASE_URL=https://api.anthropic.com/v1
ANTHROPIC_MODEL=claude-sonnet-4-6
ANTHROPIC_WEIGHT=30
ANTHROPIC_PRIORITY=2

# ---- Provider: DeepSeek ----
DEEPSEEK_ENABLED=true
DEEPSEEK_API_KEY=sk-xxx
DEEPSEEK_BASE_URL=https://api.deepseek.com/v1
DEEPSEEK_MODEL=deepseek-chat
DEEPSEEK_WEIGHT=20
DEEPSEEK_PRIORITY=3

# ---- Provider: OpenRouter ----
OPENROUTER_ENABLED=false
OPENROUTER_API_KEY=sk-or-xxx
OPENROUTER_BASE_URL=https://openrouter.ai/api/v1
OPENROUTER_MODEL=anthropic/claude-sonnet-4

# ---- Provider: Ollama (Local) ----
OLLAMA_ENABLED=false
OLLAMA_BASE_URL=http://localhost:11434/v1
OLLAMA_MODEL=llama3.1:70b

# ---- Model Mapping ----
# Maps Claude model names to target provider models
BIG_MODEL=gpt-4o           # claude-opus → this
MIDDLE_MODEL=gpt-4o        # claude-sonnet → this
SMALL_MODEL=gpt-4o-mini    # claude-haiku → this
FORCE_MODEL=false          # Override all model names

# ---- Circuit Breaker ----
FAILURE_THRESHOLD=5
COOLDOWN_SECONDS=60
HEALTH_CHECK_INTERVAL=30

# ---- Rate Limiting ----
RATE_LIMIT_ENABLED=true
RATE_LIMIT_RPM=100          # Requests per minute per client
RATE_LIMIT_BURST=20

# ---- Client Authentication ----
# Keys that clients can use to access the gateway
VALID_API_KEYS=["your-client-key-1","your-client-key-2"]
```

### 10.2 Konfigurasi YAML (Alternative)

```yaml
# config.yaml
gateway:
  port: 8080
  host: 0.0.0.0
  debug: false

routing:
  strategy: weighted
  fallback_on_error: true
  max_retries: 2

model_mapping:
  claude-opus-4: ${BIG_MODEL:-gpt-4o}
  claude-sonnet-4-6: ${MIDDLE_MODEL:-gpt-4o}
  claude-haiku-4-5: ${SMALL_MODEL:-gpt-4o-mini}

providers:
  - name: openai-primary
    enabled: ${OPENAI_ENABLED:-true}
    base_url: ${OPENAI_BASE_URL:-https://api.openai.com/v1}
    api_key: ${OPENAI_API_KEY}
    model: ${OPENAI_MODEL:-gpt-4o}
    weight: 50
    priority: 1
    timeout: 120
    
  - name: anthropic-secondary
    enabled: ${ANTHROPIC_ENABLED:-true}
    base_url: ${ANTHROPIC_BASE_URL:-https://api.anthropic.com/v1}
    api_key: ${ANTHROPIC_API_KEY}
    model: ${ANTHROPIC_MODEL:-claude-sonnet-4-6}
    weight: 30
    priority: 2
    passthrough: true  # Skip conversion for Anthropic models
    
  - name: deepseek-tertiary
    enabled: ${DEEPSEEK_ENABLED:-true}
    base_url: ${DEEPSEEK_BASE_URL:-https://api.deepseek.com/v1}
    api_key: ${DEEPSEEK_API_KEY}
    model: ${DEEPSEEK_MODEL:-deepseek-chat}
    weight: 20
    priority: 3

circuit_breaker:
  failure_threshold: 5
  cooldown_seconds: 60
  health_check_interval: 30

rate_limiting:
  enabled: true
  rpm: 100
  burst: 20
```

---

## 11. Deployment

### 11.1 Requirements

```txt
# requirements.txt
fastapi==0.115.0
uvicorn[standard]==0.30.0
httpx==0.27.0
pydantic==2.9.0
pydantic-settings==2.5.0
python-dotenv==1.0.1
tiktoken==0.7.0
structlog==24.4.0
```

### 11.2 Main Application

```python
# main.py

import os
import json
from contextlib import asynccontextmanager
from fastapi import FastAPI, Request, HTTPException
from fastapi.responses import StreamingResponse, JSONResponse
from fastapi.middleware.cors import CORSMiddleware

from src.converters.anthropic_to_openai import AnthropicToOpenAIConverter
from src.converters.openai_to_anthropic import OpenAIToAnthropicConverter
from src.router.engine import RouterEngine, RoutingStrategy, ProviderInstance
from src.health.monitor import HealthMonitor
from src.middleware.auth import validate_api_key
from src.providers.openai import OpenAIProvider


# ---- Configuration ----

def load_config() -> dict:
    """Load configuration from environment and YAML."""
    model_mapping = {
        "claude-opus-4": os.getenv("BIG_MODEL", "gpt-4o"),
        "claude-opus-4-0": os.getenv("BIG_MODEL", "gpt-4o"),
        "claude-sonnet-4-6": os.getenv("MIDDLE_MODEL", "gpt-4o"),
        "claude-sonnet-4-5-20250514": os.getenv("MIDDLE_MODEL", "gpt-4o"),
        "claude-haiku-4-5": os.getenv("SMALL_MODEL", "gpt-4o-mini"),
        "claude-haiku-4-5-20251001": os.getenv("SMALL_MODEL", "gpt-4o-mini"),
    }
    
    providers = []
    
    # OpenAI
    if os.getenv("OPENAI_ENABLED", "true").lower() == "true":
        providers.append(ProviderInstance(
            name="openai",
            base_url=os.getenv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
            api_key=os.getenv("OPENAI_API_KEY", ""),
            model=os.getenv("OPENAI_MODEL", "gpt-4o"),
            weight=int(os.getenv("OPENAI_WEIGHT", "50")),
            priority=int(os.getenv("OPENAI_PRIORITY", "1")),
            cooldown_seconds=float(os.getenv("COOLDOWN_SECONDS", "60")),
            failure_threshold=int(os.getenv("FAILURE_THRESHOLD", "5")),
        ))
    
    # DeepSeek
    if os.getenv("DEEPSEEK_ENABLED", "false").lower() == "true":
        providers.append(ProviderInstance(
            name="deepseek",
            base_url=os.getenv("DEEPSEEK_BASE_URL", "https://api.deepseek.com/v1"),
            api_key=os.getenv("DEEPSEEK_API_KEY", ""),
            model=os.getenv("DEEPSEEK_MODEL", "deepseek-chat"),
            weight=int(os.getenv("DEEPSEEK_WEIGHT", "20")),
            priority=int(os.getenv("DEEPSEEK_PRIORITY", "3")),
            cooldown_seconds=float(os.getenv("COOLDOWN_SECONDS", "60")),
            failure_threshold=int(os.getenv("FAILURE_THRESHOLD", "5")),
        ))
    
    # OpenRouter
    if os.getenv("OPENROUTER_ENABLED", "false").lower() == "true":
        providers.append(ProviderInstance(
            name="openrouter",
            base_url=os.getenv("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
            api_key=os.getenv("OPENROUTER_API_KEY", ""),
            model=os.getenv("OPENROUTER_MODEL", "anthropic/claude-sonnet-4"),
            weight=int(os.getenv("OPENROUTER_WEIGHT", "10")),
            priority=int(os.getenv("OPENROUTER_PRIORITY", "4")),
            cooldown_seconds=float(os.getenv("COOLDOWN_SECONDS", "60")),
            failure_threshold=int(os.getenv("FAILURE_THRESHOLD", "5")),
        ))
    
    return {
        "model_mapping": model_mapping,
        "providers": providers,
        "strategy": RoutingStrategy(os.getenv("ROUTING_STRATEGY", "weighted")),
        "max_retries": int(os.getenv("MAX_RETRIES", "2")),
    }


# ---- Application Setup ----

config = load_config()
request_converter = AnthropicToOpenAIConverter(config["model_mapping"])
response_converter = OpenAIToAnthropicConverter()
router = RouterEngine(
    providers=config["providers"],
    strategy=config["strategy"],
    max_retries=config["max_retries"]
)
health_monitor = HealthMonitor(
    providers={p.name: p for p in config["providers"]}
)

# Provider HTTP clients
provider_clients: dict[str, OpenAIProvider] = {}


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application startup/shutdown lifecycle."""
    # Startup: create provider clients and start health monitor
    for provider in config["providers"]:
        provider_clients[provider.name] = OpenAIProvider(
            base_url=provider.base_url,
            api_key=provider.api_key,
            timeout=120
        )
    await health_monitor.start()
    yield
    # Shutdown: cleanup
    await health_monitor.stop()
    for client in provider_clients.values():
        await client.client.aclose()


app = FastAPI(
    title="AI API Gateway",
    description="Anthropic ↔ OpenAI compatible proxy with multi-provider routing",
    version="1.0.0",
    lifespan=lifespan
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)


# ---- API Endpoints ----

@app.post("/v1/messages")
async def handle_messages(request: Request):
    """Main endpoint: receives Anthropic format, returns Anthropic format."""
    
    # 1. Validate authentication
    api_key = request.headers.get("x-api-key", "")
    if not validate_api_key(api_key, os.getenv("VALID_API_KEYS", "[]")):
        raise HTTPException(status_code=401, detail="Invalid API key")
    
    # 2. Parse request body
    body = await request.json()
    
    # 3. Check if should passthrough to Anthropic directly
    model = body.get("model", "")
    if model.startswith("claude-") and _should_passthrough(model):
        return await _passthrough_anthropic(body, api_key)
    
    # 4. Convert Anthropic → OpenAI format
    openai_request = request_converter.convert(body)
    openai_body = openai_request.model_dump(exclude_none=True)
    
    is_stream = body.get("stream", False)
    
    if is_stream:
        # Streaming response
        async def generate_stream():
            async def do_request(provider, data):
                client = provider_clients[provider.name]
                return client._stream_completion(data)
            
            try:
                provider = await router.select_provider()
                stream = provider_clients[provider.name]._stream_completion(openai_body)
                
                async for event in response_converter.convert_stream(
                    stream, 
                    model=provider.model,
                ):
                    yield event
                    
            except Exception as e:
                # Try fallback
                attempted = {provider.name}
                for _ in range(router.max_retries):
                    try:
                        fallback_provider = await router.select_provider(exclude=attempted)
                        stream = provider_clients[fallback_provider.name]._stream_completion(openai_body)
                        
                        async for event in response_converter.convert_stream(
                            stream, 
                            model=fallback_provider.model,
                        ):
                            yield event
                        return
                    except Exception:
                        attempted.add(fallback_provider.name)
                
                # All failed
                yield response_converter._sse_event("error", {
                    "type": "error",
                    "error": {"type": "api_error", "message": str(e)}
                })
        
        return StreamingResponse(
            generate_stream(),
            media_type="text/event-stream",
            headers={
                "Cache-Control": "no-cache",
                "Connection": "keep-alive",
                "X-Accel-Buffering": "no",
            }
        )
    
    else:
        # Non-streaming response
        async def do_request(provider, data):
            client = provider_clients[provider.name]
            return await client.chat_completion(data)
        
        result = await router.execute_with_fallback(
            do_request, 
            openai_body
        )
        
        # Convert OpenAI response → Anthropic format
        anthropic_response = response_converter.convert_non_stream(
            result, 
            model=result.get("model", openai_body["model"])
        )
        
        return JSONResponse(content=anthropic_response)


@app.post("/v1/messages/count_tokens")
async def count_tokens(request: Request):
    """Approximate token counting."""
    body = await request.json()
    messages = body.get("messages", [])
    
    # Use tiktoken for estimation
    import tiktoken
    try:
        enc = tiktoken.encoding_for_model("gpt-4")
    except Exception:
        enc = tiktoken.get_encoding("cl100k_base")
    
    total_tokens = 0
    for msg in messages:
        content = msg.get("content", "")
        if isinstance(content, str):
            total_tokens += len(enc.encode(content))
        elif isinstance(content, list):
            for block in content:
                if isinstance(block, dict) and block.get("type") == "text":
                    total_tokens += len(enc.encode(block.get("text", "")))
    
    # Add system prompt tokens
    system = body.get("system", "")
    if system:
        if isinstance(system, str):
            total_tokens += len(enc.encode(system))
    
    return {"input_tokens": total_tokens}


@app.get("/v1/models")
async def list_models():
    """List available models."""
    models = []
    for provider in config["providers"]:
        models.append({
            "id": provider.model,
            "object": "model",
            "owned_by": provider.name,
            "capabilities": {}
        })
    
    return {"object": "list", "data": models}


@app.get("/health")
async def health_check():
    """Basic health check."""
    return {"status": "ok"}


@app.get("/health/providers")
async def provider_health():
    """Detailed provider health status."""
    statuses = {}
    for name, provider in router.providers.items():
        statuses[name] = {
            "circuit_state": provider.circuit_state.value,
            "avg_latency_ms": round(provider.avg_latency_ms, 1),
            "total_requests": provider.total_requests,
            "total_failures": provider.total_failures,
            "failure_rate": (
                f"{(provider.total_failures/provider.total_requests*100):.1f}%"
                if provider.total_requests > 0 else "0%"
            ),
            "weight": provider.weight,
            "priority": provider.priority,
        }
    
    return {
        "strategy": router.strategy.value,
        "providers": statuses
    }


# ---- Helper Functions ----

def _should_passthrough(model: str) -> bool:
    """Check if model should be passed directly to Anthropic."""
    # If model is Claude and Anthropic provider exists, passthrough
    return (
        os.getenv("ANTHROPIC_PASSTHROUGH", "false").lower() == "true"
        and "anthropic" in [p.name for p in config["providers"]]
    )


async def _passthrough_anthropic(body: dict, api_key: str):
    """Forward request directly to Anthropic API."""
    import httpx
    
    async with httpx.AsyncClient() as client:
        response = await client.post(
            f"{os.getenv('ANTHROPIC_BASE_URL', 'https://api.anthropic.com')}/v1/messages",
            headers={
                "x-api-key": os.getenv("ANTHROPIC_API_KEY"),
                "anthropic-version": "2023-06-01",
                "content-type": "application/json",
            },
            json=body,
            timeout=120
        )
        
        if body.get("stream"):
            return StreamingResponse(
                response.aiter_bytes(),
                media_type="text/event-stream"
            )
        
        return JSONResponse(
            content=response.json(),
            status_code=response.status_code
        )


# ---- Entry Point ----

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(
        "main:app",
        host=os.getenv("GATEWAY_HOST", "0.0.0.0"),
        port=int(os.getenv("GATEWAY_PORT", "8080")),
        reload=os.getenv("DEBUG", "false").lower() == "true",
        log_level=os.getenv("LOG_LEVEL", "info").lower(),
    )
```

### 11.3 Dockerfile

```dockerfile
FROM python:3.12-slim

WORKDIR /app

# Install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy application
COPY . .

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
    CMD curl -f http://localhost:8080/health || exit 1

# Run
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8080"]
```

### 11.4 docker-compose.yml

```yaml
version: "3.8"

services:
  ai-gateway:
    build: .
    ports:
      - "8080:8080"
    environment:
      - GATEWAY_PORT=8080
      - ROUTING_STRATEGY=weighted
      - MAX_RETRIES=2
      
      # Provider: OpenAI
      - OPENAI_ENABLED=true
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - OPENAI_MODEL=gpt-4o
      - OPENAI_WEIGHT=50
      - OPENAI_PRIORITY=1
      
      # Provider: DeepSeek
      - DEEPSEEK_ENABLED=true
      - DEEPSEEK_API_KEY=${DEEPSEEK_API_KEY}
      - DEEPSEEK_MODEL=deepseek-chat
      - DEEPSEEK_WEIGHT=30
      - DEEPSEEK_PRIORITY=2
      
      # Provider: OpenRouter
      - OPENROUTER_ENABLED=true
      - OPENROUTER_API_KEY=${OPENROUTER_API_KEY}
      - OPENROUTER_MODEL=anthropic/claude-sonnet-4
      - OPENROUTER_WEIGHT=20
      - OPENROUTER_PRIORITY=3
      
      # Model mapping
      - BIG_MODEL=gpt-4o
      - MIDDLE_MODEL=gpt-4o
      - SMALL_MODEL=gpt-4o-mini
      
      # Circuit breaker
      - FAILURE_THRESHOLD=5
      - COOLDOWN_SECONDS=60
      
      # Auth
      - GATEWAY_SECRET=${GATEWAY_SECRET}
      - VALID_API_KEYS=["${CLIENT_API_KEY}"]
    env_file:
      - .env
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 5s
      retries: 3
```

### 11.5 Claude Code Integration

Untuk menggunakan gateway ini dengan Claude Code:

```bash
# Set environment variables for Claude Code
export ANTHROPIC_BASE_URL=http://localhost:8080
export ANTHROPIC_API_KEY=your-client-key

# Run Claude Code
claude
```

Atau secara inline:

```bash
ANTHROPIC_BASE_URL=http://localhost:8080 ANTHROPIC_API_KEY=your-client-key claude
```

---

## 12. Testing

### 12.1 Manual Testing dengan curl

```bash
# Test non-streaming request
curl -X POST http://localhost:8080/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: your-client-key" \
  -d '{
    "model": "claude-sonnet-4-6",
    "max_tokens": 100,
    "messages": [
      {"role": "user", "content": "Hello, say hi in one word."}
    ]
  }'

# Test streaming request
curl -X POST http://localhost:8080/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: your-client-key" \
  -N \
  -d '{
    "model": "claude-haiku-4-5",
    "max_tokens": 100,
    "stream": true,
    "messages": [
      {"role": "user", "content": "Count from 1 to 5."}
    ]
  }'

# Test with tool use
curl -X POST http://localhost:8080/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: your-client-key" \
  -d '{
    "model": "claude-sonnet-4-6",
    "max_tokens": 200,
    "messages": [
      {"role": "user", "content": "What is the weather in Jakarta?"}
    ],
    "tools": [
      {
        "name": "get_weather",
        "description": "Get current weather for a location",
        "input_schema": {
          "type": "object",
          "properties": {
            "location": {"type": "string"}
          },
          "required": ["location"]
        }
      }
    ]
  }'

# Test health endpoint
curl http://localhost:8080/health/providers

# Test token counting
curl -X POST http://localhost:8080/v1/messages/count_tokens \
  -H "Content-Type: application/json" \
  -H "x-api-key: your-client-key" \
  -d '{
    "model": "claude-sonnet-4-6",
    "messages": [
      {"role": "user", "content": "Hello, how are you today?"}
    ]
  }'
```

### 12.2 Automated Tests

```python
# tests/test_integration.py

import pytest
import httpx

BASE_URL = "http://localhost:8080"
HEADERS = {
    "Content-Type": "application/json",
    "x-api-key": "test-key"
}

@pytest.mark.asyncio
async def test_basic_message():
    async with httpx.AsyncClient() as client:
        response = await client.post(
            f"{BASE_URL}/v1/messages",
            headers=HEADERS,
            json={
                "model": "claude-sonnet-4-6",
                "max_tokens": 50,
                "messages": [
                    {"role": "user", "content": "Say hello"}
                ]
            }
        )
        assert response.status_code == 200
        data = response.json()
        assert data["type"] == "message"
        assert data["role"] == "assistant"
        assert len(data["content"]) > 0
        assert data["stop_reason"] == "end_turn"

@pytest.mark.asyncio
async def test_streaming():
    async with httpx.AsyncClient() as client:
        async with client.stream(
            "POST",
            f"{BASE_URL}/v1/messages",
            headers=HEADERS,
            json={
                "model": "claude-haiku-4-5",
                "max_tokens": 50,
                "stream": True,
                "messages": [
                    {"role": "user", "content": "Say hi"}
                ]
            }
        ) as response:
            assert response.status_code == 200
            events = []
            async for line in response.aiter_lines():
                if line.startswith("event: "):
                    events.append(line[7:])
            
            assert "message_start" in events
            assert "content_block_start" in events
            assert "message_stop" in events

@pytest.mark.asyncio
async def test_auth_rejected():
    async with httpx.AsyncClient() as client:
        response = await client.post(
            f"{BASE_URL}/v1/messages",
            headers={"Content-Type": "application/json", "x-api-key": "invalid"},
            json={
                "model": "claude-sonnet-4-6",
                "messages": [{"role": "user", "content": "test"}]
            }
        )
        assert response.status_code == 401

@pytest.mark.asyncio
async def test_health_endpoint():
    async with httpx.AsyncClient() as client:
        response = await client.get(f"{BASE_URL}/health")
        assert response.status_code == 200
        assert response.json()["status"] == "ok"

@pytest.mark.asyncio
async def test_provider_health():
    async with httpx.AsyncClient() as client:
        response = await client.get(
            f"{BASE_URL}/health/providers",
            headers=HEADERS
        )
        assert response.status_code == 200
        data = response.json()
        assert "strategy" in data
        assert "providers" in data
```

### 12.3 Load Testing

```bash
# Using wrk for load testing
wrk -t4 -c100 -d30s -s load_test.lua http://localhost:8080/v1/messages

# load_test.lua content:
# wrk.method = "POST"
# wrk.headers["Content-Type"] = "application/json"
# wrk.headers["x-api-key"] = "your-key"
# wrk.body = '{"model":"claude-haiku-4-5","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}'
```

---

## 📊 Ringkasan

| Aspek | Detail |
|-------|--------|
| **Input Format** | Anthropic Messages API (`/v1/messages`) |
| **Output Format** | Anthropic Messages API (dari Claude Code perspective) |
| **Internal Format** | OpenAI Chat Completions API (`/v1/chat/completions`) |
| **Streaming** | Full SSE support dengan event translation |
| **Tool Use** | Anthropic tool_use ↔ OpenAI function calling |
| **Routing** | Round Robin, Fallback, Weighted, Least Latency |
| **Resilience** | Circuit Breaker, Auto Retry, Failover |
| **Providers** | OpenAI, Anthropic, DeepSeek, OpenRouter, Ollama, Azure, Groq, dll |
| **Framework** | Python + FastAPI + httpx |
| **Deployment** | Docker-ready |

---

Dokumentasi ini mencakup seluruh aspek pembangunan AI API Gateway dari arsitektur tingkat tinggi hingga kode implementasi detail. Tinggal disesuaikan dengan kebutuhan spesifik project.
