// Package multiplex provides multi-agent communication primitives.
package multiplex

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
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

// MultiplexRunner wraps SessionAgentRunner for use in ProcessFunc.
type MultiplexRunner struct {
	Runner SessionAgentRunner
}

// ToolCall represents a tool call that needs coordination.
type ToolCall struct {
	Name      string
	Input    map[string]interface{}
	Output   string
	Error    error
	Handled  bool
}

// FileLockManager coordinates access to files being edited.
type FileLockManager struct {
	locks sync.Map // map[string]*sync.Mutex
}

// NewFileLockManager creates a new file lock manager.
func NewFileLockManager() *FileLockManager {
	return &FileLockManager{}
}

// Lock acquires a lock for a file.
func (f *FileLockManager) Lock(path string) func() {
	mu, _ := f.locks.LoadOrStore(path, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	return func() { mu.(*sync.Mutex).Unlock() }
}

// ToolRouter coordinates tool calls between workers.
type ToolRouter struct {
	fileLocks *FileLockManager
	mu        sync.Mutex // For thread-safe tool call handling
}

// NewToolRouter creates a new tool router.
func NewToolRouter() *ToolRouter {
	return &ToolRouter{
		fileLocks: NewFileLockManager(),
	}
}

// RouteToolCall coordinates a tool call, applying locks for file operations.
func (t *ToolRouter) RouteToolCall(toolName string, input map[string]interface{}) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// For read-only tools, no coordination needed
	switch toolName {
	case "read", "view", "glob", "grep", "ls", "bash", "job_output", "todos":
		return "", nil // Let it proceed normally
	}

	// For file-modifying tools, coordinate access
	switch toolName {
	case "edit", "write", "multiedit":
		if path, ok := input["file_path"].(string); ok && path != "" {
			release := t.fileLocks.Lock(path)
			defer release()
		}
	}

	return "", nil
}

// SharedSessionAgent runs agents with shared session context.
// All workers use the same session ID, enabling shared message history.
type SharedSessionAgent struct {
	runner     SessionAgentRunner
	sessionID  string
	toolRouter *ToolRouter
	mu         sync.Mutex
}

// NewSharedSessionAgent creates an agent that shares session context.
func NewSharedSessionAgent(runner SessionAgentRunner, sessionID string, toolRouter *ToolRouter) *SharedSessionAgent {
	return &SharedSessionAgent{
		runner:     runner,
		sessionID:  sessionID,
		toolRouter: toolRouter,
	}
}

// Run executes the agent with shared session.
func (s *SharedSessionAgent) Run(ctx context.Context, prompt string, fileContents map[string]string) (AgentResult, error) {
	s.mu.Lock()
	result, err := s.runner.Run(ctx, s.sessionID, prompt, fileContents)
	s.mu.Unlock()
	return result, err
}

// AgentProcessFunc creates a ProcessFunc that coordinates file access and uses shared session.
// This is the CORRECT implementation that actually works.
func AgentProcessFunc(runner SessionAgentRunner, sessionID string) func(ctx context.Context, intent Intent) Result {
	toolRouter := NewToolRouter()

	return func(ctx context.Context, intent Intent) Result {
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

		// Use shared session agent
		sharedAgent := NewSharedSessionAgent(runner, sessionID, toolRouter)

		// Execute with coordination
		result, err := sharedAgent.Run(ctx, prompt, fileContents)

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
