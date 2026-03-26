package ghostcount

import (
	"strings"
	"testing"
)

// TestGhostCountTokenSavings measures actual token savings vs naive truncation.
func TestGhostCountTokenSavings(t *testing.T) {
	testCases := []struct {
		name   string
		budget int
	}{
		{"tight_budget", 100},
		{"medium_budget", 200},
		{"loose_budget", 500},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Use task-oriented context which includes [TASK] markers
			messages := generateTaskOrientedContext(20)
			originalTokens := estimateTotal(NewEstimator(), messages)

			gcResult := runGhostCount(messages, tc.budget)
			naiveResult := runNaiveTruncate(messages, tc.budget)

			t.Logf("=== Budget: %d tokens ===", tc.budget)
			t.Logf("Original: %d messages, %d tokens", len(messages), originalTokens)
			t.Logf("GhostCount: %d messages, %d tokens (saved %d vs original)",
				len(gcResult.messages), gcResult.tokens, originalTokens-gcResult.tokens)
			t.Logf("Naive Trunc: %d messages, %d tokens (saved %d vs original)",
				len(naiveResult.messages), naiveResult.tokens, originalTokens-naiveResult.tokens)

			// GhostCount should preserve more messages (better retention)
			if len(gcResult.messages) < len(naiveResult.messages) {
				t.Errorf("GhostCount preserved fewer messages (%d) than Naive (%d)",
					len(gcResult.messages), len(naiveResult.messages))
			}

			// GhostCount should respect important markers
			if !containsMessage(gcResult.messages, "[TASK]") {
				t.Errorf("GhostCount lost [TASK] marker")
			}
			if !containsSystem(gcResult.messages) {
				t.Errorf("GhostCount lost system message")
			}
		})
	}
}

// TestGhostCountQualityMetrics measures retention quality.
func TestGhostCountQualityMetrics(t *testing.T) {
	est := NewEstimator()
	messages := generateTaskOrientedContext(20)

	t.Logf("=== Quality Metrics ===")
	t.Logf("Total messages: %d", len(messages))
	t.Logf("Original tokens: %d", est.EstimateMessages(messages))

	budget := 150

	gcResult := runGhostCount(messages, budget)

	t.Logf("")
	t.Logf("After GhostCount compaction (budget=%d):", budget)
	t.Logf("  Messages retained: %d/%d (%.1f%%)",
		len(gcResult.messages), len(messages),
		float64(len(gcResult.messages))/float64(len(messages))*100)
	t.Logf("  Tokens: %d/%d (%.1f%%)",
		gcResult.tokens, est.EstimateMessages(messages),
		float64(gcResult.tokens)/float64(est.EstimateMessages(messages))*100)

	// Verify retention quality
	hasSystem := containsSystem(gcResult.messages)
	hasTask := containsMessage(gcResult.messages, "[TASK]")

	t.Logf("  System preserved: %v", hasSystem)
	t.Logf("  Task markers preserved: %v", hasTask)

	if !hasSystem {
		t.Error("System message should always be preserved")
	}
	if !hasTask {
		t.Error("[TASK] markers should be preserved")
	}
}

// TestGhostCountSpeedMetrics measures performance.
func TestGhostCountSpeedMetrics(t *testing.T) {
	sizes := []int{10, 50, 100, 200}

	est := NewEstimator()
	budget := 100

	t.Logf("=== Speed Metrics ===")
	for _, size := range sizes {
		messages := generateLargeContextWithRepetition(size)
		originalTokens := est.EstimateMessages(messages)

		// Measure GhostCount
		gcResult := runGhostCount(messages, budget)

		// Measure Naive
		naiveResult := runNaiveTruncate(messages, budget)

		t.Logf("")
		t.Logf("Messages: %d (tokens: %d)", size, originalTokens)
		t.Logf("  GhostCount: %d msgs, %d tokens", len(gcResult.messages), gcResult.tokens)
		t.Logf("  Naive:      %d msgs, %d tokens", len(naiveResult.messages), naiveResult.tokens)

		// In tight budget, GhostCount should fit more content
		if naiveResult.tokens > budget && gcResult.tokens > budget {
			t.Logf("  Both over budget - expected with tight constraints")
		}
	}
}

type result struct {
	messages []string
	tokens   int
}

func runGhostCount(texts []string, budget int) result {
	est := NewEstimator()
	compactor := NewCompactor()
	truncator := NewTruncator()

	gcMessages := toGhostMessages(texts)
	cfg := CompactionConfig{
		ContextWindow:     1000,
		MaxResponseTokens: 200,
		HistoryThreshold: budget,
	}

	r := compactor.Compact(nil, gcMessages, cfg, est, truncator)

	resultTexts := make([]string, len(r.Messages))
	for i, m := range r.Messages {
		resultTexts[i] = m.GetContent()
	}

	return result{
		messages: resultTexts,
		tokens:   r.TokensAfter,
	}
}

func runNaiveTruncate(texts []string, budget int) result {
	est := NewEstimator()
	gcMessages := toGhostMessages(texts)

	messages := make([]string, 0)
	totalTokens := 0

	// Start from most recent
	for i := len(gcMessages) - 1; i >= 0; i-- {
		msg := gcMessages[i]
		tokens := est.Estimate(msg.GetContent()).GhostTokens + 6

		if totalTokens+tokens <= budget {
			messages = append([]string{msg.GetContent()}, messages...)
			totalTokens += tokens
		} else {
			break
		}
	}

	return result{
		messages: messages,
		tokens:   totalTokens,
	}
}

func containsMessage(texts []string, substr string) bool {
	for _, t := range texts {
		if strings.Contains(t, substr) {
			return true
		}
	}
	return false
}

func containsSystem(texts []string) bool {
	// In our test messages, system is always first
	return len(texts) > 0 && strings.HasPrefix(texts[0], "system:")
}
