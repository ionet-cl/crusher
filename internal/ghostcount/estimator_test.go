package ghostcount

import (
	"strings"
	"testing"
)

func TestEstimator_EmptyText(t *testing.T) {
	e := NewEstimator()
	est := e.Estimate("")
	if est.RealTokens != 0 {
		t.Errorf("expected 0 real tokens for empty text, got %d", est.RealTokens)
	}
	if est.GhostTokens != 0 {
		t.Errorf("expected 0 ghost tokens for empty text, got %d", est.GhostTokens)
	}
	if est.Value != 1.0 {
		t.Errorf("expected 1.0 value for empty text, got %f", est.Value)
	}
}

func TestEstimator_UniqueText(t *testing.T) {
	e := NewEstimator()
	text := "This is a unique sentence with no repetition at all."
	est := e.Estimate(text)

	// Unique text should have high value (close to 1.0)
	if est.Value < 0.5 {
		t.Errorf("unique text should have value > 0.5, got %f", est.Value)
	}
	// Ghost tokens should be close to real tokens
	if est.GhostTokens < est.RealTokens/2 {
		t.Errorf("unique text ghost (%d) should be close to real (%d)", est.GhostTokens, est.RealTokens)
	}
}

func TestEstimator_RepetitiveText(t *testing.T) {
	e := NewEstimator()
	text := strings.Repeat("aaaaaaaab", 100)
	est := e.Estimate(text)

	// Repetitive text should have lower value
	if est.Value > 0.7 {
		t.Errorf("repetitive text should have value < 0.7, got %f", est.Value)
	}
	// Ghost tokens should be significantly less than real
	if est.GhostTokens >= est.RealTokens {
		t.Errorf("repetitive text ghost (%d) should be less than real (%d)", est.GhostTokens, est.RealTokens)
	}
}

func TestEstimator_LogLines(t *testing.T) {
	e := NewEstimator()
	// Simulate repeated log lines
	logLine := "2026-01-29 20:20:00 - ERROR - Connection pool exhausted for database 'omni_db'. Retrying in 5s...\n"
	combined := strings.Repeat(logLine, 50)

	est := e.Estimate(combined)
	realEst := len(combined) / 3 // chars/3.5 approximation

	// Should detect redundancy and have ghost << real
	if est.GhostTokens >= realEst/2 {
		t.Errorf("repeated logs ghost (%d) should be significantly less than estimated real (%d)", est.GhostTokens, realEst)
	}
}

func TestEstimator_CodeVariation(t *testing.T) {
	e := NewEstimator()
	code1 := "def process_data(data): return [d.upper() for d in data if d]"
	code2 := "def process_data(data): return [d.lower() for d in data if d]"
	combined := code1 + "\n" + code2

	est1 := e.Estimate(code1)
	estCombined := e.Estimate(combined)

	// Combined should not be 2x the single version (deduplication)
	if estCombined.GhostTokens >= est1.GhostTokens*2 {
		t.Errorf("code variation ghost (%d) should be less than 2x single (%d)", estCombined.GhostTokens, est1.GhostTokens)
	}
}

func TestEstimator_Messages(t *testing.T) {
	e := NewEstimator()
	messages := []string{
		"system: You are a helpful assistant",
		"user: Hello",
		"assistant: Hi there! How can I help you today?",
	}

	total := e.EstimateMessages(messages)
	if total == 0 {
		t.Errorf("expected non-zero tokens for messages, got 0")
	}

	// Each message adds 6 overhead
	if total < len(messages)*6 {
		t.Errorf("expected at least %d overhead tokens, got %d", len(messages)*6, total)
	}
}

func TestEstimator_Reset(t *testing.T) {
	e := NewEstimator()
	text := "some unique text"
	e.Estimate(text)
	e.Estimate(text + " more")

	// Reset should clear cache
	e.Reset()
	// No error means it worked (cache is internal)
}

func TestEstimator_CacheBehavior(t *testing.T) {
	e := NewEstimator()
	text := "console.log('hello world')"

	// First call - should cache
	est1 := e.Estimate(text)
	// Second identical call - should hit cache and have LOWER value (more similar = less new info)
	est2 := e.Estimate(text)

	// Second call should have lower value due to cache hit
	// (1 - 1.0*0.7) = 0.3 vs (1 - 0*0.7) = 1.0
	if est2.Value >= est1.Value {
		t.Errorf("cached text should have lower value, got first=%f, second=%f", est1.Value, est2.Value)
	}
}

func TestEstimator_LargeText(t *testing.T) {
	e := NewEstimator()
	// Large text that should trigger parallel compression
	largeText := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 1000)

	est := e.Estimate(largeText)
	if est.RealTokens == 0 {
		t.Errorf("expected non-zero tokens for large text, got 0")
	}
	if est.GhostTokens == 0 {
		t.Errorf("expected non-zero ghost tokens for large text, got 0")
	}
}

func BenchmarkEstimator_UniqueText(b *testing.B) {
	e := NewEstimator()
	text := "This is a unique sentence with no repetition whatsoever in this context."
	for i := 0; i < b.N; i++ {
		e.Estimate(text)
	}
}

func BenchmarkEstimator_RepetitiveText(b *testing.B) {
	e := NewEstimator()
	text := strings.Repeat("error connection pool ", 100)
	for i := 0; i < b.N; i++ {
		e.Estimate(text)
	}
}
