package ghostcount

import (
	"strings"
	"testing"
)

// mockMessage implements Message for testing.
type mockMessage struct {
	role    string
	content string
}

func (m *mockMessage) GetRole() string { return m.role }
func (m *mockMessage) GetContent() string { return m.content }

func newMockMessage(role, content string) Message {
	return &mockMessage{role: role, content: content}
}

func TestTruncator_TruncateToolOutput_Short(t *testing.T) {
	tr := NewTruncator().(*truncator)
	output := "short output"
	result := tr.TruncateToolOutput(output)
	if result != output {
		t.Errorf("short output should not be truncated, got %s", result)
	}
}

func TestTruncator_TruncateToolOutput_Long(t *testing.T) {
	tr := NewTruncator().(*truncator)
	// Create output longer than MaxChars (2048)
	output := strings.Repeat("x", 3000)
	result := tr.TruncateToolOutput(output)

	// Should contain truncation marker
	if !strings.Contains(result, TruncationMarker) {
		t.Errorf("truncated output should contain marker")
	}
	// Should be shorter than original
	if len(result) >= len(output) {
		t.Errorf("truncated output (%d) should be shorter than original (%d)", len(result), len(output))
	}
	// Should preserve beginning
	if !strings.HasPrefix(result, "xxxx") {
		t.Errorf("truncated output should preserve beginning, got %s", result[:10])
	}
	// Should preserve end
	if !strings.HasSuffix(result, strings.Repeat("x", tr.config.TailChars)) {
		t.Errorf("truncated output should preserve end")
	}
}

func TestTruncator_ShouldTruncate(t *testing.T) {
	tr := NewTruncator()
	e := NewEstimator()

	messages := []Message{
		newMockMessage("system", "You are a helpful assistant"),
		newMockMessage("user", "Hello"),
	}

	// With generous budget, should not truncate
	if tr.ShouldTruncate(messages, 10000, e) {
		t.Errorf("should not truncate with large budget")
	}

	// With tiny budget, should truncate
	if !tr.ShouldTruncate(messages, 1, e) {
		t.Errorf("should truncate with tiny budget")
	}
}

func TestTruncator_TruncateMessages_PreservesSystem(t *testing.T) {
	tr := NewTruncator()
	e := NewEstimator()

	// Create many messages to ensure truncation occurs
	messages := []Message{
		newMockMessage("system", "System prompt"),
		newMockMessage("user", "Message 1"),
		newMockMessage("assistant", "Response 1"),
		newMockMessage("user", "Message 2"),
		newMockMessage("assistant", "Response 2"),
		newMockMessage("user", "Message 3"),
		newMockMessage("assistant", "Response 3"),
	}

	// Use ShouldTruncate to verify truncation is needed with small budget
	if !tr.ShouldTruncate(messages, 30, e) {
		t.Errorf("should need truncation with small budget")
	}

	// Test that TruncateMessages preserves system message
	result, _ := tr.TruncateMessages(messages, 30, e)

	// System should always be in result
	foundSystem := false
	for _, msg := range result {
		if msg.GetRole() == "system" {
			foundSystem = true
			break
		}
	}
	if !foundSystem {
		t.Errorf("system message should be preserved")
	}
}

func TestTruncator_TruncateMessages_PreservesTask(t *testing.T) {
	tr := NewTruncator()
	e := NewEstimator()

	messages := []Message{
		newMockMessage("user", "Regular message"),
		newMockMessage("user", "[TASK] Important task here"),
		newMockMessage("assistant", "Working on it"),
	}

	// Small budget
	result, _ := tr.TruncateMessages(messages, 10, e)

	// Task message should be preserved
	foundTask := false
	for _, msg := range result {
		if strings.Contains(msg.GetContent(), "[TASK]") {
			foundTask = true
			break
		}
	}
	if !foundTask {
		t.Errorf("[TASK] message should be preserved")
	}
}

func TestTruncator_TruncateMessages_PreservesRecent(t *testing.T) {
	tr := NewTruncator()
	e := NewEstimator()

	// Create messages where recent ones are important
	messages := []Message{
		newMockMessage("system", "System prompt"),
		newMockMessage("user", "Old message 1"),
		newMockMessage("user", "Old message 2"),
		newMockMessage("assistant", "Recent response"),
		newMockMessage("user", "Most recent user message"),
	}

	// Verify truncation is needed
	if !tr.ShouldTruncate(messages, 40, e) {
		t.Errorf("should need truncation with small budget")
	}

	result, wasTruncated := tr.TruncateMessages(messages, 40, e)

	if !wasTruncated {
		t.Errorf("expected truncation to occur")
	}

	// Last 2 messages should be preserved somewhere in result
	foundRecent := false
	foundMostRecent := false
	for _, msg := range result {
		content := msg.GetContent()
		if content == "Recent response" {
			foundRecent = true
		}
		if content == "Most recent user message" {
			foundMostRecent = true
		}
	}

	if !foundRecent {
		t.Errorf("recent response should be preserved")
	}
	if !foundMostRecent {
		t.Errorf("most recent message should be preserved")
	}
}

func TestTruncator_TruncateMessages_NoTruncationNeeded(t *testing.T) {
	tr := NewTruncator()
	e := NewEstimator()

	messages := []Message{
		newMockMessage("system", "Short prompt"),
		newMockMessage("user", "Hi"),
	}

	// Large budget - no truncation needed
	result, wasTruncated := tr.TruncateMessages(messages, 10000, e)

	if wasTruncated {
		t.Errorf("should not truncate with large budget")
	}
	if len(result) != len(messages) {
		t.Errorf("should return all messages, got %d, want %d", len(result), len(messages))
	}
}

func TestTruncator_CustomConfig(t *testing.T) {
	cfg := TruncationConfig{
		MaxChars:  1024,
		HeadChars: 400,
		TailChars: 1024 - 400 - len(TruncationMarker),
		Marker:    TruncationMarker,
	}
	tr := NewTruncatorWithConfig(cfg).(*truncator)

	if tr.config.MaxChars != 1024 {
		t.Errorf("expected MaxChars 1024, got %d", tr.config.MaxChars)
	}
}

func TestTruncatedMessage(t *testing.T) {
	original := newMockMessage("assistant", strings.Repeat("x", 5000))
	truncated := truncateMessageContent(original, 100)

	if truncated.GetRole() != "assistant" {
		t.Errorf("role should be preserved")
	}

	content := truncated.GetContent()
	if len(content) >= 5000 {
		t.Errorf("content should be truncated")
	}
}
