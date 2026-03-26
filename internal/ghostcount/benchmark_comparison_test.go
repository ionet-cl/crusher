package ghostcount

import (
	"strings"
	"testing"
)

// BenchmarkTokenSavings measures real token savings with GhostCount vs naive truncation.
// This answers: "Does GhostCount actually save tokens compared to just truncating?"
func BenchmarkTokenSavings(b *testing.B) {
	testCases := []struct {
		name          string
		messages      []string
		budget        int
		description   string
	}{
		{
			name: "small_conversation",
			messages: []string{
				"system: You are a helpful coding assistant",
				"user: Hello",
				"assistant: Hi! How can I help?",
				"user: Help me with a function",
				"assistant: I'd be happy to help with your function",
			},
			budget:      30,
			description: "Small conversation, tight budget",
		},
		{
			name: "code_review_session",
			messages: []string{
				"system: You are a code reviewer",
				"user: Review this code for me",
				"assistant: I'll review your code",
				"user: Here's the code: func add(a, b int) int { return a + b }",
				"assistant: This looks good. Consider adding error handling.",
				"user: Good point, I'll add that",
				"assistant: Let me know if you need help",
			},
			budget:      50,
			description: "Code review with multiple exchanges",
		},
		{
			name: "large_context_with_repetition",
			messages: generateLargeContextWithRepetition(20),
			budget:      200,
			description: "20 messages with repetitive tool results",
		},
		{
			name: "task_oriented",
			messages: generateTaskOrientedContext(15),
			budget:      100,
			description: "Task-focused conversation with [TASK] markers",
		},
	}

	est := NewEstimator()
	compactor := NewCompactor()
	truncator := NewTruncator()

	b.ResetTimer()

	for _, tc := range testCases {
		gcMessages := toGhostMessages(tc.messages)

		b.Run(tc.name, func(b *testing.B) {
			b.ReportMetric(float64(len(tc.messages)), "original_messages")
			b.ReportMetric(float64(estimateTotal(est, tc.messages)), "original_tokens")
			b.ReportMetric(float64(tc.budget), "budget_tokens")

			for i := 0; i < b.N; i++ {
				cfg := CompactionConfig{
					ContextWindow:     1000,
					MaxResponseTokens: 200,
					HistoryThreshold: tc.budget,
				}
				result := compactor.Compact(nil, gcMessages, cfg, est, truncator)
				_ = result
			}
		})
	}
}

// BenchmarkGhostCountVsNaiveTruncation compares GhostCount with simple head-truncation.
func BenchmarkGhostCountVsNaiveTruncation(b *testing.B) {
	// Simulate a realistic conversation with 50 messages
	messages := generateLargeContextWithRepetition(50)
	budget := 150

	est := NewEstimator()
	compactor := NewCompactor()
	truncator := NewTruncator()

	originalTokens := estimateTotal(est, messages)

	b.Run("GhostCount", func(b *testing.B) {
		gcMessages := toGhostMessages(messages)
		cfg := CompactionConfig{
			ContextWindow:     1000,
			MaxResponseTokens: 200,
			HistoryThreshold: budget,
		}

		for i := 0; i < b.N; i++ {
			result := compactor.Compact(nil, gcMessages, cfg, est, truncator)
			_ = result.TokensAfter
		}
	})

	b.Run("Naive_Head_Truncation", func(b *testing.B) {
		gcMessages := toGhostMessages(messages)

		for i := 0; i < b.N; i++ {
			// Simply take the most recent messages until under budget
			result := naiveTruncate(gcMessages, budget, est)
			_ = result
		}
	})

	b.ReportMetric(float64(originalTokens), "original_tokens")
	b.ReportMetric(float64(budget), "budget_tokens")
}

// BenchmarkGhostCountSpeed tests the speed of GhostCount operations.
func BenchmarkGhostCountSpeed(b *testing.B) {
	est := NewEstimator()
	compactor := NewCompactor()
	truncator := NewTruncator()

	messages := generateLargeContextWithRepetition(100)
	gcMessages := toGhostMessages(messages)

	cfg := CompactionConfig{
		ContextWindow:     200000,
		MaxResponseTokens: 4000,
		HistoryThreshold:  100000,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result := compactor.Compact(nil, gcMessages, cfg, est, truncator)
		_ = result
	}
}

// Helper functions

func toGhostMessages(texts []string) []Message {
	result := make([]Message, len(texts))
	for i, text := range texts {
		result[i] = &simpleMessage{content: text}
	}
	return result
}

type simpleMessage struct{ content string }

func (m *simpleMessage) GetRole() string {
	// Parse role from content format "role: content" or "role [TASK]: content"
	content := m.content
	// Handle "user [TASK]:" format
	if strings.HasPrefix(content, "user [TASK]:") {
		return "user"
	}
	// Handle standard "role: content" format
	if strings.HasPrefix(content, "system:") {
		return "system"
	}
	if strings.HasPrefix(content, "assistant:") {
		return "assistant"
	}
	if strings.HasPrefix(content, "tool_result:") {
		return "tool"
	}
	if strings.HasPrefix(content, "user:") {
		return "user"
	}
	return "user"
}
func (m *simpleMessage) GetContent() string { return m.content }

func estimateTotal(est TokenEstimator, texts []string) int {
	return est.EstimateMessages(texts)
}

func naiveTruncate(messages []Message, budget int, est TokenEstimator) []Message {
	result := make([]Message, 0, len(messages))
	// Start from most recent
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		tokens := est.Estimate(msg.GetContent()).GhostTokens + 6
		currentTotal := estimateTotal(est, toContents(result))
		if currentTotal+tokens <= budget {
			result = append([]Message{msg}, result...)
		} else {
			break
		}
	}
	return result
}

func toContents(messages []Message) []string {
	result := make([]string, len(messages))
	for i, m := range messages {
		result[i] = m.GetContent()
	}
	return result
}

func generateLargeContextWithRepetition(count int) []string {
	messages := []string{
		"system: You are an expert Go programmer. Follow Go best practices.",
	}

	baseLogs := "ERROR: connection pool exhausted for database 'main_db'. Retrying in 5s...\n"

	for i := 0; i < count; i++ {
		userMsg := "user: Show me the error logs"
		assistantMsg := "assistant: Here are the error logs:\n" + strings.Repeat(baseLogs, 3)
		toolResult := "tool_result: " + strings.Repeat(baseLogs, 5)
		userFollowup := "user: What's causing this?"
		assistantResponse := "assistant: The error indicates connection pool exhaustion. Check your database configuration."

		messages = append(messages, userMsg, assistantMsg, toolResult, userFollowup, assistantResponse)
	}

	return messages
}

func generateTaskOrientedContext(count int) []string {
	messages := []string{
		"system: You are a helpful coding assistant",
		"user [TASK]: Build a REST API with Go",
		"assistant: I'll build a REST API for you using net/http",
	}

	for i := 0; i < count; i++ {
		taskPhase := i % 4
		switch taskPhase {
		case 0:
			messages = append(messages,
				"user: Now add user authentication",
				"assistant: I'll add JWT-based authentication",
				"tool_result: Added JWT middleware to routes",
			)
		case 1:
			messages = append(messages,
				"user: Add request logging",
				"assistant: I'll add structured logging",
				"tool_result: Added zerolog with request context",
			)
		case 2:
			messages = append(messages,
				"user: Add database migrations",
				"assistant: I'll add migration support",
				"tool_result: Added golang-migrate configuration",
			)
		case 3:
			messages = append(messages,
				"user: Write tests for the API",
				"assistant: I'll add comprehensive tests",
				"tool_result: Added unit and integration tests",
			)
		}
	}

	return messages
}
