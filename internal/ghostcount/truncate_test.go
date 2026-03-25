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

	messages := []Message{
		newMockMessage("system", "You are a helpful assistant"),
		newMockMessage("user", "[TASK] Do something"),
		newMockMessage("assistant", "I did something"),
		newMockMessage("user", "Continue"),
	}

	// Very small budget - should only keep protected messages
	result, wasTruncated := tr.TruncateMessages(messages, 10, e)

	if !wasTruncated {
		t.Errorf("expected truncation with small budget")
	}

	// System message should be preserved
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

	// Create messages where only recent ones matter
	messages := []Message{
		newMockMessage("system", "System prompt"),
		newMockMessage("user", "Old message 1"),
		newMockMessage("user", "Old message 2"),
		newMockMessage("assistant", "Recent response"),
		newMockMessage("user", "Most recent user message"),
	}

	// Small budget
	result, _ := tr.TruncateMessages(messages, 50, e)

	// Last 2 messages should be preserved
	if len(result) < 2 {
		t.Errorf("should preserve at least 2 recent messages, got %d", len(result))
	}

	// Most recent should be last
	lastMsg := result[len(result)-1]
	if lastMsg.GetContent() != "Most recent user message" {
		t.Errorf("most recent message should be last, got %s", lastMsg.GetContent())
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
