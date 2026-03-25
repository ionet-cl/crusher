// Package agent provides circuit breaker and recovery logic with full audit trail.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent/circuit"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/session"
)

const maxCircuitBreakerRetries = 2

// circuitBreakerRetryCount tracks retry attempts per session.
var circuitBreakerRetryCount = make(map[string]int)

// MaxCircuitBreakerRetries returns the maximum number of retry attempts allowed.
func MaxCircuitBreakerRetries() int {
	return maxCircuitBreakerRetries
}

// AuditEntry represents a single recovery action for audit trail.
type AuditEntry struct {
	SessionID    string
	Timestamp    time.Time
	Action       string
	Strategy     circuit.Strategy
	Error        string
	RecoveryMsg  string
	Success      bool
	Details      string
}

// RecoveryAudit stores all recovery actions for debugging.
var RecoveryAudit []AuditEntry

// RecoveryManager handles error recovery for agent sessions.
type RecoveryManager struct {
	enableGhostCount     bool
	enableCircuitBreaker bool
	messages            message.Service
	sessions            session.Service
	ghostCompact        func(ctx context.Context, sessionID string, forceCompact bool) error
}

// NewRecoveryManager creates a recovery manager.
func NewRecoveryManager(opts RecoveryManagerOptions) *RecoveryManager {
	return &RecoveryManager{
		enableGhostCount:     opts.EnableGhostCount,
		enableCircuitBreaker: opts.EnableCircuitBreaker,
		messages:             opts.Messages,
		sessions:             opts.Sessions,
		ghostCompact:        opts.GhostCompact,
	}
}

// RecoveryManagerOptions configures the recovery manager.
type RecoveryManagerOptions struct {
	EnableGhostCount     bool
	EnableCircuitBreaker bool
	Messages             message.Service
	Sessions             session.Service
	GhostCompact         func(ctx context.Context, sessionID string, forceCompact bool) error
}

// CanRetry checks if we have retries remaining for this session.
func (rm *RecoveryManager) CanRetry(sessionID string) bool {
	count := circuitBreakerRetryCount[sessionID]
	if count >= maxCircuitBreakerRetries {
		slog.Warn("circuit_breaker: max retries exceeded",
			"sid", sessionID,
			"count", count,
			"max", maxCircuitBreakerRetries)
		return false
	}
	circuitBreakerRetryCount[sessionID] = count + 1
	slog.Debug("circuit_breaker: retry allowed",
		"sid", sessionID,
		"attempt", count+1,
		"max", maxCircuitBreakerRetries)
	return true
}

// AttemptRecovery tries to recover from an error based on the strategy.
// Full audit trail is maintained for all operations.
func (rm *RecoveryManager) AttemptRecovery(ctx context.Context, call SessionAgentCall, err error) (bool, error) {
	entry := AuditEntry{
		SessionID: call.SessionID,
		Timestamp: time.Now(),
		Error: err.Error(),
	}

	if !rm.enableCircuitBreaker {
		entry.Action = "disabled"
		entry.Success = false
		entry.Details = "circuit breaker is disabled"
		slog.Warn("circuit_breaker: recovery disabled, passing through error",
			"sid", call.SessionID,
			"error", err)
		RecoveryAudit = append(RecoveryAudit, entry)
		return false, err
	}

	errInfo := circuit.Detect(err)
	entry.Strategy = errInfo.Strategy

	if !errInfo.IsRecoverable {
		entry.Action = "not_recoverable"
		entry.Success = false
		entry.Details = fmt.Sprintf("error type: %s, message: %s", errInfo.ErrMsg, err)
		slog.Warn("circuit_breaker: error not recoverable",
			"sid", call.SessionID,
			"error", err,
			"err_msg", errInfo.ErrMsg,
			"strategy", errInfo.Strategy)
		RecoveryAudit = append(RecoveryAudit, entry)
		return false, err
	}

	if !rm.CanRetry(call.SessionID) {
		entry.Action = "max_retries_exceeded"
		entry.Success = false
		entry.Details = "circuit breaker retry limit reached"
		RecoveryAudit = append(RecoveryAudit, entry)
		return false, err
	}

	slog.Info("circuit_breaker: attempting recovery",
		"sid", call.SessionID,
		"strategy", errInfo.Strategy,
		"strategy_name", GetStrategyName(errInfo.Strategy),
		"error", errInfo.ErrMsg,
		"delay", errInfo.Delay)

	entry.Action = "attempting"

	var recoveryErr error
	switch errInfo.Strategy {
	case circuit.StrategyGhostCompact:
		recoveryErr = rm.recoverWithGhostCompact(ctx, call, errInfo)
		entry.Action = "ghost_compact"
	case circuit.StrategyWaitAndRetry:
		recoveryErr = rm.recoverWithWait(ctx, call, errInfo)
		entry.Action = "wait_retry"
	case circuit.StrategyCleanAndRetry:
		recoveryErr = rm.recoverWithClean(ctx, call, errInfo)
		entry.Action = "clean_retry"
	default:
		recoveryErr = fmt.Errorf("unknown strategy: %d", errInfo.Strategy)
		entry.Action = "unknown_strategy"
	}

	if recoveryErr != nil {
		entry.Success = false
		entry.Details = recoveryErr.Error()
		slog.Error("circuit_breaker: recovery failed",
			"sid", call.SessionID,
			"strategy", entry.Action,
			"error", recoveryErr)
	} else {
		entry.Success = true
		entry.Details = fmt.Sprintf("recovery action completed, strategy: %s", GetStrategyName(errInfo.Strategy))
		slog.Info("circuit_breaker: recovery completed",
			"sid", call.SessionID,
			"strategy", entry.Action,
			"will_retry", true)
	}

	RecoveryAudit = append(RecoveryAudit, entry)
	return recoveryErr == nil, recoveryErr
}

// StrategyName returns a human-readable name for a circuit.Strategy.
func GetStrategyName(s circuit.Strategy) string {
	switch s {
	case circuit.StrategyGhostCompact:
		return "ghost_compact"
	case circuit.StrategyWaitAndRetry:
		return "wait_retry"
	case circuit.StrategyCleanAndRetry:
		return "clean_retry"
	default:
		return "unknown"
	}
}

func (rm *RecoveryManager) recoverWithGhostCompact(ctx context.Context, call SessionAgentCall, errInfo circuit.ErrInfo) error {
	action := "ghost_compact_start"
	slog.Info("recovery_ghost_compact: starting",
		"sid", call.SessionID,
		"error", errInfo.ErrMsg)

	if rm.enableGhostCount && rm.ghostCompact != nil {
		if compactErr := rm.ghostCompact(ctx, call.SessionID, true); compactErr != nil {
			slog.Error("recovery_ghost_compact: compact failed",
				"sid", call.SessionID,
				"error", compactErr)
			action = "ghost_compact_failed"
			// Continue anyway - we'll still try with recovery message
		} else {
			slog.Info("recovery_ghost_compact: compact succeeded",
				"sid", call.SessionID)
			action = "ghost_compact_success"
		}
	} else {
		slog.Warn("recovery_ghost_compact: ghost_count not enabled or noop",
			"sid", call.SessionID)
		action = "ghost_compact_disabled"
	}

	recoveryMsg := circuit.BuildRecoveryMessage(errors.New(errInfo.ErrMsg))
	slog.Debug("recovery_ghost_compact: creating recovery message",
		"sid", call.SessionID,
		"message", recoveryMsg)

	_, createErr := rm.messages.Create(ctx, call.SessionID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: recoveryMsg}},
	})
	if createErr != nil {
		slog.Error("recovery_ghost_compact: failed to create recovery message",
			"sid", call.SessionID,
			"error", createErr)
		return createErr
	}

	slog.Info("recovery_ghost_compact: recovery message created",
		"sid", call.SessionID,
		"action", action,
		"message", recoveryMsg)
	return nil
}

func (rm *RecoveryManager) recoverWithWait(ctx context.Context, call SessionAgentCall, errInfo circuit.ErrInfo) error {
	slog.Info("recovery_wait: starting",
		"sid", call.SessionID,
		"delay", errInfo.Delay)

	time.Sleep(errInfo.Delay)

	slog.Info("recovery_wait: delay complete",
		"sid", call.SessionID)
	return nil
}

func (rm *RecoveryManager) recoverWithClean(ctx context.Context, call SessionAgentCall, errInfo circuit.ErrInfo) error {
	recoveryMsg := circuit.BuildRecoveryMessage(errors.New(errInfo.ErrMsg))

	slog.Info("recovery_clean: starting",
		"sid", call.SessionID,
		"message", recoveryMsg)

	_, createErr := rm.messages.Create(ctx, call.SessionID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: recoveryMsg}},
	})
	if createErr != nil {
		slog.Error("recovery_clean: failed to create recovery message",
			"sid", call.SessionID,
			"error", createErr)
		return createErr
	}

	slog.Info("recovery_clean: recovery message created",
		"sid", call.SessionID)
	return nil
}

// GetAuditTrail returns the full recovery audit trail.
func GetAuditTrail() []AuditEntry {
	return RecoveryAudit
}

// ClearAuditTrail clears the audit trail (useful for testing).
func ClearAuditTrail() {
	RecoveryAudit = nil
}

// LogProviderError creates a structured log for provider errors.
func LogProviderError(err *fantasy.ProviderError, sessionID string) {
	if err == nil {
		return
	}

	slog.Error("provider_error",
		"sid", sessionID,
		"type", "provider_error",
		"title", err.Title,
		"message", err.Message,
		"status_code", err.StatusCode,
		"context_too_large", err.IsContextTooLarge(),
		"retryable", err.IsRetryable(),
		"context_used", err.ContextUsedTokens,
		"context_max", err.ContextMaxTokens)
}
