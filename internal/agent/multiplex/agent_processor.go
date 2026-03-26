// Package multiplex provides multi-agent communication primitives.
package multiplex

import (
	"context"
	"fmt"
)

// SessionAgentRunner is an interface for running agent sessions.
// This allows multiplex to work with any agent implementation without import cycles.
type SessionAgentRunner interface {
	Run(ctx context.Context, sessionID string, prompt string) (AgentResult, error)
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

		// Use the agent to process this intent's goal
		result, err := runner.Run(ctx, sessionID, intent.Goal)

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
			FilesModified: nil, // Agent doesn't track modified files in this integration
		}
	}
}
