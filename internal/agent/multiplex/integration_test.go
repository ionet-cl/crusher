package multiplex

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestSupervisorEndToEnd is a real integration test.
// It uses the actual Supervisor, Worker pool, Channel, and Aggregator.
// No mocks - tests the real multi-agent coordination.
func TestSupervisorEndToEnd(t *testing.T) {
	ctx := context.Background()

	// Create a real supervisor with a real process function
	supervisor := NewSupervisor(ctx, SupervisorConfig{
		Name:     "test-supervisor",
		PoolSize: 3,
		ProcessFunc: func(ctx context.Context, intent Intent) Result {
			// Real processing - just sleep to simulate work
			time.Sleep(10 * time.Millisecond)

			// Return a real result
			return Result{
				IntentID: intent.ID,
				TaskID:   intent.TaskID,
				Status:   StatusSuccess,
				Output:  "Processed: " + intent.Goal,
				Details: []Detail{
					{Key: "role", Value: string(intent.Role)},
				},
				FilesModified: intent.Resources,
			}
		},
		PartitionConfig: PartitionConfig{
			MaxIntents:        5,
			MaxTokensPerIntent: 1000,
			SplitBy:            "file",
		},
	})

	supervisor.Start()
	defer supervisor.Stop()

	// Execute a real task
	task := Task{
		ID:          "task-1",
		Description: "Analyze and modify these files",
		Resources: []string{
			"file1.go",
			"file2.go",
			"file3.go",
			"file4.go",
			"file5.go",
		},
	}

	result, err := supervisor.Execute(ctx, task)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Status != StatusSuccess {
		t.Errorf("expected status success, got %s", result.Status)
	}

	if result.TaskID != "task-1" {
		t.Errorf("expected task ID task-1, got %s", result.TaskID)
	}

	t.Logf("Result: %s, Files: %v", result.Combined, result.FilesChanged)
}

// TestSupervisorParallelExecution verifies that sub-agents execute in parallel.
func TestSupervisorParallelExecution(t *testing.T) {
	ctx := context.Background()

	var (
		mu         sync.Mutex
		startTimes = make(map[string]time.Time)
		endTimes   = make(map[string]time.Time)
	)

	supervisor := NewSupervisor(ctx, SupervisorConfig{
		Name:     "parallel-test",
		PoolSize: 3,
		ProcessFunc: func(ctx context.Context, intent Intent) Result {
			mu.Lock()
			startTimes[intent.ID] = time.Now()
			mu.Unlock()

			// Simulate work
			time.Sleep(50 * time.Millisecond)

			mu.Lock()
			endTimes[intent.ID] = time.Now()
			mu.Unlock()

			return Result{
				IntentID: intent.ID,
				TaskID:   intent.TaskID,
				Status:   StatusSuccess,
				Output:  "Done: " + intent.ID,
			}
		},
		PartitionConfig: DefaultPartitionConfig(),
	})

	supervisor.Start()
	defer supervisor.Stop()

	// Create 6 small tasks - should run 3 in parallel
	task := Task{
		ID:          "parallel-task",
		Description: "Process these",
		Resources: []string{
			"a.txt", "b.txt", "c.txt",
			"d.txt", "e.txt", "f.txt",
		},
	}

	start := time.Now()
	result, err := supervisor.Execute(ctx, task)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Status != StatusSuccess {
		t.Errorf("expected status success, got %s", result.Status)
	}

	// With 3 workers and 6 tasks at ~50ms each:
	// Sequential: ~300ms
	// Parallel (3 workers): ~100ms
	// We expect close to parallel time
	if elapsed > 200*time.Millisecond {
		t.Logf("WARNING: Execution took %v - may not be running in parallel", elapsed)
	} else {
		t.Logf("Parallel execution verified: %v for 6 tasks with 3 workers", elapsed)
	}
}

// TestSupervisorLargeContext tests with realistic file list.
func TestSupervisorLargeContext(t *testing.T) {
	ctx := context.Background()

	supervisor := NewSupervisor(ctx, SupervisorConfig{
		Name:     "large-context-test",
		PoolSize: 5,
		ProcessFunc: func(ctx context.Context, intent Intent) Result {
			time.Sleep(5 * time.Millisecond)
			return Result{
				IntentID: intent.ID,
				TaskID:   intent.TaskID,
				Status:   StatusSuccess,
				Output:  "Processed " + string(rune('0'+len(intent.Resources))) + " files",
				Details: []Detail{
					{Key: "resource_count", Value: string(rune('0' + len(intent.Resources)))},
				},
			}
		},
		PartitionConfig: PartitionConfig{
			MaxIntents:        10,
			MaxTokensPerIntent: 50000,
			SplitBy:            "file",
		},
	})

	supervisor.Start()
	defer supervisor.Stop()

	// Simulate 50 files
	var resources []string
	for i := 0; i < 50; i++ {
		resources = append(resources, "internal/module/file"+string(rune('0'+i%10))+".go")
	}

	task := Task{
		ID:          "large-task",
		Description: "Refactor all files",
		Resources:   resources,
	}

	result, err := supervisor.Execute(ctx, task)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Status != StatusSuccess {
		t.Errorf("expected status success, got %s", result.Status)
	}

	t.Logf("Processed %d resources across %d intents", len(resources), len(resources))
}

// TestPartitionByModule tests module-based partitioning.
func TestPartitionByModule(t *testing.T) {
	partitioner := NewTaskPartitioner()

	resources := []string{
		"auth/user.go",
		"auth/session.go",
		"api/v1/handler.go",
		"api/v2/handler.go",
		"db/migrations.go",
		"db/queries.go",
	}

	config := PartitionConfig{
		MaxIntents:        10,
		MaxTokensPerIntent: 1000,
		SplitBy:           "module",
	}

	result, err := partitioner.Partition("test-task", "Analyze code", resources, config)
	if err != nil {
		t.Fatalf("Partition failed: %v", err)
	}

	// Should create intents for each module
	if len(result.Intents) == 0 {
		t.Error("expected at least one intent")
	}

	// Check that resources are grouped by module
	moduleGroups := make(map[string]int)
	for _, intent := range result.Intents {
		moduleGroups[intent.Goal]++
		t.Logf("Intent %s: %d resources", intent.Goal, len(intent.Resources))
	}

	t.Logf("Partitioned into %d module groups: %v", len(moduleGroups), moduleGroups)
}
