// Package multiplex provides multi-agent communication primitives.
package multiplex

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// SessionAgentRunner is an interface for running agent sessions.
// This allows multiplex to work with any agent implementation without import cycles.
type SessionAgentRunner interface {
	Run(ctx context.Context, sessionID string, prompt string, fileContents map[string]string) (AgentResult, error)
}

// AgentResult represents the result of an agent run.
type AgentResult struct {
	Text string
}

// AgentProcessFunc creates a ProcessFunc that uses a real SessionAgent.
// This enables transparent multiplexing with the actual Crusher agent.
func AgentProcessFunc(runner SessionAgentRunner) func(ctx context.Context, intent Intent) Result {
	return func(ctx context.Context, intent Intent) Result {
		// Create a unique session ID for this intent
		sessionID := fmt.Sprintf("multiplex-%s-%s", intent.TaskID, intent.ID)

		// Pre-read file contents so the agent has actual code to work with
		fileContents := make(map[string]string)
		for _, path := range intent.Resources {
			content, err := os.ReadFile(path)
			if err != nil {
				fileContents[path] = fmt.Sprintf("[Could not read file: %v]", err)
				continue
			}
			// Truncate very large files to avoid token limits
			if len(content) > 50000 {
				content = content[:50000]
				fileContents[path] = string(content) + "\n[... truncated due to size ...]"
			} else {
				fileContents[path] = string(content)
			}
		}

		// Build a prompt that includes the task AND the file contents
		prompt := buildPromptWithContents(intent.Goal, fileContents)

		// Use the agent to process this intent's goal with file contents
		result, err := runner.Run(ctx, sessionID, prompt, fileContents)

		if err != nil {
			return Result{
				IntentID: intent.ID,
				TaskID:   intent.TaskID,
				Status:   StatusFailed,
				Output:   fmt.Sprintf("Error: %v", err),
			}
		}

		return Result{
			IntentID:      intent.ID,
			TaskID:        intent.TaskID,
			Status:        StatusSuccess,
			Output:        result.Text,
			FilesModified: nil,
		}
	}
}

// buildPromptWithContents creates a prompt that includes file contents.
func buildPromptWithContents(goal string, fileContents map[string]string) string {
	if len(fileContents) == 0 {
		return goal
	}

	var b strings.Builder
	b.WriteString(goal)
	b.WriteString("\n\n=== FILES TO ANALYZE ===\n\n")

	for path, content := range fileContents {
		fmt.Fprintf(&b, "--- %s ---\n%s\n\n", path, content)
	}

	b.WriteString("=== END FILES ===\n")
	return b.String()
}
