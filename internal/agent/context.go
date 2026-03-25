package agent

// StopReason indicates why agent execution should stop.
type StopReason int

const (
	StopReasonNone StopReason = iota
	StopReasonContextLimit // Context window near limit
	StopReasonLoopDetected // Repetitive tool calls detected
	StopReasonManual       // User cancelled
)

// StopCondition evaluates whether agent should stop.
type StopCondition struct {
	ShouldStop bool
	Reason     StopReason
}

// ContextConfig holds threshold configuration for context management.
type ContextConfig struct {
	// Token thresholds
	LargeContextThreshold int64 // Models > this use buffer-based threshold
	LargeContextBuffer   int64 // Reserved tokens for large models
	SmallContextRatio    float64 // Reserved ratio for small models
}

// DefaultContextConfig returns sensible defaults matching existing constants.
func DefaultContextConfig() ContextConfig {
	return ContextConfig{
		LargeContextThreshold: largeContextWindowThreshold,
		LargeContextBuffer:    largeContextWindowBuffer,
		SmallContextRatio:     smallContextWindowRatio,
	}
}

// ContextManager handles context window decisions.
type ContextManager struct {
	config ContextConfig
}

// NewContextManager creates a ContextManager with default config.
func NewContextManager() *ContextManager {
	return &ContextManager{
		config: DefaultContextConfig(),
	}
}

// ShouldStop evaluates context window against token usage.
// Returns StopCondition indicating if agent should stop.
func (cm *ContextManager) ShouldStop(
	ctxWindow int64,
	promptTokens int64,
	completionTokens int64,
) StopCondition {
	tokens := promptTokens + completionTokens
	remaining := ctxWindow - tokens

	var threshold int64
	if ctxWindow > cm.config.LargeContextThreshold {
		threshold = cm.config.LargeContextBuffer
	} else {
		threshold = int64(float64(ctxWindow) * cm.config.SmallContextRatio)
	}

	if remaining <= threshold {
		return StopCondition{
			ShouldStop: true,
			Reason:     StopReasonContextLimit,
		}
	}

	return StopCondition{
		ShouldStop: false,
		Reason:     StopReasonNone,
	}
}
