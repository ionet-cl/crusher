package multiplex

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// BenchmarkParallelSpeedup measures the speedup from parallel vs sequential execution.
// This benchmarks the core claim that auto-multiplex provides significant speedup.
func BenchmarkParallelSpeedup(b *testing.B) {
	testCases := []struct {
		name        string
		poolSize    int
		taskCount   int
		taskTimeout time.Duration
	}{
		{"2_workers_4_tasks", 2, 4, 50 * time.Millisecond},
		{"3_workers_6_tasks", 3, 6, 50 * time.Millisecond},
		{"4_workers_8_tasks", 4, 8, 50 * time.Millisecond},
		{"6_workers_12_tasks", 6, 12, 50 * time.Millisecond},
		{"3_workers_18_tasks", 3, 18, 20 * time.Millisecond},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// Measure parallel execution
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ctx := context.Background()
				supervisor := NewSupervisor(ctx, SupervisorConfig{
					Name:     "benchmark",
					PoolSize: tc.poolSize,
					ProcessFunc: func(ctx context.Context, intent Intent) Result {
						time.Sleep(tc.taskTimeout)
						return Result{
							IntentID: intent.ID,
							TaskID:   intent.TaskID,
							Status:   StatusSuccess,
							Output:   "done",
						}
					},
					PartitionConfig: PartitionConfig{
						MaxIntents:        tc.taskCount,
						MaxTokensPerIntent: 100,
						SplitBy:           "file",
					},
				})

				resources := make([]string, tc.taskCount)
				for j := range tc.taskCount {
					resources[j] = fmt.Sprintf("task_%d.go", j)
				}

				supervisor.Start()
				_, _ = supervisor.Execute(ctx, Task{
					ID:          "benchmark",
					Description: "Benchmark task",
					Resources:   resources,
				})
				supervisor.Stop()
			}

			// Calculate theoretical speedup
			sequentialTime := tc.taskTimeout * time.Duration(tc.taskCount)
			idealParallelTime := tc.taskTimeout * time.Duration((tc.taskCount+tc.poolSize-1)/tc.poolSize)
			maxSpeedup := float64(sequentialTime) / float64(idealParallelTime)

			b.ReportMetric(float64(tc.poolSize), "pool_size")
			b.ReportMetric(float64(tc.taskCount), "task_count")
			b.ReportMetric(float64(tc.taskTimeout.Milliseconds()), "task_timeout_ms")
			b.ReportMetric(maxSpeedup, "ideal_speedup")
		})
	}
}

// BenchmarkSequentialBaseline measures sequential execution time for comparison.
func BenchmarkSequentialBaseline(b *testing.B) {
	taskTimeouts := []time.Duration{10 * time.Millisecond, 50 * time.Millisecond, 100 * time.Millisecond}
	taskCounts := []int{4, 8, 12}

	for _, timeout := range taskTimeouts {
		for _, count := range taskCounts {
			b.Run(fmt.Sprintf("%d_tasks_%dms", count, timeout.Milliseconds()), func(b *testing.B) {
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					ctx := context.Background()
					// Sequential execution - pool size of 1
					supervisor := NewSupervisor(ctx, SupervisorConfig{
						Name:     "sequential",
						PoolSize: 1, // Sequential
						ProcessFunc: func(ctx context.Context, intent Intent) Result {
							time.Sleep(timeout)
							return Result{
								IntentID: intent.ID,
								TaskID:   intent.TaskID,
								Status:   StatusSuccess,
								Output:   "done",
							}
						},
						PartitionConfig: PartitionConfig{
							MaxIntents:        count,
							MaxTokensPerIntent: 100,
							SplitBy:           "file",
						},
					})

					resources := make([]string, count)
					for j := range count {
						resources[j] = fmt.Sprintf("task_%d.go", j)
					}

					supervisor.Start()
					_, _ = supervisor.Execute(ctx, Task{
						ID:          "sequential",
						Description: "Sequential benchmark",
						Resources:   resources,
					})
					supervisor.Stop()
				}

				sequentialTime := timeout * time.Duration(count)
				b.ReportMetric(float64(count), "task_count")
				b.ReportMetric(float64(timeout.Milliseconds()), "task_timeout_ms")
				b.ReportMetric(float64(sequentialTime.Milliseconds()), "total_time_ms")
			})
		}
	}
}

// BenchmarkThroughput measures tasks completed per second.
func BenchmarkThroughput(b *testing.B) {
	poolSizes := []int{1, 2, 3, 4, 6}
	taskCount := 24
	taskTimeout := 25 * time.Millisecond

	b.ResetTimer()
	b.ReportAllocs()

	for _, poolSize := range poolSizes {
		b.Run(fmt.Sprintf("%d_workers", poolSize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				ctx := context.Background()
				supervisor := NewSupervisor(ctx, SupervisorConfig{
					Name:     "throughput",
					PoolSize: poolSize,
					ProcessFunc: func(ctx context.Context, intent Intent) Result {
						time.Sleep(taskTimeout)
						return Result{
							IntentID: intent.ID,
							TaskID:   intent.TaskID,
							Status:   StatusSuccess,
							Output:   "done",
						}
					},
					PartitionConfig: PartitionConfig{
						MaxIntents:        taskCount,
						MaxTokensPerIntent: 100,
						SplitBy:           "file",
					},
				})

				resources := make([]string, taskCount)
				for j := range taskCount {
					resources[j] = fmt.Sprintf("task_%d.go", j)
				}

				supervisor.Start()
				start := time.Now()
				_, _ = supervisor.Execute(ctx, Task{
					ID:          "throughput",
					Description: "Throughput benchmark",
					Resources:   resources,
				})
				elapsed := time.Since(start)
				supervisor.Stop()

				// Throughput = tasks / second
				throughput := float64(taskCount) / elapsed.Seconds()
				b.ReportMetric(throughput, "tasks_per_second")
				b.ReportMetric(float64(poolSize), "workers")
			}
		})
	}
}

// BenchmarkPartitioningPerformance measures how fast task partitioning works.
func BenchmarkPartitioningPerformance(b *testing.B) {
	testCases := []struct {
		name       string
		fileCount  int
		maxTokens  int
	}{
		{"small_10_files_500_tokens", 10, 500},
		{"medium_50_files_1000_tokens", 50, 1000},
		{"large_200_files_2000_tokens", 200, 2000},
		{"xlarge_500_files_5000_tokens", 500, 5000},
	}

	partitioner := NewTaskPartitioner()

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			resources := make([]string, tc.fileCount)
			for i := range tc.fileCount {
				resources[i] = fmt.Sprintf("internal/module/submodule/file%d.go", i)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				partitioner.Partition(
					"benchmark-task",
					"Analyze and modify these files",
					resources,
					PartitionConfig{
						MaxIntents:        100,
						MaxTokensPerIntent: tc.maxTokens,
						SplitBy:           "module",
					},
				)
			}

			b.ReportMetric(float64(tc.fileCount), "file_count")
			b.ReportMetric(float64(tc.maxTokens), "max_tokens_per_intent")
		})
	}
}

// BenchmarkGhostCountIntegration measures GhostManager performance in multiplex context.
func BenchmarkGhostCountIntegration(b *testing.B) {
	cfg := GhostConfig{
		Enabled:          true,
		HistoryThreshold: 5000,
		ContextWindow:    20000,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		gm := NewGhostManager(cfg)
		agentID := fmt.Sprintf("bench-agent-%d", i)

		// Simulate conversation
		for j := 0; j < 100; j++ {
			gm.AddMessage(agentID, "user", fmt.Sprintf("User message %d with some content for context", j))
			gm.AddMessage(agentID, "assistant", fmt.Sprintf("Assistant response %d with detailed explanation and code snippets", j))
		}

		// Measure compaction
		if gm.ShouldCompact(agentID) {
			_ = gm.Compact(context.Background(), agentID)
		}

		gm.UnregisterAgent(agentID)
	}
}
