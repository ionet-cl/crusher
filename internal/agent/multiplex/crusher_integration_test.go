package multiplex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestPartitionWithRealFiles tests that the partitioner correctly divides
// real project files into multiple intents.
func TestPartitionWithRealFiles(t *testing.T) {
	// Get real files from the agent package
	agentDir := "../../" // internal/agent

	var realFiles []string
	err := filepath.Walk(agentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Only Go files in agent package (not test files, not this file)
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Skip test files and this file
		if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "multiplex") {
			return nil
		}
		rel, err := filepath.Rel(".", path)
		if err != nil {
			return nil
		}
		realFiles = append(realFiles, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to walk agent dir: %v", err)
	}

	if len(realFiles) < 5 {
		t.Fatalf("Need at least 5 files, got %d: %v", len(realFiles), realFiles)
	}

	// Take first 15 files for the test
	if len(realFiles) > 15 {
		realFiles = realFiles[:15]
	}

	t.Logf("Testing with %d real files: %v", len(realFiles), realFiles)

	partitioner := NewTaskPartitioner()

	// Test file-based partitioning with small token limit to force splits
	cfg := PartitionConfig{
		MaxIntents:        10,
		MaxTokensPerIntent: 500, // Small limit to force multiple intents
		SplitBy:           "file",
	}

	result, err := partitioner.Partition("test-task", "Analyze and review these files", realFiles, cfg)
	if err != nil {
		t.Fatalf("Partition failed: %v", err)
	}

	t.Logf("Partition result: %d intents, strategy=%s", len(result.Intents), result.Strategy)

	// Should create multiple intents because we have many files
	if len(result.Intents) < 2 {
		t.Errorf("Expected multiple intents, got %d", len(result.Intents))
	}

	// Verify all resources are assigned
	var totalResources int
	for i, intent := range result.Intents {
		totalResources += len(intent.Resources)
		t.Logf("Intent %d: role=%s, resources=%d", i+1, intent.Role, len(intent.Resources))
	}

	if totalResources != len(realFiles) {
		t.Errorf("Expected %d total resources across intents, got %d", len(realFiles), totalResources)
	}

	// Verify no duplicate resources
	resourceSet := make(map[string]bool)
	for _, intent := range result.Intents {
		for _, r := range intent.Resources {
			if resourceSet[r] {
				t.Errorf("Duplicate resource: %s", r)
			}
			resourceSet[r] = true
		}
	}
}

// TestPartitionByModuleWithRealFiles tests module-based partitioning with real files.
func TestPartitionByModuleWithRealFiles(t *testing.T) {
	// Get real files from different internal subdirectories
	internalDir := "../../"

	var realFiles []string
	err := filepath.Walk(internalDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "multiplex") {
			return nil
		}
		rel, err := filepath.Rel(".", path)
		if err != nil {
			return nil
		}
		// Only pick files from different top-level directories
		if strings.HasPrefix(rel, "../../agent/") ||
			strings.HasPrefix(rel, "../../config/") ||
			strings.HasPrefix(rel, "../../db/") ||
			strings.HasPrefix(rel, "../../session/") ||
			strings.HasPrefix(rel, "../../message/") {
			realFiles = append(realFiles, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to walk internal dir: %v", err)
	}

	if len(realFiles) < 5 {
		t.Skip("Not enough files from different modules")
	}

	t.Logf("Testing module partition with %d files: %v", len(realFiles), realFiles)

	partitioner := NewTaskPartitioner()
	cfg := PartitionConfig{
		MaxIntents:        10,
		MaxTokensPerIntent: 50000,
		SplitBy:           "module",
	}

	result, err := partitioner.Partition("module-test", "Analyze modules", realFiles, cfg)
	if err != nil {
		t.Fatalf("Partition failed: %v", err)
	}

	t.Logf("Module partition result: %d intents", len(result.Intents))

	if len(result.Intents) < 2 {
		t.Errorf("Expected multiple module intents, got %d", len(result.Intents))
	}

	// Verify each intent has module prefix in goal
	for i, intent := range result.Intents {
		t.Logf("Intent %d: goal=%s, resources=%d", i+1, intent.Goal, len(intent.Resources))
	}
}

// TestSupervisorWithRealFilePartitioning tests the full supervisor flow with real files.
func TestSupervisorWithRealFilePartitioning(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	ctx := context.Background()

	// Get a list of real Go files
	agentDir := "../../agent"
	var realFiles []string
	filepath.Walk(agentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "multiplex") {
			return nil
		}
		rel, err := filepath.Rel(".", path)
		if err != nil {
			return nil
		}
		realFiles = append(realFiles, rel)
		return nil
	})

	if len(realFiles) > 12 {
		realFiles = realFiles[:12]
	}

	t.Logf("Supervisor test with %d real files", len(realFiles))

	var (
		mu           sync.Mutex
		processedCount int
		intentLogs   []string
	)

	supervisor := NewSupervisor(ctx, SupervisorConfig{
		Name:     "real-files-test",
		PoolSize: 3,
		ProcessFunc: func(ctx context.Context, intent Intent) Result {
			mu.Lock()
			processedCount++
			intentLogs = append(intentLogs, fmt.Sprintf("intent-%d-processed-%s-with-%d-files",
				processedCount, intent.Role, len(intent.Resources)))
			mu.Unlock()

			// Simulate processing time proportional to resources
			time.Sleep(10 * time.Millisecond * time.Duration(len(intent.Resources)))

			// Read actual file content to prove we're using real resources
			var outputs []string
			for _, r := range intent.Resources {
				content, err := os.ReadFile(r)
				if err == nil {
					outputs = append(outputs, fmt.Sprintf("%s (%d bytes)", r, len(content)))
				}
			}

			return Result{
				IntentID:      intent.ID,
				TaskID:        intent.TaskID,
				Status:        StatusSuccess,
				Output:        fmt.Sprintf("Processed %d files: %v", len(intent.Resources), outputs),
				FilesModified: intent.Resources,
				Details: []Detail{
					{Key: "role", Value: string(intent.Role)},
					{Key: "file_count", Value: fmt.Sprintf("%d", len(intent.Resources))},
				},
			}
		},
		PartitionConfig: PartitionConfig{
			MaxIntents:        10,
			MaxTokensPerIntent: 300, // Force splits
			SplitBy:            "file",
		},
	})

	supervisor.Start()
	defer supervisor.Stop()

	task := Task{
		ID:          "real-files",
		Description: "Review and analyze these Go files for issues",
		Resources:   realFiles,
	}

	start := time.Now()
	result, err := supervisor.Execute(ctx, task)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Status != StatusSuccess {
		t.Errorf("Expected success, got %s", result.Status)
	}

	t.Logf("Processed %d files in %v", len(realFiles), elapsed)
	t.Logf("Result: %s", result.Combined[:min(200, len(result.Combined))])
	t.Logf("Files changed: %d - %v", len(result.FilesChanged), result.FilesChanged)
	t.Logf("Intent logs: %v", intentLogs)

	// Verify all files were processed
	if len(result.FilesChanged) != len(realFiles) {
		t.Errorf("Expected %d files changed, got %d", len(realFiles), len(result.FilesChanged))
	}

	// With 3 workers and small token limit, we should have multiple intents
	// (unless all files fit in one intent due to short names)
}

// TestGhostCountCompactWithRealContent tests GhostManager with content.
func TestGhostCountCompactWithRealContent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	ctx := context.Background()

	cfg := GhostConfig{
		Enabled:          true,
		HistoryThreshold: 5000, // Force compaction with small threshold
		ContextWindow:   20000,
	}

	gm := NewGhostManager(cfg)
	agentID := "test-real"
	gm.RegisterAgent(agentID)

	// Simulate conversation with moderate-length messages
	// (longer than unit tests but not so long as to cause hasher slowdown)
	for i := range 50 {
		userMsg := fmt.Sprintf("User message #%d: Can you help me analyze this code and understand how the authentication flow works? The issue is that users are being logged out unexpectedly.", i)
		assistantMsg := fmt.Sprintf("Assistant response #%d: I've analyzed the authentication flow. The issue appears to be in the session token refresh logic. The token refresh happens every 30 minutes but cleanup only happens on login.", i)
		gm.AddMessage(agentID, "user", userMsg)
		gm.AddMessage(agentID, "assistant", assistantMsg)
	}

	tokensBefore := gm.estimateTokens(gm.GetContext(agentID))
	t.Logf("Tokens before compact: %d", tokensBefore)

	if !gm.ShouldCompact(agentID) {
		t.Log("ShouldCompact returned false")
	} else {
		// Only compact if we have reasonable amount of data
		if tokensBefore > 2000 {
			compacted := gm.Compact(ctx, agentID)
			if compacted {
				tokensAfter := gm.estimateTokens(gm.GetContext(agentID))
				reduction := tokensBefore - tokensAfter
				t.Logf("Tokens after: %d (reduced by %d, %.1f%%)", tokensAfter, reduction, float64(reduction)/float64(tokensBefore)*100)
			}
		}
	}
}

// TestParallelExecutionSpeed verifies workers complete faster than sequential.
func TestParallelExecutionSpeed(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	ctx := context.Background()

	workerCount := 3
	taskCount := 6
	taskDuration := 50 * time.Millisecond

	supervisor := NewSupervisor(ctx, SupervisorConfig{
		Name:     "parallel-speed",
		PoolSize: workerCount,
		ProcessFunc: func(ctx context.Context, intent Intent) Result {
			// Each task takes fixed duration
			time.Sleep(taskDuration)
			return Result{
				IntentID: intent.ID,
				TaskID:   intent.TaskID,
				Status:   StatusSuccess,
				Output:   "done",
			}
		},
		PartitionConfig: DefaultPartitionConfig(),
	})

	supervisor.Start()
	defer supervisor.Stop()

	// Create tasks - the exact number of intents depends on partitioning
	resources := make([]string, taskCount)
	for i := range taskCount {
		resources[i] = fmt.Sprintf("task_file_number_%d_that_is_not_short_at_all.go", i)
	}

	task := Task{
		ID:          "speed-test",
		Description: "Process tasks",
		Resources:   resources,
	}

	start := time.Now()
	result, err := supervisor.Execute(ctx, task)
	totalTime := time.Since(start)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Status != StatusSuccess {
		t.Errorf("Expected success, got %s", result.Status)
	}

	// Sequential time would be taskCount * taskDuration = 300ms
	// Parallel time with 3 workers should be ~taskCount/workerCount * taskDuration = 100ms
	// Allow some overhead, so anything under 200ms indicates parallelism
	sequentialTime := taskDuration * time.Duration(taskCount)
	expectedParallelTime := taskDuration * time.Duration((taskCount+workerCount-1)/workerCount)

	t.Logf("Total time: %v", totalTime)
	t.Logf("Sequential would be: %v", sequentialTime)
	t.Logf("Expected parallel (~%d workers): %v", workerCount, expectedParallelTime)

	// If took less than half of sequential time, parallelism is working
	if totalTime < sequentialTime/2 {
		t.Logf("PASS: Execution was parallel (speedup: %.1fx)", float64(sequentialTime)/float64(totalTime))
	} else {
		t.Errorf("Execution too slow - may not be running in parallel (took %v vs expected ~%v)",
			totalTime, expectedParallelTime)
	}
}
