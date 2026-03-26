// Package multiplex provides multi-agent communication primitives.
package multiplex

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/crush/internal/ghostcount"
)

// TaskPartitioner divides a large task into smaller intents for parallel execution.
type TaskPartitioner struct {
	estimator ghostcount.TokenEstimator
}

// NewTaskPartitioner creates a partitioner.
func NewTaskPartitioner() *TaskPartitioner {
	return &TaskPartitioner{
		estimator: ghostcount.NewEstimator(),
	}
}

// PartitionConfig controls how tasks are partitioned.
type PartitionConfig struct {
	// MaxIntents is the maximum number of intents to create.
	MaxIntents int
	// MaxTokensPerIntent is the target tokens per intent.
	MaxTokensPerIntent int
	// SplitBy indicates how to split: "file", "module", "region"
	SplitBy string
}

// DefaultPartitionConfig returns sensible defaults.
func DefaultPartitionConfig() PartitionConfig {
	return PartitionConfig{
		MaxIntents:        10,
		MaxTokensPerIntent: 50000, // ~200KB of text
		SplitBy:            "file",
	}
}

// PartitionResult contains the result of partitioning.
type PartitionResult struct {
	Intents   []Intent
	Strategy  string
	TotalTokens int
}

// Partition takes a task description and resources, and creates intents.
// It analyzes the content and divides based on SplitBy strategy.
func (tp *TaskPartitioner) Partition(
	taskID string,
	taskDescription string,
	resources []string,
	config PartitionConfig,
) (*PartitionResult, error) {
	if len(resources) == 0 {
		// No resources to partition - single intent
		return &PartitionResult{
			Intents: []Intent{
				{
					ID:     fmt.Sprintf("%s-1", taskID),
					TaskID: taskID,
					Role:   RoleAnalyzer,
					Goal:   taskDescription,
				},
			},
			Strategy: "single",
		}, nil
	}

	switch config.SplitBy {
	case "file":
		return tp.partitionByFile(taskID, taskDescription, resources, config)
	case "module":
		return tp.partitionByModule(taskID, taskDescription, resources, config)
	default:
		return tp.partitionByFile(taskID, taskDescription, resources, config)
	}
}

// partitionByFile creates one intent per file (or group of small files).
func (tp *TaskPartitioner) partitionByFile(
	taskID string,
	taskDescription string,
	resources []string,
	config PartitionConfig,
) (*PartitionResult, error) {
	var intents []Intent
	var currentTokens int
	var currentGroup []string

	estimateTokens := func(resources []string) int {
		// Rough estimate: ~4 chars per token
		total := 0
		for _, r := range resources {
			total += len(r)
		}
		return total / 4
	}

	addIntent := func() {
		if len(currentGroup) == 0 {
			return
		}
		role := tp.deriveRole(taskDescription)
		intent := Intent{
			ID:        fmt.Sprintf("%s-%d", taskID, len(intents)+1),
			TaskID:    taskID,
			Role:      role,
			Goal:      taskDescription,
			Resources: currentGroup,
			Priority:  len(intents), // Earlier = higher priority
		}
		intents = append(intents, intent)
		currentGroup = nil
		currentTokens = 0
	}

	for i, resource := range resources {
		resourceTokens := len(resource) / 4

		// If single resource is too large, split it
		if resourceTokens > config.MaxTokensPerIntent {
			// Finish current group
			addIntent()

			// Create intent for this large resource alone
			role := tp.deriveRole(taskDescription)
			intents = append(intents, Intent{
				ID:        fmt.Sprintf("%s-%d", taskID, len(intents)+1),
				TaskID:    taskID,
				Role:      role,
				Goal:      taskDescription,
				Resources: []string{resource},
				Priority:  i, // Maintain original order
			})
			continue
		}

		// Check if adding this resource would exceed limit
		if currentTokens+resourceTokens > config.MaxTokensPerIntent && len(currentGroup) > 0 {
			addIntent()
		}

		currentGroup = append(currentGroup, resource)
		currentTokens += resourceTokens

		// Respect max intents
		if len(intents)+len(currentGroup) > config.MaxIntents {
			addIntent()
		}
	}

	addIntent() // Flush remaining

	return &PartitionResult{
		Intents:      intents,
		Strategy:     "file",
		TotalTokens:  estimateTokens(resources),
	}, nil
}

// partitionByModule groups resources by module/directory.
func (tp *TaskPartitioner) partitionByModule(
	taskID string,
	taskDescription string,
	resources []string,
	config PartitionConfig,
) (*PartitionResult, error) {
	// Group resources by module (directory path)
	moduleMap := make(map[string][]string)
	for _, resource := range resources {
		parts := strings.Split(resource, "/")
		module := "root"
		if len(parts) > 1 {
			// Find the first non-empty, non-".." part
			for i, p := range parts {
				if p != "" && p != ".." {
					module = p
					break
				}
				// If we hit "..", use the next real directory
				if i > 0 && p == ".." && i+1 < len(parts) && parts[i+1] != "" && parts[i+1] != ".." {
					module = parts[i+1]
					break
				}
			}
		}
		moduleMap[module] = append(moduleMap[module], resource)
	}

	var intents []Intent
	priority := 0
	for module, resources := range moduleMap {
		role := tp.deriveRole(taskDescription)
		intent := Intent{
			ID:        fmt.Sprintf("%s-%d", taskID, len(intents)+1),
			TaskID:    taskID,
			Role:      role,
			Goal:      fmt.Sprintf("[%s] %s", module, taskDescription),
			Resources: resources,
			Priority:  priority,
		}
		intents = append(intents, intent)
		priority++
	}

	return &PartitionResult{
		Intents:  intents,
		Strategy: "module",
	}, nil
}

// deriveRole infers the agent role from task description.
func (tp *TaskPartitioner) deriveRole(task string) Role {
	lower := strings.ToLower(task)

	// Keywords that suggest specific roles
	if strings.Contains(lower, "refactor") ||
		strings.Contains(lower, "change") ||
		strings.Contains(lower, "modify") ||
		strings.Contains(lower, "update") {
		return RoleEditor
	}
	if strings.Contains(lower, "review") ||
		strings.Contains(lower, "check") ||
		strings.Contains(lower, "validate") ||
		strings.Contains(lower, "test") {
		return RoleReviewer
	}
	if strings.Contains(lower, "fetch") ||
		strings.Contains(lower, "get") ||
		strings.Contains(lower, "download") ||
		strings.Contains(lower, "retrieve") {
		return RoleFetcher
	}

	// Default to analyzer
	return RoleAnalyzer
}
