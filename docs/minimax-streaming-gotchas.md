# MiniMax Thinking Streaming - Gotchas & Learnings

## Summary

Getting MiniMax's reasoning/think content to stream in real-time via `--ai-debug` (X-RAY VISION mode) required understanding the relationship between MiniMax's API endpoints, authentication methods, and fantasy SDK's callback mechanisms.

---

## Key Discovery: MiniMax Has TWO API Endpoints

MiniMax offers two different API endpoints that look similar but behave differently:

| Endpoint | Auth | Streaming Format | Fantasy SDK Support |
|----------|------|------------------|---------------------|
| `https://api.minimax.io/anthropic` | Bearer token | `thinking_delta` (Anthropic format) | **YES** ✅ |
| `https://api.minimax.io/v1` | API Key | `reasoning_details_delta` (OpenAI format) | **NO** ❌ |

### Why This Matters

- `thinking_delta` is what fantasy SDK's `OnReasoningDelta` callback handles
- `reasoning_details_delta` is a different event type that fantasy doesn't recognize
- Using the wrong endpoint = no streaming think content even with `reasoning_split: true`

---

## Gotcha #1: Don't Use `openaicompat` for MiniMax

**Problem**: Initially tried using `openaicompat` type with `https://api.minimax.io/v1` to pass `reasoning_split: true` via `extra_body`.

**Result**: Got `404 Not Found` because the URL path didn't match.

**Solution**: Use `anthropic` type with `https://api.minimax.io/anthropic` base URL.

### Wrong Configuration
```go
providerType = openaicompat.Name  // "openaicompat"
baseURL = "https://api.minimax.io/v1"  // Causes 404
```

### Correct Configuration
```go
providerType = anthropic.Name  // "anthropic"
baseURL = "https://api.minimax.io/anthropic"
```

---

## Gotcha #2: Fantasy SDK's `OnReasoningDelta` Only Handles Anthropic Format

**Problem**: Fantasy SDK's `OnReasoningDelta` callback only fires for `thinking_delta` events (Anthropic format), NOT `reasoning_details_delta` (OpenAI format).

**Evidence**:
- MiniMax `/v1` (OpenAI endpoint) sends `reasoning_details_delta` → callback **never fires**
- MiniMax `/anthropic` (Anthropic endpoint) sends `thinking_delta` → callback **fires correctly**

### Code Path
```
MiniMax API
  └── thinking_delta event
        └── fantasy SDK parses SSE
              └── OnReasoningDelta callback invoked
                    └── ThinkCallback invoked (our debug callback)
```

---

## Gotcha #3: The `thinkCallback` Must Be Passed to `SessionAgentCall`

**Problem**: The `thinkCallback` was passed to `coordinator.Run()` but never forwarded to `sessionAgent.Run()`.

**Symptom**: `OnReasoningDelta` was being called internally but debug output never showed it.

**Solution**: Add `ThinkCallback` to `SessionAgentCall` struct:

```go
// Before (missing ThinkCallback in call)
SessionAgentCall{
    SessionID:        sessionID,
    Prompt:           prompt,
    // ThinkCallback missing!
}

// After (ThinkCallback included)
SessionAgentCall{
    SessionID:        sessionID,
    Prompt:           prompt,
    ThinkCallback:    thinkCallback,  // NOW INCLUDED
}
```

---

## Gotcha #4: MiniMax Auth Uses `Authorization: Bearer` Header

**Problem**: MiniMax's Anthropic endpoint requires `Authorization: Bearer <api_key>` header, not just the API key.

**Both of these work**:
```bash
# Via header
curl -H "Authorization: Bearer $API_KEY" ...

# Via x-api-key header
curl -H "x-api-key: $API_KEY" ...
```

In fantasy SDK, the `anthropicBuilder` handles this automatically for MiniMax:
```go
case cfg.ID == "minimax" || cfg.ID == "minimax-china":
    os.Setenv("ANTHROPIC_API_KEY", "")
    cfg.Headers["Authorization"] = "Bearer " + cfg.APIKey
```

---

## Gotcha #5: `reasoning_split` Is Not Needed with Anthropic Endpoint

**Discovery**: When using MiniMax's Anthropic-compatible endpoint (`/anthropic`), thinking streams automatically via `thinking_delta`. The `reasoning_split` parameter is only needed for the OpenAI-compatible endpoint (`/v1`) to separate reasoning from content.

**With Anthropic endpoint**: Thinking streams automatically, no extra params needed.

---

## Configuration Changes Made

### `internal/agent/coordinator.go`

Added MiniMax-specific provider routing:

```go
// MiniMax: use Anthropic-compatible endpoint for streaming thinking support
// The Anthropic endpoint at https://api.minimax.io/anthropic sends thinking_delta
// events which fantasy's anthropic provider handles correctly
if providerCfg.ID == "minimax" || providerCfg.ID == "minimax-china" {
    providerType = anthropic.Name
    baseURL = "https://api.minimax.io/anthropic"
}
```

And added `ThinkCallback` to the call:

```go
run := func() (*fantasy.AgentResult, error) {
    return c.currentAgent.Run(ctx, SessionAgentCall{
        SessionID:        sessionID,
        Prompt:           prompt,
        // ... other fields ...
        ThinkCallback:    thinkCallback,  // WAS MISSING!
    })
}
```

---

## Testing Commands

### Verify Streaming Works
```bash
echo "why is the sky blue?" | ./crush chat --ai-debug
```

You should see `─── MODEL THINKING ───` section with streaming content.

### Verify API Direct (curl)
```bash
curl -s -X POST "https://api.minimax.io/anthropic/v1/messages" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ANTHROPIC_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "MiniMax-M2.7-highspeed",
    "max_tokens": 100,
    "messages": [{"role": "user", "content": "hi"}],
    "stream": true
  }'
```

---

## Related Files

- `internal/agent/coordinator.go` - Provider routing and call construction
- `internal/agent/agent.go` - `OnReasoningDelta` callback handling
- `internal/agent/providers/anthropic.go` - Anthropic provider builder
- `internal/agent/debug.go` - X-RAY VISION debug output rendering
- `internal/cmd/chat.go` - `thinkCallback` wiring for `--ai-debug`

---

## Credits

- MiniMax Documentation: https://platform.minimax.io/docs/guides/quickstart-preparation
- MiniMax API Reference: https://platform.minimax.io/docs/api-reference/text-anthropic-api
