package agent

// StopReason indicates why agent execution should stop.
type StopReason int

const (
	StopReasonNone StopReason = iota
	StopReasonContextLimit
	StopReasonLoopDetected
	StopReasonManual
)

// StopCondition evaluates whether agent should stop.
type StopCondition struct {
	ShouldStop bool
	Reason    StopReason
}

// ContextConfig holds threshold configuration for context management.
type ContextConfig struct {
	LargeContextThreshold int64
	LargeContextBuffer   int64
	SmallContextRatio    float64
}

// DefaultContextConfig returns sensible defaults.
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

// NewContextManager creates a ContextManager.
func NewContextManager() *ContextManager {
	return &ContextManager{config: DefaultContextConfig()}
}

// ShouldStop evaluates context window against token usage.
func (cm *ContextManager) ShouldStop(ctxWindow, promptTokens, completionTokens int64) StopCondition {
	remaining := ctxWindow - (promptTokens + completionTokens)

	var threshold int64
	if ctxWindow > cm.config.LargeContextThreshold {
		threshold = cm.config.LargeContextBuffer
	} else {
		threshold = int64(float64(ctxWindow) * cm.config.SmallContextRatio)
	}

	if remaining <= threshold {
		return StopCondition{ShouldStop: true, Reason: StopReasonContextLimit}
	}
	return StopCondition{ShouldStop: false, Reason: StopReasonNone}
}
