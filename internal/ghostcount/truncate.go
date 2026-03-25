package ghostcount

// ContextTruncator handles selective truncation of message content.
type ContextTruncator interface {
	// TruncateToolOutput truncates tool output preserving head and tail.
	TruncateToolOutput(output string) string

	// TruncateMessages removes messages when approaching token budget.
	// Returns pruned messages and whether truncation occurred.
	TruncateMessages(msgs []Message, budget int, estimator TokenEstimator) ([]Message, bool)

	// ShouldTruncate checks if truncation is needed.
	ShouldTruncate(msgs []Message, budget int, estimator TokenEstimator) bool
}

// truncator implements ContextTruncator.
type truncator struct {
	config TruncationConfig
}

// NewTruncator creates a new ContextTruncator with default config.
func NewTruncator() ContextTruncator {
	return NewTruncatorWithConfig(DefaultTruncationConfig())
}

// NewTruncatorWithConfig creates a new ContextTruncator with custom config.
func NewTruncatorWithConfig(cfg TruncationConfig) ContextTruncator {
	if cfg.MaxChars <= 0 {
		cfg.MaxChars = 2048
	}
	if cfg.HeadChars <= 0 {
		cfg.HeadChars = 800
	}
	if cfg.TailChars <= 0 {
		cfg.TailChars = cfg.MaxChars - cfg.HeadChars - len(cfg.Marker)
	}
	if cfg.Marker == "" {
		cfg.Marker = TruncationMarker
	}
	return &truncator{config: cfg}
}

// TruncateToolOutput truncates tool output preserving head and tail.
// Pattern: head + marker + tail
func (t *truncator) TruncateToolOutput(output string) string {
	if len(output) <= t.config.MaxChars {
		return output
	}

	headLen := t.config.HeadChars
	tailLen := t.config.MaxChars - headLen - len(t.config.Marker)
	if tailLen < 100 {
		// If tail would be too small, just truncate to max
		return output[:t.config.MaxChars-3] + "..."
	}

	return output[:headLen] + t.config.Marker + output[len(output)-tailLen:]
}

// ShouldTruncate checks if total tokens exceed budget.
func (t *truncator) ShouldTruncate(msgs []Message, budget int, estimator TokenEstimator) bool {
	totalTokens := estimator.EstimateMessages(messagesToStrings(msgs))
	return totalTokens > budget
}

// TruncateMessages removes oldest messages from the middle when approaching token budget.
// Protected: RetainAlways messages, RetainPriority with active symbols, last 2 messages.
func (t *truncator) TruncateMessages(msgs []Message, budget int, estimator TokenEstimator) ([]Message, bool) {
	if len(msgs) <= 3 {
		return msgs, false
	}

	// Calculate current tokens
	texts := messagesToStrings(msgs)
	totalTokens := estimator.EstimateMessages(texts)

	if totalTokens <= budget {
		return msgs, false
	}

	// Identify protected indices
	protected := make(map[int]bool)
	for i, msg := range msgs {
		role := msg.GetRole()
		content := msg.GetContent()

		// Always protect system messages
		if role == "system" {
			protected[i] = true
			continue
		}

		// Always protect task messages (contain [TASK] marker)
		if role == "user" && containsTaskMarker(content) {
			protected[i] = true
			continue
		}
	}

	// Protect last 2 messages (most recent context)
	if len(msgs) >= 1 {
		protected[len(msgs)-1] = true
	}
	if len(msgs) >= 2 {
		protected[len(msgs)-2] = true
	}

	// Truncate from middle, oldest first, until budget is met
	result := make([]Message, 0, len(msgs))

	// First pass: add all protected messages
	for i, msg := range msgs {
		if protected[i] {
			result = append(result, msg)
		}
	}

	// Second pass: add remaining messages, truncating content if needed
	for i, msg := range msgs {
		if protected[i] {
			continue
		}

		// Check budget before adding
		currentTokens := estimator.EstimateMessages(messagesToStrings(result))
		msgTokens := estimator.Estimate(msg.GetContent()).GhostTokens + 6

		if currentTokens+msgTokens <= budget {
			result = append(result, msg)
		} else {
			// Truncate this message's content
			truncated := truncateMessageContent(msg, t.config.MaxChars)
			result = append(result, truncated)

			// Check again after truncation
			newTokens := estimator.EstimateMessages(messagesToStrings(result))
			if newTokens <= budget {
				continue
			}
			// Even truncated didn't fit - stop adding
			break
		}
	}

	// Verify result actually improved the situation
	finalTokens := estimator.EstimateMessages(messagesToStrings(result))
	return result, finalTokens < totalTokens
}

func (t *truncator) estimateTotalTokens(msgs []Message, estimator TokenEstimator) int {
	texts := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		texts = append(texts, msg.GetContent())
	}
	return estimator.EstimateMessages(texts)
}

// messagesToStrings extracts content strings from messages.
func messagesToStrings(msgs []Message) []string {
	result := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		result = append(result, msg.GetContent())
	}
	return result
}

// containsTaskMarker checks if content contains [TASK] marker.
func containsTaskMarker(content string) bool {
	const taskMarker = "[TASK]"
	for i := 0; i <= len(content)-len(taskMarker); i++ {
		if content[i:i+len(taskMarker)] == taskMarker {
			return true
		}
	}
	return false
}

// truncateMessageContent truncates a message's content to maxChars.
func truncateMessageContent(msg Message, maxChars int) Message {
	return &truncatedMessage{
		original: msg,
		maxChars: maxChars,
	}
}

// truncatedMessage wraps a message with truncated content.
type truncatedMessage struct {
	original Message
	maxChars int
}

func (m *truncatedMessage) GetRole() string {
	return m.original.GetRole()
}

func (m *truncatedMessage) GetContent() string {
	content := m.original.GetContent()
	if len(content) <= m.maxChars {
		return content
	}
	return content[:m.maxChars-len(TruncationMarker)-3] + TruncationMarker + "..."
}
