package agent

import (
	"context"
	"fmt"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent/multiplex"
	"github.com/charmbracelet/crush/internal/message"
)

// multiplexRunner implements multiplex.SessionAgentRunner using the coordinator's agent.
type multiplexRunner struct {
	coordinator *coordinator
}

func (m *multiplexRunner) Run(ctx context.Context, sessionID string, prompt string, fileContents map[string]string) (multiplex.AgentResult, error) {
	// fileContents is already included in prompt by AgentProcessFunc, so we just pass the prompt
	result, err := m.coordinator.currentAgent.Run(ctx, SessionAgentCall{
		SessionID: sessionID,
		Prompt:    prompt,
	})
	if err != nil {
		return multiplex.AgentResult{}, err
	}

	text := ""
	if result != nil && result.Response.Content != nil {
		for _, content := range result.Response.Content {
			if tc, ok := content.(interface{ GetText() string }); ok {
				text += tc.GetText()
			}
		}
	}

	return multiplex.AgentResult{Text: text}, nil
}

// runWithMultiplex automatically runs multiplex if there's enough parallel work.
// Returns nil to fall back to normal flow if multiplex isn't beneficial.
func (c *coordinator) runWithMultiplex(
	ctx context.Context,
	sessionID string,
	prompt string,
	attachments []message.Attachment,
) (*fantasy.AgentResult, error) {
	// Scan the working directory for relevant files
	scanner := multiplex.NewRepoScanner()
	repo := multiplex.RepoSpec{
		ID:   "working",
		Root: c.cfg.WorkingDir(),
		Type: multiplex.RepoTypeLocal,
	}

	contents, err := scanner.Scan(ctx, repo)
	if err != nil || contents == nil || len(contents.Files) == 0 {
		return nil, nil // Fall back to normal flow
	}

	// Only use multiplex if we found enough work to parallelize
	if len(contents.Modules) < 2 && len(contents.Files) < 5 {
		return nil, nil // Not enough work to justify multiplex overhead
	}

	// Determine split strategy based on repo size
	splitBy := "module"
	if len(contents.Modules) > 10 {
		splitBy = "module"
	} else if len(contents.Files) > 50 {
		splitBy = "file"
	}

	// Create intents from repo contents
	intents := multiplex.IntentFromRepoContents(contents, sessionID, prompt, splitBy)
	if len(intents) < 2 {
		return nil, nil // Not enough intents to parallelize
	}

	// Create supervisor with real agent
	runner := &multiplexRunner{coordinator: c}
	supervisor := multiplex.NewSupervisor(ctx, multiplex.SupervisorConfig{
		Name:        "auto-multiplex",
		PoolSize:    4,
		ProcessFunc: multiplex.AgentProcessFunc(runner),
		PartitionConfig: multiplex.PartitionConfig{
			MaxIntents:         20,
			MaxTokensPerIntent: 40000,
			SplitBy:            splitBy,
		},
	})

	supervisor.Start()
	defer supervisor.Stop()

	// Execute task
	task := multiplex.Task{
		ID:          sessionID,
		Description: prompt,
		Resources:   contents.Files,
	}

	result, err := supervisor.Execute(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("multiplex execution failed: %w", err)
	}

	// Convert SupervisorResult to fantasy.AgentResult
	return &fantasy.AgentResult{
		Response: fantasy.Response{
			Content: fantasy.ResponseContent{
				fantasy.TextContent{Text: result.Combined},
			},
		},
	}, nil
}
