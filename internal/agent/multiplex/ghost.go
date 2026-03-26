// Package multiplex provides multi-agent communication primitives.
package multiplex

import (
	"context"
	"log/slog"

	"github.com/charmbracelet/crush/internal/ghostcount"
)

// GhostConfig configures GhostCount for multi-agent use.
type GhostConfig struct {
	// Enabled turns GhostCount on/off.
	Enabled bool
	// HistoryThreshold is the token budget for conversation history.
	HistoryThreshold int
	// ContextWindow is the model's context window size.
	ContextWindow int
}

// DefaultGhostConfig returns sensible defaults.
func DefaultGhostConfig() GhostConfig {
	return GhostConfig{
		Enabled:          true,
		HistoryThreshold: 100000, // 100K tokens
		ContextWindow:   200000, // 200K tokens
	}
}

// GhostManager manages context compression across multiple agents.
// Each agent has local GhostCount, and the manager coordinates compression.
type GhostManager struct {
	config    GhostConfig
	estimator ghostcount.TokenEstimator
	compactor ghostcount.MessageCompactor
	truncator ghostcount.ContextTruncator

	// Per-agent message stores
	agentMessages map[string][]ghostcount.Message
}

// NewGhostManager creates a GhostManager.
func NewGhostManager(config GhostConfig) *GhostManager {
	return &GhostManager{
		config:    config,
		estimator: ghostcount.NewEstimator(),
		compactor: ghostcount.NewCompactor(),
		truncator: ghostcount.NewTruncator(),
		agentMessages: make(map[string][]ghostcount.Message),
	}
}

// RegisterAgent registers an agent with the manager.
func (gm *GhostManager) RegisterAgent(agentID string) {
	gm.agentMessages[agentID] = make([]ghostcount.Message, 0)
}

// UnregisterAgent removes an agent from the manager.
func (gm *GhostManager) UnregisterAgent(agentID string) {
	delete(gm.agentMessages, agentID)
}

// AddMessage adds a message to an agent's context.
func (gm *GhostManager) AddMessage(agentID string, role, content string) {
	if !gm.config.Enabled {
		return
	}

	msg := &ghostMessage{
		role:    role,
		content: content,
	}
	gm.agentMessages[agentID] = append(gm.agentMessages[agentID], msg)
}

// ShouldCompact checks if an agent's context needs compression.
func (gm *GhostManager) ShouldCompact(agentID string) bool {
	if !gm.config.Enabled {
		return false
	}

	msgs, ok := gm.agentMessages[agentID]
	if !ok || len(msgs) == 0 {
		return false
	}

	return gm.compactor.ShouldCompact(
		gm.estimateTokens(msgs),
		gm.config.HistoryThreshold,
	)
}

// Compact compresses an agent's context using GhostCount.
func (gm *GhostManager) Compact(ctx context.Context, agentID string) bool {
	if !gm.config.Enabled {
		return false
	}

	msgs, ok := gm.agentMessages[agentID]
	if !ok || len(msgs) == 0 {
		return false
	}

	cfg := ghostcount.NewCompactionConfig(
		gm.config.ContextWindow,
		4000, // 4K for response
	)
	cfg.HistoryThreshold = gm.config.HistoryThreshold

	result := gm.compactor.Compact(ctx, msgs, cfg, gm.estimator, gm.truncator)

	if result.WasCompacted {
		gm.agentMessages[agentID] = result.Messages
		slog.Debug("ghost: compacted agent context",
			"agent", agentID,
			"before", result.TokensBefore,
			"after", result.TokensAfter)
	}

	return result.WasCompacted
}

// GetContext returns the current messages for an agent.
func (gm *GhostManager) GetContext(agentID string) []ghostcount.Message {
	if !gm.config.Enabled {
		return nil
	}

	msgs, ok := gm.agentMessages[agentID]
	if !ok {
		return nil
	}

	return msgs
}

// TotalTokens returns total tokens across all agents.
func (gm *GhostManager) TotalTokens() int {
	total := 0
	for _, msgs := range gm.agentMessages {
		total += gm.estimateTokens(msgs)
	}
	return total
}

func (gm *GhostManager) estimateTokens(msgs []ghostcount.Message) int {
	texts := make([]string, len(msgs))
	for i, m := range msgs {
		texts[i] = m.GetContent()
	}
	return gm.estimator.EstimateMessages(texts)
}

// ghostMessage implements ghostcount.Message.
type ghostMessage struct {
	role    string
	content string
}

func (m *ghostMessage) GetRole() string {
	return m.role
}

func (m *ghostMessage) GetContent() string {
	return m.content
}
