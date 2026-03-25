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

// ── ANSI Color Constants ──────────────────────────────────────────────

const (
	_RESET     = "\033[0m"
	_BOLD      = "\033[1m"
	_DIM       = "\033[2m"
	_ITALIC    = "\033[3m"
	_UNDERLINE = "\033[4m"

	_CYAN    = "\033[38;5;75m"
	_GREEN   = "\033[38;5;78m"
	_YELLOW  = "\033[38;5;220m"
	_RED     = "\033[38;5;196m"
	_MAGENTA = "\033[38;5;176m"
	_WHITE   = "\033[97m"
	_GRAY    = "\033[38;5;245m"
	_L_GRAY  = "\033[38;5;250m"
)

// Status icons for visual clarity
const (
	_ICON_SUCCESS = _GREEN + "✓" + _RESET
	_ICON_ERROR   = _RED + "✗" + _RESET
	_ICON_WARN    = _YELLOW + "⚠" + _RESET
	_ICON_INFO    = _CYAN + "●" + _RESET
	_ICON_THINK   = _MAGENTA + "◆" + _RESET
	_ICON_TOOL    = _YELLOW + "▶" + _RESET
	_ICON_TOKEN   = _CYAN + "◇" + _RESET
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

// Header prints a section header with brand styling.
func (d *AIDebugger) Header(section string) {
	d.output.WriteString(fmt.Sprintf("\n%s %s %s\n", _CYAN+"═══"+_RESET, _BOLD+section+_RESET, _DIM+strings.Repeat("═", max(0, 50-len(section)-4))+_RESET))
}

// SubHeader prints a subsection header.
func (d *AIDebugger) SubHeader(section string) {
	d.output.WriteString(fmt.Sprintf("\n%s %s %s\n", _MAGENTA+"───"+_RESET, _BOLD+section+_RESET, _DIM+"───"+_RESET))
}

// KV prints a key-value pair.
func (d *AIDebugger) KV(key string, value interface{}) {
	d.output.WriteString(fmt.Sprintf("  %-30s %v\n", _GRAY+key+":"+_RESET, value))
}

// KVSuccess prints a key-value pair with success styling.
func (d *AIDebugger) KVSuccess(key string, value interface{}) {
	d.output.WriteString(fmt.Sprintf("  %s %-28s %s\n", _ICON_SUCCESS, _GRAY+key+":"+_RESET, _GREEN+fmt.Sprintf("%v", value)+_RESET))
}

// KVError prints a key-value pair with error styling.
func (d *AIDebugger) KVError(key string, value interface{}) {
	d.output.WriteString(fmt.Sprintf("  %s %-28s %s\n", _ICON_ERROR, _GRAY+key+":"+_RESET, _RED+fmt.Sprintf("%v", value)+_RESET))
}

// KVWarn prints a key-value pair with warning styling.
func (d *AIDebugger) KVWarn(key string, value interface{}) {
	d.output.WriteString(fmt.Sprintf("  %s %-28s %s\n", _ICON_WARN, _GRAY+key+":"+_RESET, _YELLOW+fmt.Sprintf("%v", value)+_RESET))
}

// KVInfo prints a key-value pair with info styling.
func (d *AIDebugger) KVInfo(key string, value interface{}) {
	d.output.WriteString(fmt.Sprintf("  %s %-28s %s\n", _ICON_INFO, _GRAY+key+":"+_RESET, _CYAN+fmt.Sprintf("%v", value)+_RESET))
}

// KVColored prints a key-value pair with color (for terminal).
func (d *AIDebugger) KVColored(key string, value interface{}) {
	d.output.WriteString(fmt.Sprintf("  %s %-28s %v\n", _CYAN+key+":"+_RESET, "", value))
}

// Separator prints a separator line.
func (d *AIDebugger) Separator() {
	d.output.WriteString(_DIM + "  " + strings.Repeat("─", 58) + _RESET + "\n")
}

// PrintAuditEntry prints a recovery audit entry with full detail.
func (d *AIDebugger) PrintAuditEntry(entry AuditEntry) {
	if !d.config.ShowAuditTrail {
		return
	}
	d.SubHeader("RECOVERY AUDIT ENTRY")
	if entry.Success {
		d.KVSuccess("Success", "Recovery succeeded")
	} else {
		d.KVError("Success", "Recovery failed")
	}
	d.KV("SessionID", entry.SessionID)
	d.KV("Timestamp", entry.Timestamp.Format("15:04:05.000"))
	d.KV("Action", entry.Action)
	d.KV("Strategy", StrategyName(entry.Strategy))
	d.KV("Error", truncateString(entry.Error, 60))
	d.KV("Details", truncateString(entry.Details, 80))
}

// PrintAllAuditTrail prints the entire audit trail as a visual table.
func (d *AIDebugger) PrintAllAuditTrail(entries []AuditEntry) {
	if !d.config.ShowAuditTrail {
		return
	}
	d.Header("RECOVERY AUDIT TRAIL")
	if len(entries) == 0 {
		d.output.WriteString(_GRAY + "  (no audit entries)\n" + _RESET)
		return
	}

	// Table header
	d.output.WriteString(fmt.Sprintf("  %s  %-12s %-18s %-10s %s\n",
		_DIM+"#"+
			_RESET, _GRAY+"TIME"+_RESET, _GRAY+"ACTION"+_RESET, _GRAY+"STRATEGY"+_RESET, _GRAY+"STATUS"+_RESET))
	d.output.WriteString(_DIM + "  " + strings.Repeat("─", 65) + _RESET + "\n")

	for i, entry := range entries {
		statusIcon := _ICON_SUCCESS
		statusColor := _GREEN
		if !entry.Success {
			statusIcon = _ICON_ERROR
			statusColor = _RED
		}
		statusStr := fmt.Sprintf("%s %s", statusIcon, entry.Action)

		d.output.WriteString(fmt.Sprintf("  %s%2d%s  %-12s %-18s %-10s %s\n",
			_CYAN, i+1, _RESET,
			entry.Timestamp.Format("15:04:05"),
			truncateString(entry.Action, 17),
			truncateString(StrategyName(entry.Strategy), 10),
			statusColor+statusStr+_RESET))
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

// PrintGhostCompactOp prints a ghost compact operation with visual progress bar.
func (d *AIDebugger) PrintGhostCompactOp(sessionID string, beforeTokens, afterTokens int, err error) {
	if !d.config.ShowGhostCompact {
		return
	}
	d.Header("GHOST COMPACT OPERATION")
	d.KV("SessionID", sessionID)

	// Visual progress bar for token reduction
	removed := beforeTokens - afterTokens
	reductionPct := 0.0
	if beforeTokens > 0 {
		reductionPct = float64(removed) / float64(beforeTokens) * 100
	}

	// Before bar (green) -> After bar (cyan)
	barWidth := 30
	beforeBars := barWidth
	afterBars := 0
	if beforeTokens > 0 {
		afterBars = int(float64(barWidth) * float64(afterTokens) / float64(beforeTokens))
		beforeBars = barWidth - afterBars
	}

	beforeBar := _RED + strings.Repeat("█", beforeBars) + _RESET
	afterBar := _GREEN + strings.Repeat("█", afterBars) + _RESET
	d.output.WriteString(fmt.Sprintf("  %-20s %s%s %d\n", _GRAY+"Tokens:"+_RESET, beforeBar, afterBar, beforeTokens))
	d.output.WriteString(fmt.Sprintf("  %-20s %s%s %d (%s removed)\n", "", "", "", afterTokens, _RED+fmt.Sprintf("-%d", removed)+_RESET))
	d.output.WriteString(fmt.Sprintf("  %-20s %s%.1f%% reduction\n", _GRAY+"Reduction:"+_RESET, _GREEN, reductionPct))

	if err != nil {
		d.KVError("Error", err.Error())
	} else {
		d.KVSuccess("Status", "Compact successful")
	}
}

// PrintProviderError prints a provider error in detail.
func (d *AIDebugger) PrintProviderError(sessionID string, err *ProviderErrorWrapper) {
	if err == nil {
		return
	}
	d.Header("PROVIDER ERROR")
	d.KVError("Title", err.Title)
	d.KV("SessionID", sessionID)
	d.KV("Message", truncateString(err.Message, 80))
	d.KV("StatusCode", err.StatusCode)

	if err.ContextTooLarge {
		d.KVWarn("ContextTooLarge", "Yes - context window exceeded")
	}
	if err.Retryable {
		d.KVInfo("Retryable", "Yes - can retry")
	}
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

// PrintTokenUsage prints token usage with a visual progress bar.
func (d *AIDebugger) PrintTokenUsage(sessionID string, promptTokens, completionTokens, totalTokens, contextWindow int) {
	if !d.config.ShowTokenUsage {
		return
	}
	d.Header("TOKEN USAGE")

	// Visual progress bar
	barWidth := 40
	usagePct := 0.0
	if contextWindow > 0 {
		usagePct = float64(totalTokens) / float64(contextWindow) * 100
	}
	filledBars := int(float64(barWidth) * usagePct / 100)
	emptyBars := barWidth - filledBars

	// Color based on usage level
	barColor := _GREEN
	if usagePct > 75 {
		barColor = _YELLOW
	}
	if usagePct > 90 {
		barColor = _RED
	}

	bar := barColor + strings.Repeat("█", filledBars) + _DIM + strings.Repeat("░", emptyBars) + _RESET
	d.output.WriteString(fmt.Sprintf("  %s %s %s %.1f%%\n", _ICON_TOKEN, _GRAY+"Context:"+_RESET, bar, usagePct))

	// Token breakdown
	d.output.WriteString(fmt.Sprintf("  %s %-15s %s %d\n", _ICON_INFO, _GRAY+"Prompt:"+_RESET, _CYAN, promptTokens))
	d.output.WriteString(fmt.Sprintf("  %s %-15s %s %d\n", _ICON_INFO, _GRAY+"Completion:"+_RESET, _CYAN, completionTokens))
	d.output.WriteString(fmt.Sprintf("  %s %-15s %s %d\n", _ICON_THINK, _GRAY+"Total:"+_RESET, _WHITE+_BOLD, totalTokens))
	d.output.WriteString(fmt.Sprintf("  %s %-15s %d\n", _ICON_TOKEN, _GRAY+"Remaining:"+_RESET, contextWindow-totalTokens))
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

// PrintToolCall prints a tool call with styled output.
func (d *AIDebugger) PrintToolCall(name string, input map[string]interface{}, output string, err error) {
	if !d.config.ShowToolCalls {
		return
	}
	d.SubHeader(fmt.Sprintf("TOOL CALL: %s", name))
	inputJSON, _ := json.MarshalIndent(input, "  ", "  ")
	d.output.WriteString(fmt.Sprintf("  %s %s\n", _ICON_TOOL, _GRAY+"Input:"+_RESET))
	d.output.WriteString(fmt.Sprintf("%s", indentString(string(inputJSON), "    ")))
	if err != nil {
		d.KVError("Error", truncateString(err.Error(), 200))
	} else {
		d.output.WriteString(fmt.Sprintf("  %s %s\n", _ICON_SUCCESS, _GRAY+"Output:"+_RESET))
		d.output.WriteString(fmt.Sprintf("    %s\n", _CYAN+truncateString(output, 200)+_RESET))
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
	d.KV("Model", _CYAN+model+_RESET)
	d.KV("Provider", _CYAN+provider+_RESET)
	d.KV("Prompt", truncateString(prompt, 300))
}

// PrintAgentEnd prints agent end information.
func (d *AIDebugger) PrintAgentEnd(sessionID string, duration time.Duration, success bool, err error) {
	d.Header("AGENT END")
	d.KV("SessionID", sessionID)
	d.KV("Duration", duration)
	if success {
		d.KVSuccess("Success", "Agent completed successfully")
	} else {
		d.KVError("Success", "Agent encountered an error")
	}
	if err != nil {
		d.KVError("Error", truncateString(err.Error(), 100))
	}
}

// PrintSummary prints a summary of all debug information.
func (d *AIDebugger) PrintSummary(entries []AuditEntry, totalRetries int) {
	d.Header("DEBUG SUMMARY")

	if len(entries) > 0 {
		successCount := 0
		for _, e := range entries {
			if e.Success {
				successCount++
			}
		}
		failedCount := len(entries) - successCount
		d.KV("TotalAuditEntries", len(entries))
		d.KV("TotalRecoveryAttempts", totalRetries)
		d.output.WriteString("\n")
		d.KVSuccess("SuccessfulRecoveries", successCount)
		if failedCount > 0 {
			d.KVError("FailedRecoveries", failedCount)
		} else {
			d.KV("FailedRecoveries", failedCount)
		}
	} else {
		d.KV("TotalAuditEntries", 0)
		d.KV("TotalRecoveryAttempts", totalRetries)
	}
}

// PrintThinkContent prints streaming think content with visual indentation.
func (d *AIDebugger) PrintThinkContent(content string) {
	d.output.WriteString(fmt.Sprintf("  %s %s%s%s\n", _ICON_THINK, _MAGENTA, content, _RESET))
}

// PrintThinkChunk prints a chunk of think content inline (for streaming).
func (d *AIDebugger) PrintThinkChunk(content string) {
	d.output.WriteString(fmt.Sprintf("%s", _MAGENTA+content+_RESET))
}

// PrintBanner prints an initial banner for AI debug mode.
func (d *AIDebugger) PrintBanner(sessionID string, model string) {
	d.output.WriteString(fmt.Sprintf(`
%s ╔══════════════════════════════════════════════════════════╗%s
%s ║%s  CRUSHER AI DEBUG  %s║
%s ╠══════════════════════════════════════════════════════════╣
%s ║%s  Session: %s   Model: %s%s
%s ║%s  Type 'quit' to exit  •  'clear' to clear screen     %s
%s ╚══════════════════════════════════════════════════════════╝%s
`,
		_CYAN, _RESET,
		_CYAN, _RESET, "",
		_CYAN, _RESET,
		_CYAN, _RESET, _WHITE+sessionID[:8]+_RESET, _CYAN, _WHITE+model+_RESET,
		_CYAN, _RESET,
		_CYAN, _RESET))
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
