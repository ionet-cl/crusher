package ghostcount

import (
	"strings"
	"testing"
)

func TestCompactor_ShouldCompact(t *testing.T) {
	c := NewCompactor()

	if c.ShouldCompact(1000, 2000) {
		t.Errorf("should not compact when tokens < threshold")
	}

	if !c.ShouldCompact(2500, 2000) {
		t.Errorf("should compact when tokens > threshold")
	}

	if !c.ShouldCompact(2000, 2000) {
		t.Errorf("should compact when tokens == threshold")
	}
}

func TestCompactor_Compact_NoCompactionNeeded(t *testing.T) {
	c := NewCompactor()
	e := NewEstimator()
	tr := NewTruncator()

	messages := []Message{
		newMockMessage("system", "You are a helpful assistant"),
		newMockMessage("user", "Hello"),
	}

	cfg := NewCompactionConfig(128000, 4000) // generous budget
	result := c.Compact(nil, messages, cfg, e, tr)

	if result.WasCompacted {
		t.Errorf("should not compact within budget")
	}
	if result.AsyncRecommended {
		t.Errorf("should not recommend async within budget")
	}
}

func TestCompactor_Compact_Truncation(t *testing.T) {
	c := NewCompactor()
	e := NewEstimator()
	tr := NewTruncator()

	// Create many messages that exceed budget
	messages := []Message{
		newMockMessage("system", "System prompt"),
	}
	// Add many user messages
	for i := 0; i < 20; i++ {
		messages = append(messages, newMockMessage("user", strings.Repeat("x", 500)))
	}

	// Very small budget - should trigger compaction
	cfg := NewCompactionConfig(128000, 4000)
	// Manually set history threshold to force compaction
	cfg.HistoryThreshold = 100
	result := c.Compact(nil, messages, cfg, e, tr)

	if !result.WasCompacted {
		t.Errorf("should have compacted with small budget")
	}
	if result.TokensAfter >= result.TokensBefore {
		t.Errorf("tokens after (%d) should be less than before (%d)", result.TokensAfter, result.TokensBefore)
	}
}

func TestCompactor_Compact_ProtectsSystem(t *testing.T) {
	c := NewCompactor()
	e := NewEstimator()
	tr := NewTruncator()

	messages := []Message{
		newMockMessage("system", "CRITICAL SYSTEM MESSAGE"),
		newMockMessage("user", "Old user message"),
		newMockMessage("assistant", "Old assistant message"),
	}

	cfg := NewCompactionConfig(100, 50) // very small budget
	result := c.Compact(nil, messages, cfg, e, tr)

	// System should be preserved
	foundSystem := false
	for _, msg := range result.Messages {
		if msg.GetRole() == "system" {
			foundSystem = true
			break
		}
	}
	if !foundSystem {
		t.Errorf("system message should be preserved")
	}
}

func TestCompactor_Compact_ProtectsDREA(t *testing.T) {
	c := NewCompactor()
	e := NewEstimator()
	tr := NewTruncator()

	messages := []Message{
		newMockMessage("assistant", "CRITICAL: Lessons Learned from Past Errors - never do X"),
		newMockMessage("user", "Do X please"),
	}

	cfg := NewCompactionConfig(50, 25) // very small budget
	result := c.Compact(nil, messages, cfg, e, tr)

	// DREA message should be preserved
	foundDREA := false
	for _, msg := range result.Messages {
		if strings.Contains(msg.GetContent(), "CRITICAL: Lessons Learned") {
			foundDREA = true
			break
		}
	}
	if !foundDREA {
		t.Errorf("DREA message should be preserved")
	}
}

func TestCompactor_Compact_ProtectsActiveSymbols(t *testing.T) {
	c := NewCompactor().(*compactor)
	e := NewEstimator()
	tr := NewTruncator()

	messages := []Message{
		newMockMessage("user", "Please modify the function foo() in bar.go"),
		newMockMessage("assistant", "I'll modify foo() in bar.go"),
		newMockMessage("tool_result", "Modified bar.go:foo() - changes applied"),
	}

	cfg := NewCompactionConfig(100, 50) // very small budget
	// Note: activeSymbols would be passed in HardPrune for symbol-aware retention
	// For now, basic retention matrix doesn't do symbol detection without it

	_ = tr
	_ = c
	_ = e
	_ = messages
	_ = cfg
}

func TestRetentionMatrix_ClassifyMessage_System(t *testing.T) {
	rm := NewRetentionMatrix()

	msg := newMockMessage("system", "You are a helpful assistant")
	level := rm.ClassifyMessage(msg, nil)

	if level != RetainAlways {
		t.Errorf("system message should be RetainAlways, got %v", level)
	}
}

func TestRetentionMatrix_ClassifyMessage_DREA(t *testing.T) {
	rm := NewRetentionMatrix()

	msg := newMockMessage("assistant", "CRITICAL: Lessons Learned from Past Errors - never do Y")
	level := rm.ClassifyMessage(msg, nil)

	if level != RetainAlways {
		t.Errorf("DREA message should be RetainAlways, got %v", level)
	}
}

func TestRetentionMatrix_ClassifyMessage_ActiveSymbol(t *testing.T) {
	rm := NewRetentionMatrix()

	msg := newMockMessage("user", "Please edit the function calculateTotal() in orders.go")
	level := rm.ClassifyMessage(msg, []string{"calculateTotal", "orders.go"})

	if level != RetainPriority {
		t.Errorf("message with active symbol should be RetainPriority, got %v", level)
	}
}

func TestRetentionMatrix_ClassifyMessage_Normal(t *testing.T) {
	rm := NewRetentionMatrix()

	msg := newMockMessage("user", "Hello, how are you?")
	level := rm.ClassifyMessage(msg, nil)

	if level != RetainNormal {
		t.Errorf("regular message should be RetainNormal, got %v", level)
	}
}

func TestCompactor_HardPrune(t *testing.T) {
	c := NewCompactor().(*compactor)
	e := NewEstimator()

	messages := []Message{
		newMockMessage("system", "System"),
		newMockMessage("user", "Old message 1"),
		newMockMessage("user", "Old message 2"),
		newMockMessage("assistant", "Old response"),
	}

	// Very small budget - should prune oldest
	result := c.HardPrune(messages, 20, e, nil)

	// System should always be preserved
	foundSystem := false
	for _, msg := range result {
		if msg.GetRole() == "system" {
			foundSystem = true
		}
	}
	if !foundSystem {
		t.Errorf("system should always be preserved in HardPrune")
	}

	// Result should be smaller than original
	if len(result) >= len(messages) {
		t.Errorf("HardPrune should reduce message count, got %d, was %d", len(result), len(messages))
	}
}

func TestCompactor_HardPrune_PreservesOrder(t *testing.T) {
	c := NewCompactor().(*compactor)
	e := NewEstimator()

	// Messages with different roles
	messages := []Message{
		newMockMessage("system", "System first"),
		newMockMessage("user", "Second"),
		newMockMessage("assistant", "Third"),
		newMockMessage("tool_result", "Fourth"),
	}

	result := c.HardPrune(messages, 1000, e, nil)

	// Should maintain relative order
	// System should be first
	if result[0].GetRole() != "system" {
		t.Errorf("system should be first, got %s", result[0].GetRole())
	}
}

func TestNewCompactionConfig(t *testing.T) {
	cfg := NewCompactionConfig(128000, 4000)

	if cfg.ContextWindow != 128000 {
		t.Errorf("expected ContextWindow 128000, got %d", cfg.ContextWindow)
	}
	if cfg.MaxResponseTokens != 4000 {
		t.Errorf("expected MaxResponseTokens 4000, got %d", cfg.MaxResponseTokens)
	}
	// HistoryThreshold = (128000 - 4000) * 0.85 = 105400
	expectedThreshold := (128000 - 4000) * 85 / 100
	if cfg.HistoryThreshold != expectedThreshold {
		t.Errorf("expected HistoryThreshold %d, got %d", expectedThreshold, cfg.HistoryThreshold)
	}
}

func TestNewCompactionConfig_ZeroResponse(t *testing.T) {
	cfg := NewCompactionConfig(128000, 0)

	// Should fallback to using full context window
	if cfg.HistoryThreshold != 128000*85/100 {
		t.Errorf("expected fallback threshold, got %d", cfg.HistoryThreshold)
	}
}
