package circuit

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"charm.land/fantasy"
)

func TestDetect_NilError(t *testing.T) {
	info := Detect(nil)
	if info.IsRecoverable {
		t.Error("nil error should not be recoverable")
	}
}

func TestDetect_ContextTooLarge(t *testing.T) {
	err := &fantasy.ProviderError{
		Message:           "context length exceeded",
		StatusCode:        400,
		ContextTooLargeErr: true,
	}

	info := Detect(err)
	if !info.IsRecoverable {
		t.Error("context too large should be recoverable")
	}
	if info.Strategy != StrategyGhostCompact {
		t.Errorf("expected StrategyGhostCompact, got %v", info.Strategy)
	}
}

func TestDetect_RetryableRateLimit(t *testing.T) {
	err := &fantasy.ProviderError{
		Message:    "rate limit exceeded",
		StatusCode: http.StatusTooManyRequests,
	}

	info := Detect(err)
	if !info.IsRecoverable {
		t.Error("rate limit should be recoverable")
	}
	if info.Strategy != StrategyWaitAndRetry {
		t.Errorf("expected StrategyWaitAndRetry, got %v", info.Strategy)
	}
	if info.Delay < 1*1000000000 { // Less than 1 second
		t.Error("rate limit should have delay >= 1s")
	}
}

func TestDetect_ServerError(t *testing.T) {
	err := &fantasy.ProviderError{
		Message:    "internal server error",
		StatusCode: http.StatusInternalServerError,
	}

	info := Detect(err)
	if !info.IsRecoverable {
		t.Error("500 error should be recoverable")
	}
	if info.Strategy != StrategyWaitAndRetry {
		t.Errorf("expected StrategyWaitAndRetry, got %v", info.Strategy)
	}
}

func TestDetect_Unauthorized(t *testing.T) {
	err := &fantasy.ProviderError{
		Message:    "unauthorized",
		StatusCode: http.StatusUnauthorized,
	}

	info := Detect(err)
	if info.IsRecoverable {
		t.Error("401 should not be recoverable")
	}
}

func TestDetect_GenericError(t *testing.T) {
	err := errors.New("some generic error")

	info := Detect(err)
	if info.IsRecoverable {
		t.Error("generic error should not be recoverable")
	}
}

func TestDetect_ContextInMessage(t *testing.T) {
	err := &fantasy.ProviderError{
		Message:    "context window is too long",
		StatusCode: 400,
	}

	info := Detect(err)
	if !info.IsRecoverable {
		t.Error("400 with 'context' in message should be recoverable")
	}
	if info.Strategy != StrategyGhostCompact {
		t.Errorf("expected StrategyGhostCompact, got %v", info.Strategy)
	}
}

func TestBuildRecoveryMessage(t *testing.T) {
	err := errors.New("context too large")
	msg := BuildRecoveryMessage(err)

	expected := "Error: context too large. Continue from where you left off."
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}
}

func TestGetRetryDelay(t *testing.T) {
	tests := []struct {
		statusCode int
		minDelay  time.Duration
	}{
		{429, 5 * time.Second},
		{500, 2 * time.Second},
		{503, 2 * time.Second},
		{504, 2 * time.Second},
		{408, 1 * time.Second},
	}

	for _, tc := range tests {
		delay := getRetryDelay(tc.statusCode)
		if delay < tc.minDelay {
			t.Errorf("status %d: delay %v < min %v", tc.statusCode, delay, tc.minDelay)
		}
	}
}
