# Refactoring: ProviderFactory + ContextManager

## Resumen

Se extrajo la lógica de construcción de providers y manejo de contexto en módulos separados, reduciendo acoplamiento y deuda técnica.

## Cambios Principales

### 1. ProviderFactory (`internal/agent/providers/`)

**Antes:** Toda la lógica de construcción de providers estaba en `coordinator.go` (~500 líneas).

**Ahora:** Factory pattern con builders separados:

```
internal/agent/providers/
├── factory.go        # ProviderFactory - routing central
├── openai.go        # Builder para OpenAI
├── anthropic.go     # Builder para Anthropic (incluye MiniMax)
├── openrouter.go    # Builder para OpenRouter
└── openaicompat.go # Builder para OpenAI-compatible (DeepSeek, Zhipu, etc.)
```

**Beneficios:**
- Agregar nuevo provider = nuevo archivo, no modificar coordinator.go
- Coordinator solo depende de `factory.Build()`
- Cada builder testeable aisladamente
- Providers soportados: openai, anthropic, openrouter, openaicompat

### 2. ContextManager (`internal/agent/context.go`)

**Antes:** Lógica de contexto mezclada en `agent.go`.

**Ahora:** Módulo separado con responsabilidad única:
- `ShouldStop()` - decide cuándo parar por límite de tokens
- `ShouldSummarize()` - decide cuándo resumir conversación
- `GhostCount` - estimación de tokens (módulo nuevo en `internal/ghostcount/`)

### 3. Template Reducido (`internal/agent/templates/coder.md.tpl`)

**Antes:** 392 líneas con "AI slop" - reglas redundantes, ejemplos excesivos.

**Ahora:** ~50 líneas concisas:

```markdown
You are Crush, a CLI AI Assistant.

**Rules:**
1. Read files before editing - match whitespace exactly
2. Be autonomous - search, decide, act
3. Test after changes
4. Be concise (<4 lines text unless explaining)
5. Never commit/push unless asked
6. Security first - defensive only
7. No emojis

**Communication:** One-word answers when possible. No preamble/postamble.
**Code refs:** Use `file:line` pattern.
```

### 4. Coordinator Limpio (`internal/agent/coordinator.go`)

- ~200 líneas removidas de código duplicado
- Providers eliminados que no se usaban: azure, vercel, bedrock, google, hyper
- Imports limpiados

## Arquitectura

```
                    ┌─────────────────────────────────────┐
                    │           Coordinator               │
                    │  (solo orchestration, ~300 líneas)   │
                    └──────────────┬──────────────────────┘
                                   │
                    ┌──────────────┼──────────────┐
                    ▼              ▼              ▼
           ┌────────────┐  ┌───────────┐  ┌────────────┐
           │ Providers/ │  │ ContextMgr │  │  Tools/    │
           │  Factory   │  │            │  │  Factory   │
           └────────────┘  └────────────┘  └────────────┘
```

## Estado de Tests

| Test Suite | Estado | Notas |
|------------|--------|-------|
| `go build ./...` | ✅ PASA | |
| `go test ./internal/agent/tools/...` | ✅ PASA | |
| `go test ./internal/config/...` | ✅ PASA | |
| `TestCoderAgent/*` | ❌ FALLA | Usan mocks vcr que requieren re-grabación |

### Por qué fallan TestCoderAgent

Los tests usan vcr para grabar/reproducir requests HTTP. Cuando se cambia el template (que va en el request), el mock no hace match.

**Solución:** Re-grabar fixtures con API real:

```bash
# Requiere API key con créditos
export MINIMAX_API_KEY="tu-key"
go test ./internal/agent/... -record
```

## Uso con Providers

### MiniMax (recomendado)

```bash
export MINIMAX_API_KEY="tu-key"
./crush --model minimax/MiniMax-M2.7-highspeed run "tu prompt"
```

### DeepSeek

```json
{
  "providers": {
    "deepseek": {
      "type": "openai-compat",
      "base_url": "https://api.deepseek.com/v1",
      "api_key": "$DEEPSEEK_API_KEY",
      "models": [{"id": "deepseek-chat", "name": "Deepseek V3", ...}]
    }
  }
}
```

### Modelos Soportados por Factory

- `openai` - OpenAI direct
- `anthropic` - Anthropic direct (incluye MiniMax)
- `openrouter` - OpenRouter
- `openaicompat` / `openai-compat` - Cualquier API OpenAI-compatible

## Archivos Creados

```
internal/agent/providers/factory.go       # ProviderFactory
internal/agent/providers/openai.go        # OpenAI builder
internal/agent/providers/anthropic.go     # Anthropic builder
internal/agent/providers/openrouter.go   # OpenRouter builder
internal/agent/providers/openaicompat.go  # OpenAI-compatible builder
internal/agent/context.go                # ContextManager
internal/ghostcount/                     # Token estimation
```

## Archivos Modificados

```
internal/agent/coordinator.go           # Removido código de providers
internal/agent/agent.go                 # Uses ContextManager
internal/agent/templates/coder.md.tpl   # Template reducido
internal/config/config.go               # EnableGhostCount option
```

## Siguientes Pasos (Plan)

- [ ] Re-grabar fixtures de TestCoderAgent
- [ ] Fase 4-5: ToolFactory extraction
- [ ] Reducir más templates (summary.md)
- [ ] Tests de integración con providers reales

## Commits

- `refactor/provider-factory-context-manager` - Rama principal con todos los cambios
