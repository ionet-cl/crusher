package ghostcount

// DREAPrefix is the prefix used to identify DREA immunity content.
const DREAPrefix = "CRITICAL: Lessons Learned from Past Errors"

// MessageCompactor manages async context compaction with selective retention.
type MessageCompactor interface {
	// Compact attempts to reduce context size using retention strategies.
	// Returns compaction result and whether async compaction is recommended.
	Compact(ctx any, messages []Message, cfg CompactionConfig, estimator TokenEstimator, truncator ContextTruncator) CompactionResult

	// ShouldCompact checks if compaction is recommended.
	ShouldCompact(tokens, threshold int) bool
}

// compactor implements MessageCompactor with RetentionMatrix.
type compactor struct {
	retentionMatrix *RetentionMatrix
}

// NewCompactor creates a new MessageCompactor.
func NewCompactor() MessageCompactor {
	return &compactor{
		retentionMatrix: NewRetentionMatrix(),
	}
}

// RetentionMatrix classifies messages by retention priority.
type RetentionMatrix struct{}

// NewRetentionMatrix creates a new retention matrix.
func NewRetentionMatrix() *RetentionMatrix {
	return &RetentionMatrix{}
}

// ClassifyMessage determines the retention level for a message.
func (rm *RetentionMatrix) ClassifyMessage(msg Message, activeSymbols []string) RetentionLevel {
	role := msg.GetRole()
	content := msg.GetContent()

	// Always retain system messages
	if role == "system" {
		return RetainAlways
	}

	// Check for DREA trauma (immunity prefix)
	if len(content) > 0 && containsDREA(content) {
		return RetainAlways
	}

	// Check if message contains active symbols (code anchors)
	if len(activeSymbols) > 0 {
		for _, symbol := range activeSymbols {
			if containsSymbol(content, symbol) {
				return RetainPriority
			}
		}
	}

	// Regular messages have normal retention
	return RetainNormal
}

// containsDREA checks for DREA trauma immunity marker.
func containsDREA(content string) bool {
	const prefix = DREAPrefix
	for i := 0; i <= len(content)-len(prefix); i++ {
		if content[i:i+len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// containsSymbol checks if content contains a symbol reference.
func containsSymbol(content, symbol string) bool {
	if len(symbol) == 0 || len(content) == 0 {
		return false
	}
	for i := 0; i <= len(content)-len(symbol); i++ {
		if content[i:i+len(symbol)] == symbol {
			return true
		}
	}
	return false
}

// ShouldCompact returns true if tokens exceed threshold.
func (c *compactor) ShouldCompact(tokens, threshold int) bool {
	return tokens >= threshold
}

// Compact attempts to compact messages using retention strategies.
// Returns the compacted result.
func (c *compactor) Compact(_ interface{}, messages []Message, cfg CompactionConfig, estimator TokenEstimator, truncator ContextTruncator) CompactionResult {
	if len(messages) == 0 {
		return CompactionResult{
			Messages:         messages,
			WasCompacted:     false,
			AsyncRecommended: false,
			TokensBefore:     0,
			TokensAfter:      0,
		}
	}

	// Calculate current tokens
	texts := messagesToStrings(messages)
	tokensBefore := estimator.EstimateMessages(texts)

	// If within budget, no compaction needed
	if tokensBefore <= cfg.HistoryThreshold {
		return CompactionResult{
			Messages:         messages,
			WasCompacted:     false,
			AsyncRecommended: false,
			TokensBefore:      tokensBefore,
			TokensAfter:       tokensBefore,
		}
	}

	// Step 1: Try lightweight truncation first (non-blocking)
	if truncator != nil {
		truncated, wasTruncated := truncator.TruncateMessages(messages, cfg.HistoryThreshold, estimator)
		if wasTruncated {
			textsAfter := messagesToStrings(truncated)
			tokensAfter := estimator.EstimateMessages(textsAfter)
			return CompactionResult{
				Messages:         truncated,
				WasCompacted:     true,
				AsyncRecommended: false,
				TokensBefore:      tokensBefore,
				TokensAfter:       tokensAfter,
			}
		}
	}

	// Step 2: Hard prune using RetentionMatrix with active symbols awareness
	compacted := c.HardPrune(messages, cfg.HistoryThreshold, estimator, cfg.ActiveSymbols)
	textsAfter := messagesToStrings(compacted)
	tokensAfter := estimator.EstimateMessages(textsAfter)

	// If still over budget, recommend async summarization
	if tokensAfter > cfg.HistoryThreshold {
		return CompactionResult{
			Messages:         compacted,
			WasCompacted:     true,
			AsyncRecommended: true,
			TokensBefore:      tokensBefore,
			TokensAfter:       tokensAfter,
		}
	}

	return CompactionResult{
		Messages:         compacted,
		WasCompacted:     tokensAfter < tokensBefore,
		AsyncRecommended: false,
		TokensBefore:      tokensBefore,
		TokensAfter:       tokensAfter,
	}
}

// HardPrune performs hard pruning keeping protected messages.
// It uses RetentionMatrix to ensure RetainAlways and RetainPriority are preserved.
func (c *compactor) HardPrune(messages []Message, budget int, estimator TokenEstimator, activeSymbols []string) []Message {
	if len(messages) == 0 {
		return messages
	}

	type classifiedMsg struct {
		msg    Message
		level  RetentionLevel
		tokens int
	}

	var alwaysMsgs, priorityMsgs, normalMsgs []classifiedMsg
	availableTokens := budget

	// Classify all messages
	for _, msg := range messages {
		level := c.retentionMatrix.ClassifyMessage(msg, activeSymbols)
		tokens := estimator.Estimate(msg.GetContent()).GhostTokens + 6

		cm := classifiedMsg{msg, level, tokens}
		switch level {
		case RetainAlways:
			alwaysMsgs = append(alwaysMsgs, cm)
			availableTokens -= tokens
		case RetainPriority:
			priorityMsgs = append(priorityMsgs, cm)
		default:
			normalMsgs = append(normalMsgs, cm)
		}
	}

	// Guard against negative
	if availableTokens < 0 {
		availableTokens = 0
	}

	// Keep priority messages (active symbols) - newest first
	var keptPriority []Message
	for i := len(priorityMsgs) - 1; i >= 0; i-- {
		if priorityMsgs[i].tokens <= availableTokens {
			keptPriority = append([]Message{priorityMsgs[i].msg}, keptPriority...)
			availableTokens -= priorityMsgs[i].tokens
		}
	}

	// Keep normal messages - newest first
	var keptNormal []Message
	for i := len(normalMsgs) - 1; i >= 0; i-- {
		if normalMsgs[i].tokens <= availableTokens {
			keptNormal = append([]Message{normalMsgs[i].msg}, keptNormal...)
			availableTokens -= normalMsgs[i].tokens
		}
	}

	// Build result: Always -> Priority -> Normal
	result := make([]Message, 0, len(messages))
	for _, cm := range alwaysMsgs {
		result = append(result, cm.msg)
	}
	result = append(result, keptPriority...)
	result = append(result, keptNormal...)

	return result
}
