package circuit

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"

	"charm.land/fantasy"
)

// Strategy defines how to recover from an error.
type Strategy int

const (
	StrategyNone Strategy = iota
	StrategyGhostCompact    // Context too large - compact and retry
	StrategyWaitAndRetry   // Rate limit or server error - wait then retry
	StrategyCleanAndRetry  // Loop or malformed - clean messages then retry
)

// ErrInfo contains error classification for recovery decisions.
type ErrInfo struct {
	IsRecoverable bool
	Strategy      Strategy
	Delay         time.Duration
	ErrMsg        string
}

// RecoveryMessage is injected when recovery is needed.
const RecoveryMessageTemplate = "Error: %s. Continue from where you left off."

// Detect classifies an error and returns recovery strategy.
func Detect(err error) ErrInfo {
	if err == nil {
		return ErrInfo{IsRecoverable: false}
	}

	var providerErr *fantasy.ProviderError
	if errors.As(err, &providerErr) {
		return classifyProviderError(providerErr)
	}

	var fantasyErr *fantasy.Error
	if errors.As(err, &fantasyErr) {
		return ErrInfo{
			IsRecoverable: false,
			ErrMsg:        fantasyErr.Message,
		}
	}

	// Check for net errors (connection reset, refused, timeout, EOF)
	if isNetError(err) {
		return ErrInfo{
			IsRecoverable: true,
			Strategy:      StrategyWaitAndRetry,
			Delay:         3 * time.Second,
			ErrMsg:        err.Error(),
		}
	}

	// Generic error - not recoverable
	return ErrInfo{
		IsRecoverable: false,
		ErrMsg:        err.Error(),
	}
}

func classifyProviderError(err *fantasy.ProviderError) ErrInfo {
	// Context too large - use GhostCompact
	if err.IsContextTooLarge() {
		return ErrInfo{
			IsRecoverable: true,
			Strategy:      StrategyGhostCompact,
			ErrMsg:        err.Message,
		}
	}

	// Retryable error (429, 5xx, etc.) - wait and retry
	if err.IsRetryable() {
		delay := getRetryDelay(err.StatusCode)
		return ErrInfo{
			IsRecoverable: true,
			Strategy:      StrategyWaitAndRetry,
			Delay:         delay,
			ErrMsg:        err.Message,
		}
	}

	// Check for specific error types
	switch err.StatusCode {
	case 400:
		// Bad request - might be context or malformed
		if contains(err.Message, "context") || contains(err.Message, "too long") {
			return ErrInfo{
				IsRecoverable: true,
				Strategy:      StrategyGhostCompact,
				ErrMsg:        err.Message,
			}
		}
		// Generic 400 - not recoverable
		return ErrInfo{
			IsRecoverable: false,
			ErrMsg:        err.Message,
		}
	case 401, 403:
		// Auth errors - not recoverable, user must fix
		return ErrInfo{
			IsRecoverable: false,
			ErrMsg:        err.Message,
		}
	case 429:
		// Rate limit - retry with delay
		return ErrInfo{
			IsRecoverable: true,
			Strategy:      StrategyWaitAndRetry,
			Delay:         getRetryDelay(429),
			ErrMsg:        err.Message,
		}
	case 500, 502, 503, 504:
		// Server errors - retry with backoff
		return ErrInfo{
			IsRecoverable: true,
			Strategy:      StrategyWaitAndRetry,
			Delay:         getRetryDelay(err.StatusCode),
			ErrMsg:        err.Message,
		}
	}

	// Unknown error - not recoverable
	return ErrInfo{
		IsRecoverable: false,
		ErrMsg:        err.Message,
	}
}

func getRetryDelay(statusCode int) time.Duration {
	switch statusCode {
	case 429:
		// Rate limit - wait 5 seconds
		return 5 * time.Second
	case 500, 502, 503, 504:
		// Server error - exponential backoff-ish, use 2 seconds
		return 2 * time.Second
	default:
		return 1 * time.Second
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// BuildRecoveryMessage creates the recovery prompt message.
func BuildRecoveryMessage(err error) string {
	return fmt.Sprintf(RecoveryMessageTemplate, err.Error())
}
