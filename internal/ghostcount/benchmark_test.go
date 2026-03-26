package ghostcount

import (
	"testing"

	"github.com/pkoukk/tiktoken-go"
)

// BenchmarkGhostCountVsTiktoken compares our estimator against tiktoken (gold standard).
func BenchmarkGhostCountVsTiktoken(b *testing.B) {
	testTexts := []struct {
		name string
		text string
	}{
		{"short_code", "func foo() { return 42 }"},
		{"repetitive_logs", "ERROR: connection failed\nERROR: connection failed\nERROR: connection failed\nERROR: connection failed\n"},
		{"json_data", `{"users": [{"id": 1, "name": "Alice"}, {"id": 2, "name": "Bob"}]}`},
		{"english_prose", "The quick brown fox jumps over the lazy dog. Pack my box with five dozen liquor jugs."},
		{"code_snippet", "func calculateTotal(items []Item) float64 {\n\ttotal := 0.0\n\tfor _, i := range items {\n\t\ttotal += i.Price\n\t}\n\treturn total\n}"},
		{"mixed_content", "user: Hello\nassistant: Hi! How can I help?\nuser: Show me users\nassistant: Here are the users: [list]"},
	}

	// Pre-load tiktoken encoder
	encoder, err := tiktoken.EncodingForModel("gpt-4")
	if err != nil {
		b.Fatal("failed to load tiktoken: ", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for _, tc := range testTexts {
		b.Run(tc.name, func(b *testing.B) {
			// GhostCount estimation
			est := NewEstimator()

			for i := 0; i < b.N; i++ {
				_ = est.Estimate(tc.text)
			}

			// Tiktoken (gold standard) - once to show comparison
			tokens := encoder.Encode(tc.text, nil, nil)
			_ = len(tokens)
		})
	}
}

// BenchmarkGhostCountAccuracy tests accuracy vs tiktoken on varied content.
func BenchmarkGhostCountAccuracy(b *testing.B) {
	encoder, err := tiktoken.EncodingForModel("gpt-4")
	if err != nil {
		b.Fatal("failed to load tiktoken: ", err)
	}

	est := NewEstimator()

	testCases := []string{
		"The quick brown fox jumps over the lazy dog.",
		"func main() { println(\"hello world\") }",
		`{"key": "value", "number": 42, "array": [1, 2, 3]}`,
		"ERROR 2026-01-29 Connection pool exhausted\n" +
			"ERROR 2026-01-29 Connection pool exhausted\n" +
			"ERROR 2026-01-29 Connection pool exhausted\n" +
			"ERROR 2026-01-29 Connection pool exhausted\n",
		"System: You are helpful.\nUser: Hi\nAssistant: Hello!\nUser: How are you?\nAssistant: Good!",
	}

	b.ReportMetric(float64(len(testCases)), "test_cases")

	for _, text := range testCases {
		tokens := encoder.Encode(text, nil, nil)
		tiktokenCount := len(tokens)

		gcResult := est.Estimate(text)
		gcCount := gcResult.RealTokens // Use RealTokens (not GhostTokens) for comparison

		accuracy := 1.0 - float64(abs(gcCount-tiktokenCount))/float64(tiktokenCount)
		b.Run(text[:min(30, len(text))]+"...", func(b *testing.B) {
			b.ReportMetric(float64(tiktokenCount), "tiktoken_tokens")
			b.ReportMetric(float64(gcCount), "ghostcount_tokens")
			b.ReportMetric(accuracy*100, "accuracy_%")
		})
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
