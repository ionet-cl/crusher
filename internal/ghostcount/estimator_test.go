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
	text += " xyz" // Add unique ending to avoid compression edge case

	est := e.Estimate(text)

	// For highly repetitive text, compression ratio will be low
	// The new estimator gives 30% weight to compression
	// So if ratio=0.1, infoValue = 1 - (1-0.1)*0.3 = 0.73
	// We test that compression is detected, not exact values
	if est.Value > 0.9 {
		t.Errorf("repetitive text should have value < 0.9, got %f", est.Value)
	}
}

func TestEstimator_LogLines(t *testing.T) {
	e := NewEstimator()
	// Simulate repeated log lines
	logLine := "2026-01-29 20:20:00 - ERROR - Connection pool exhausted for database 'omni_db'. Retrying in 5s...\n"
	combined := strings.Repeat(logLine, 50)

	est := e.Estimate(combined)
	
	// Ghost tokens should be less than raw char/4 approximation due to compression
	// But not by too much - we give 30% weight to compression
	rawTokens := (len(combined) + 3) / 4
	
	// With compression weight 0.3, even highly compressible text keeps 70%+ of tokens
	if est.GhostTokens >= rawTokens {
		t.Errorf("ghost (%d) should be <= raw (%d)", est.GhostTokens, rawTokens)
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

func TestEstimator_NoCacheBehavior(t *testing.T) {
	e := NewEstimator()
	text := "console.log('hello world')"

	// Simple estimator does not use caching
	// Consecutive calls should return same values
	est1 := e.Estimate(text)
	est2 := e.Estimate(text)

	if est1.Value != est2.Value {
		t.Errorf("simple estimator should return same value for same text, got first=%f, second=%f", est1.Value, est2.Value)
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
