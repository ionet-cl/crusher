// Package agent provides debug utilities for AI debugging with full transparency.
package agent

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/agent/circuit"
	"github.com/charmbracelet/crush/internal/message"
)

// DebugConfig holds configuration for debug output.
type DebugConfig struct {
	ShowAuditTrail     bool
	ShowCircuitBreaker bool
	ShowGhostCompact   bool
	ShowTokenUsage     bool
	ShowToolCalls      bool
	ShowMessages       bool
	ShowRecovery       bool
	ShowAllState       bool
}

// DefaultDebugConfig returns a config with all debug features enabled.
func DefaultDebugConfig() DebugConfig {
	return DebugConfig{
		ShowAuditTrail:     true,
		ShowCircuitBreaker: true,
		ShowGhostCompact:   true,
		ShowTokenUsage:     true,
		ShowToolCalls:      true,
		ShowMessages:       true,
		ShowRecovery:       true,
		ShowAllState:       true,
	}
}

// AIDebugger provides structured debug output for AI operations.
type AIDebugger struct {
	config DebugConfig
	output *strings.Builder
}

// NewAIDebugger creates a new AI debugger.
func NewAIDebugger(config DebugConfig) *AIDebugger {
	return &AIDebugger{
		config: config,
		output: &strings.Builder{},
	}
}

// Header prints a section header.
func (d *AIDebugger) Header(section string) {
	d.output.WriteString(fmt.Sprintf("\n%s %s %s\n", "═══", section, strings.Repeat("═", max(0, 60-len(section)-4))))
}

// SubHeader prints a subsection header.
func (d *AIDebugger) SubHeader(section string) {
	d.output.WriteString(fmt.Sprintf("\n─── %s ───\n", section))
}

// KV prints a key-value pair.
func (d *AIDebugger) KV(key string, value interface{}) {
	d.output.WriteString(fmt.Sprintf("  %-30s %v\n", key+":", value))
}

// KVColored prints a key-value pair with color (for terminal).
func (d *AIDebugger) KVColored(key string, value interface{}) {
	d.output.WriteString(fmt.Sprintf("  \033[36m%-30s\033[0m %v\n", key+":", value))
}

// Separator prints a separator line.
func (d *AIDebugger) Separator() {
	d.output.WriteString("  " + strings.Repeat("─", 58) + "\n")
}

// PrintAuditEntry prints a recovery audit entry.
func (d *AIDebugger) PrintAuditEntry(entry AuditEntry) {
	if !d.config.ShowAuditTrail {
		return
	}
	d.SubHeader("RECOVERY AUDIT ENTRY")
	d.KV("SessionID", entry.SessionID)
	d.KV("Timestamp", entry.Timestamp.Format(time.RFC3339Nano))
	d.KV("Action", entry.Action)
	d.KV("Strategy", StrategyName(entry.Strategy))
	d.KV("Error", entry.Error)
	d.KV("Success", entry.Success)
	d.KV("Details", entry.Details)
}

// PrintAllAuditTrail prints the entire audit trail.
func (d *AIDebugger) PrintAllAuditTrail(entries []AuditEntry) {
	if !d.config.ShowAuditTrail {
		return
	}
	d.Header("RECOVERY AUDIT TRAIL")
	if len(entries) == 0 {
		d.output.WriteString("  (no audit entries)\n")
		return
	}
	for i, entry := range entries {
		d.output.WriteString(fmt.Sprintf("\n  [%d] %s\n", i+1, entry.Timestamp.Format(time.RFC3339Nano)))
		d.KV("  Action", entry.Action)
		d.KV("  Strategy", StrategyName(entry.Strategy))
		d.KV("  Error", truncateString(entry.Error, 80))
		d.KV("  Success", entry.Success)
		d.KV("  Details", truncateString(entry.Details, 80))
	}
}

// PrintCircuitBreakerState prints the current circuit breaker state.
func (d *AIDebugger) PrintCircuitBreakerState(sessionID string, errInfo circuit.ErrInfo, recoverable bool) {
	if !d.config.ShowCircuitBreaker {
		return
	}
	d.Header("CIRCUIT BREAKER STATE")
	d.KV("SessionID", sessionID)
	d.KV("ErrorDetected", errInfo.ErrMsg)
	d.KV("Recoverable", recoverable)
	d.KV("Strategy", StrategyName(errInfo.Strategy))
	d.KV("Delay", errInfo.Delay)
}

// PrintGhostCompactOp prints a ghost compact operation.
func (d *AIDebugger) PrintGhostCompactOp(sessionID string, beforeTokens, afterTokens int, err error) {
	if !d.config.ShowGhostCompact {
		return
	}
	d.Header("GHOST COMPACT OPERATION")
	d.KV("SessionID", sessionID)
	d.KV("BeforeTokens", beforeTokens)
	d.KV("AfterTokens", afterTokens)
	d.KV("TokensRemoved", beforeTokens-afterTokens)
	d.KV("Reduction%", fmt.Sprintf("%.1f%%", float64(beforeTokens-afterTokens)/float64(beforeTokens)*100))
	if err != nil {
		d.KV("Error", err.Error())
	} else {
		d.KV("Status", "SUCCESS")
	}
}

// PrintProviderError prints a provider error in detail.
func (d *AIDebugger) PrintProviderError(sessionID string, err *ProviderErrorWrapper) {
	if err == nil {
		return
	}
	d.Header("PROVIDER ERROR")
	d.KV("SessionID", sessionID)
	d.KV("Title", err.Title)
	d.KV("Message", err.Message)
	d.KV("StatusCode", err.StatusCode)
	d.KV("ContextTooLarge", err.ContextTooLarge)
	d.KV("Retryable", err.Retryable)
	d.KV("ContextUsedTokens", err.ContextUsedTokens)
	d.KV("ContextMaxTokens", err.ContextMaxTokens)
}

// ProviderErrorWrapper wraps fantasy.ProviderError for debug output.
type ProviderErrorWrapper struct {
	Title             string
	Message           string
	StatusCode        int
	ContextTooLarge   bool
	Retryable         bool
	ContextUsedTokens int
	ContextMaxTokens  int
}

// PrintTokenUsage prints token usage information.
func (d *AIDebugger) PrintTokenUsage(sessionID string, promptTokens, completionTokens, totalTokens, contextWindow int) {
	if !d.config.ShowTokenUsage {
		return
	}
	d.Header("TOKEN USAGE")
	d.KV("SessionID", sessionID)
	d.KV("PromptTokens", promptTokens)
	d.KV("CompletionTokens", completionTokens)
	d.KV("TotalTokens", totalTokens)
	d.KV("ContextWindow", contextWindow)
	d.KV("UsagePercent", fmt.Sprintf("%.1f%%", float64(totalTokens)/float64(contextWindow)*100))
	d.KV("Remaining", contextWindow-totalTokens)
}

// PrintMessage prints a message in the debug output.
func (d *AIDebugger) PrintMessage(msg message.Message) {
	if !d.config.ShowMessages {
		return
	}
	d.SubHeader(fmt.Sprintf("MESSAGE [%s]", msg.Role))
	d.KV("ID", msg.ID)
	d.KV("Role", msg.Role)
	d.KV("Model", msg.Model)
	d.KV("Provider", msg.Provider)
	content := msg.Content().Text
	d.KV("Content", truncateString(content, 200))
}

// PrintToolCall prints a tool call.
func (d *AIDebugger) PrintToolCall(name string, input map[string]interface{}, output string, err error) {
	if !d.config.ShowToolCalls {
		return
	}
	d.SubHeader(fmt.Sprintf("TOOL CALL: %s", name))
	inputJSON, _ := json.MarshalIndent(input, "  ", "  ")
	d.output.WriteString(fmt.Sprintf("  Input:\n%s\n", indentString(string(inputJSON), "    ")))
	if err != nil {
		d.KV("Error", err.Error())
	} else {
		d.KV("Output", truncateString(output, 200))
	}
}

// PrintRecoveryAttempt prints a recovery attempt.
func (d *AIDebugger) PrintRecoveryAttempt(sessionID string, strategy circuit.Strategy, attempt int, maxRetries int) {
	if !d.config.ShowRecovery {
		return
	}
	d.Header(fmt.Sprintf("RECOVERY ATTEMPT #%d", attempt))
	d.KV("SessionID", sessionID)
	d.KV("Strategy", StrategyName(strategy))
	d.KV("Attempt", fmt.Sprintf("%d/%d", attempt, maxRetries))
}

// PrintStateChange prints a state change in the agent.
func (d *AIDebugger) PrintStateChange(sessionID, fromState, toState string) {
	if !d.config.ShowAllState {
		return
	}
	d.KVColored("STATE", fmt.Sprintf("%s → %s", fromState, toState))
}

// PrintAgentStart prints agent start information.
func (d *AIDebugger) PrintAgentStart(sessionID, prompt string, model, provider string) {
	d.Header("AGENT START")
	d.KV("SessionID", sessionID)
	d.KV("Model", model)
	d.KV("Provider", provider)
	d.KV("Prompt", truncateString(prompt, 300))
}

// PrintAgentEnd prints agent end information.
func (d *AIDebugger) PrintAgentEnd(sessionID string, duration time.Duration, success bool, err error) {
	d.Header("AGENT END")
	d.KV("SessionID", sessionID)
	d.KV("Duration", duration)
	d.KV("Success", success)
	if err != nil {
		d.KV("Error", err.Error())
	}
}

// PrintSummary prints a summary of all debug information.
func (d *AIDebugger) PrintSummary(entries []AuditEntry, totalRetries int) {
	d.Header("DEBUG SUMMARY")
	d.KV("TotalAuditEntries", len(entries))
	d.KV("TotalRecoveryAttempts", totalRetries)

	if len(entries) > 0 {
		successCount := 0
		for _, e := range entries {
			if e.Success {
				successCount++
			}
		}
		d.KV("SuccessfulRecoveries", successCount)
		d.KV("FailedRecoveries", len(entries)-successCount)
	}
}

// String returns the debug output as a string.
func (d *AIDebugger) String() string {
	return d.output.String()
}

// Reset clears the debug output.
func (d *AIDebugger) Reset() {
	d.output.Reset()
}

// Helper functions.

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func indentString(s, indent string) string {
	lines := strings.Split(s, "\n")
	result := ""
	for _, line := range lines {
		result += indent + line + "\n"
	}
	return result
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// LogAuditEntry logs an audit entry to slog for real-time debugging.
func LogAuditEntry(entry AuditEntry) {
	slog.Debug("audit_entry",
		"sid", entry.SessionID,
		"timestamp", entry.Timestamp.Format(time.RFC3339Nano),
		"action", entry.Action,
		"strategy", StrategyName(entry.Strategy),
		"error", entry.Error,
		"success", entry.Success,
		"details", entry.Details,
	)
}

// LogGhostCompact logs ghost compact operations in real-time.
func LogGhostCompact(sessionID string, beforeTokens, afterTokens int, err error) {
	if err != nil {
		slog.Info("ghost_compact",
			"sid", sessionID,
			"before_tokens", beforeTokens,
			"after_tokens", afterTokens,
			"tokens_removed", beforeTokens-afterTokens,
			"status", "error",
			"error", err.Error(),
		)
	} else {
		slog.Info("ghost_compact",
			"sid", sessionID,
			"before_tokens", beforeTokens,
			"after_tokens", afterTokens,
			"tokens_removed", beforeTokens-afterTokens,
			"reduction_pct", fmt.Sprintf("%.1f%%", float64(beforeTokens-afterTokens)/float64(beforeTokens)*100),
			"status", "success",
		)
	}
}

// LogCircuitBreakerDecision logs circuit breaker decisions in real-time.
func LogCircuitBreakerDecision(sessionID string, errInfo circuit.ErrInfo, canRecover bool, retryCount int) {
	slog.Info("circuit_breaker_decision",
		"sid", sessionID,
		"error_msg", errInfo.ErrMsg,
		"recoverable", canRecover,
		"strategy", StrategyName(errInfo.Strategy),
		"delay", errInfo.Delay,
		"retry_count", retryCount,
	)
}

// LogTokenUsage logs token usage in real-time.
func LogTokenUsage(sessionID string, promptTokens, completionTokens, totalTokens, contextWindow int) {
	slog.Debug("token_usage",
		"sid", sessionID,
		"prompt_tokens", promptTokens,
		"completion_tokens", completionTokens,
		"total_tokens", totalTokens,
		"context_window", contextWindow,
		"usage_pct", fmt.Sprintf("%.1f%%", float64(totalTokens)/float64(contextWindow)*100),
		"remaining", contextWindow-totalTokens,
	)
}

// GetCircuitBreakerRetryCount returns the current retry count for a session.
func GetCircuitBreakerRetryCount(sessionID string) int {
	return circuitBreakerRetryCount[sessionID]
}
