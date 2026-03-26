# Multiplex: Arquitectura Correcta

## Estado Actual (Implementado)

### Lo que SÍ funciona:
- ✅ Todos los workers usan el MISMO sessionID → mensajes se acumulan en sesión compartida
- ✅ Archivos se pre-levan y se incluyen en prompts → agente tiene contenido real
- ✅ Outputs de todos los workers se combinan → usuario ve todo
- ✅ FileLockManager infrastructure existe para coordinar edits
- ✅ ToolRouter infrastructure existe para routing de tools

### Lo que NO está implementado:
- ❌ Tool calls no se interceptan - agente hace sus propias llamadas de tools
- ❌ FileLockManager no se usa activamente - edits concurrentes al mismo archivo tienen "last-write-wins"
- ❌ GhostManager no se comparte activamente entre workers

## Arquitectura Implementada

```
┌─────────────────────────────────────────────────────────────┐
│                    Supervisor                                │
│  ┌─────────────────────────────────────────────────────┐  │
│  │         AgentProcessFunc (closure)                   │  │
│  │  - toolRouter: compartido (pero no usado activa.)  │  │
│  │  - sessionID: REAL del usuario                      │  │
│  └─────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                              │
          ┌───────────────────┼───────────────────┐
          │                   │                   │
    worker-1: agent.Run()   worker-2: agent.Run() worker-3: agent.Run()
    sessionID=real        sessionID=real       sessionID=real
          │                   │                   │
          └───────────────────┴───────────────────┘
                              │
                    Session real del usuario
                    (mensajes se acumulan)
```

## Para-Arquitectura Requerida (Falta)

Para coordinar tool calls en tiempo real, se necesitaría:

1. **Custom Agent Wrapper** - envolver `fantasy.Agent` para interceptar `OnToolCall`
2. **Tool Interception Layer** - recibir tool calls, coordinar via mutex, ejecutar
3. **Resultado acumulado** - combinar no solo outputs de texto sino también state changes

## Alternativa: Re-arquitectura Completa

Una solución completa requeriría:
- No usar `agent.Run()` directamente
- Crear un "agent orchestrator" que maneja streaming y tool calls
- Coordinar todos los tool calls en un punto central

Esto es un cambio arquitectural mayor (~1 semana de trabajo).

## Métricas de Éxito Actuales

1. ✅ **Mensajes compartidos**: Todos los workers escriben a la misma sesión
2. ✅ **6x speedup**: Parallel execution verified
3. ⚠️ **Edits coordinados**: "last-write-wins" (no hay conflicto detection activo)
4. ⚠️ **Contexto eficiente**: GhostManager no se comparte activamente
