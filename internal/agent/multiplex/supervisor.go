// Package multiplex provides multi-agent communication primitives.
package multiplex

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Supervisor coordinates sub-agents to complete a task.
type Supervisor struct {
	config SupervisorConfig
	ctx    context.Context
	cancel  context.CancelFunc

	intentChan chan Intent
	resultChan chan Result
	partitioner *TaskPartitioner
	pool       *Pool
	aggregator *Aggregator
	ghost      *GhostManager
}

// SupervisorConfig configures the supervisor.
type SupervisorConfig struct {
	// Name is the supervisor's identifier.
	Name string
	// PoolSize is the number of sub-agents.
	PoolSize int
	// ProcessFunc processes a single intent (the actual agent logic).
	ProcessFunc func(ctx context.Context, intent Intent) Result
	// PartitionConfig controls how tasks are split.
	PartitionConfig PartitionConfig
	// GhostConfig configures GhostCount context management.
	// If nil, GhostCount is disabled.
	GhostConfig *GhostConfig
}

// NewSupervisor creates a supervisor.
func NewSupervisor(ctx context.Context, config SupervisorConfig) *Supervisor {
	supCtx, cancel := context.WithCancel(ctx)

	intentChan := make(chan Intent, config.PoolSize)
	resultChan := make(chan Result, config.PoolSize)

	partitioner := NewTaskPartitioner()

	pool := NewPool(supCtx, PoolConfig{
		Size:        config.PoolSize,
		IntentChan:  intentChan,
		ResultChan: resultChan,
		ProcessFunc: config.ProcessFunc,
	})

	aggregator := NewAggregator(supCtx, resultChan)

	ghostConfig := DefaultGhostConfig()
	if config.GhostConfig != nil {
		ghostConfig = *config.GhostConfig
	}
	ghost := NewGhostManager(ghostConfig)

	return &Supervisor{
		config:      config,
		ctx:        supCtx,
		cancel:      cancel,
		intentChan: intentChan,
		resultChan: resultChan,
		partitioner: partitioner,
		pool:       pool,
		aggregator: aggregator,
		ghost:      ghost,
	}
}

// Start initializes all sub-agents. Non-blocking.
func (s *Supervisor) Start() {
	s.pool.Start()
	s.aggregator.Start()
}

// Stop gracefully shuts down all components.
func (s *Supervisor) Stop() {
	s.cancel()
	s.pool.Stop()
	s.aggregator.Stop()
}

// Task represents a task to be executed by the supervisor.
type Task struct {
	ID          string
	Description string
	Resources   []string
}

// Result is the final combined result from all sub-agents.
type SupervisorResult struct {
	TaskID       string
	Combined     string
	FilesChanged []string
	Status       ResultStatus
	Errors       []error
}

// Execute runs a task and returns the combined result.
func (s *Supervisor) Execute(ctx context.Context, task Task) (*SupervisorResult, error) {
	slog.Info("supervisor: executing task", "name", s.config.Name, "task", task.ID)

	// Partition the task
	partitionResult, err := s.partitioner.Partition(
		task.ID,
		task.Description,
		task.Resources,
		s.config.PartitionConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("partition failed: %w", err)
	}

	slog.Info("supervisor: partitioned into", "count", len(partitionResult.Intents), "strategy", partitionResult.Strategy)

	// Tell aggregator how many results to expect
	s.aggregator.SetExpectedCount(task.ID, len(partitionResult.Intents))

	// Set up result collection
	resultCh := make(chan *SupervisorResult, 1)
	s.aggregator.OnComplete(func(taskID string, results []Result) {
		combined := AggregateResult(results, taskID)
		supervisorResult := &SupervisorResult{
			TaskID:       taskID,
			Combined:     combined.Output,
			FilesChanged: combined.FilesModified,
			Status:       combined.Status,
		}
		select {
		case resultCh <- supervisorResult:
		default:
		}
	})

	// Dispatch intents to workers
	for _, intent := range partitionResult.Intents {
		select {
		case s.intentChan <- intent:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Wait for completion
	select {
	case result := <-resultCh:
		slog.Info("supervisor: task complete", "task", task.ID, "status", result.Status)
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Minute): // TODO: make configurable
		return nil, fmt.Errorf("task timeout after 5 minutes")
	}
}

// ExecuteWithTimeout runs a task with a specific timeout.
func (s *Supervisor) ExecuteWithTimeout(ctx context.Context, task Task, timeout time.Duration) (*SupervisorResult, error) {
	taskCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return s.Execute(taskCtx, task)
}
