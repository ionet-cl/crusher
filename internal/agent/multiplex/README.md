# Multiplex: Multi-Agent Coordination for Crusher

Pure-Go channel primitives for coordinating multiple sub-agents to complete large tasks in parallel.

## Design Philosophy

- **DRY + SOLID + KISS + LEAN**: Minimal, focused code with clear responsibilities
- **No AI lore slop**: No embeddings, semantic indexes, or mmap complexity
- **Zero-latency context sharing**: Agents share **intentions**, not context
- **Pure Go channels**: Communication via channels only, no shared memory, no mutex
- **Transparent integration**: Automatic use by Crusher, no user configuration needed

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         Supervisor                           │
│  ┌─────────────┐  ┌─────────┐  ┌───────────┐  ┌────────┐ │
│  │ TaskPartioner│→│IntentChan│→ │  Worker   │→│ResultChan│
│  └─────────────┘  └─────────┘  │   Pool    │  └────────┘ │
│                                └─────┬─────┘     ↓          │
│  ┌──────────────┐                    │    ┌───────────┐   │
│  │GhostManager  │←───────────────────┘    │Aggregator │   │
│  └──────────────┘                         └───────────┘   │
└─────────────────────────────────────────────────────────────┘
                              ↓
                    SupervisorResult
```

## Core Components

### Intent (What We Want)

```go
type Intent struct {
    ID, TaskID, Role     // Who/what identification
    Goal                // What to achieve
    Constraints []Constraint // must/must_not/prefer/avoid
    Resources []string   // Paths/URIs agent can access
    Priority int        // Execution order
}
```

### RepoScanner (Automatic Repository Discovery)

Scans any local repository and finds relevant code files:

```go
scanner := NewRepoScanner()
contents, err := scanner.Scan(ctx, RepoSpec{
    ID:   "my-repo",
    Root: "/path/to/repo",
    Type: RepoTypeLocal,
})
```

### AgentProcessFunc (Transparent Agent Integration)

Creates a ProcessFunc that uses the real SessionAgent:

```go
processFunc := AgentProcessFunc(sessionAgent)
```

## Usage

### Automatic Multi-Repo Processing

```go
// Scan repositories
scanner := NewRepoScanner()
repos := []RepoSpec{
    {ID: "repo-a", Root: "/path/to/repo-a", Type: RepoTypeLocal},
    {ID: "repo-b", Root: "/path/to/repo-b", Type: RepoTypeLocal},
}

// Create intents for each repo, partitioned by module
var allIntents []Intent
for _, repo := range repos {
    contents, _ := scanner.Scan(ctx, repo)
    intents := IntentFromRepoContents(contents, taskID, "Analyze code", "module")
    allIntents = append(allIntents, intents...)
}

// Execute with real agent
supervisor := NewSupervisor(ctx, SupervisorConfig{
    PoolSize:      4,
    ProcessFunc:   AgentProcessFunc(sessionAgent),
    PartitionConfig: DefaultPartitionConfig(),
})
supervisor.Start()
result, _ := supervisor.Execute(ctx, Task{
    ID:          taskID,
    Description: "Analyze code",
    Resources:   nil, // resources come from repo scanning
})
```

### Transparent Parallel File Processing

When Crusher processes a task with multiple files, multiplex automatically distributes work across workers:

```go
// Supervisor handles partitioning and parallel execution automatically
supervisor := NewSupervisor(ctx, SupervisorConfig{
    Name:        "auto-multiplex",
    PoolSize:    4,
    ProcessFunc: AgentProcessFunc(sessionAgent),
    PartitionConfig: PartitionConfig{
        MaxIntents:        10,
        MaxTokensPerIntent: 50000,
        SplitBy:           "file", // or "module"
    },
})
```

## Testing

```bash
go test -v ./internal/agent/multiplex/...
```

## Test Results (17 tests passing)

| Test | Description | Status |
|------|-------------|--------|
| TestChannel_* (8 tests) | Channel primitives | PASS |
| TestPartitionWithRealFiles | Partition 15 files into 2 intents | PASS |
| TestPartitionByModuleWithRealFiles | Partition 80 files into 5 modules | PASS |
| TestSupervisorWithRealFilePartitioning | Process 12 real files | PASS |
| TestGhostCountCompactWithRealContent | GhostManager compaction | PASS |
| TestParallelExecutionSpeed | 6x speedup verified | PASS |
| TestSupervisorEndToEnd | End-to-end flow | PASS |
| TestSupervisorLargeContext | 50 resources, 6 intents | PASS |
| TestPartitionByModule | Module grouping | PASS |

## Key Principles

1. **Intent, not context**: Pass what to do, not what was said
2. **No shared memory**: Pure channel communication between components
3. **Zero-copy**: Results flow through channels without shared state
4. **Composable**: Each component works independently
5. **Testable**: Easy to test with mock ProcessFunc
