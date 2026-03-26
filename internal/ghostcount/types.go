package ghostcount

// TokenEstimate contains the result of semantic token estimation.
type TokenEstimate struct {
	RealTokens  int       // Raw character-based estimate (len / 3.5)
	GhostTokens int       // Redundancy-adjusted estimate (actual information value)
	Value      float64    // Semantic value 0.0-1.0 (1 = unique, 0 = fully redundant)
	Signature  []uint64  // MinHash signature for cache comparison
}

// Config holds configuration for the ghostcount system.
type Config struct {
	// MinHash permutations (64-128 recommended for good accuracy)
	Permutations int
	// LRU cache size for Jaccard similarity
	CacheSize int
	// Similarity threshold to cache a signature (0.8 = don't cache if >80% similar)
	CacheThreshold float64
	// Characters per token for base estimation
	CharsPerToken float64
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Permutations:  64,
		CacheSize:     20,
		CacheThreshold: 0.8,
		CharsPerToken: 3.5,
	}
}

// RetentionLevel defines message retention priority.
type RetentionLevel int

const (
	// RetainAlways messages are never truncated (system, DREA trauma)
	RetainAlways RetentionLevel = iota
	// RetainPriority messages are kept if they reference active symbols
	RetainPriority
	// RetainNormal messages are truncated from oldest first
	RetainNormal
	// RetainPrune messages are always removed first
	RetainPrune
)

// CompactionConfig holds configuration for context compaction.
type CompactionConfig struct {
	// Total context window size
	ContextWindow int
	// Max tokens reserved for response
	MaxResponseTokens int
	// Calculated: (ContextWindow - MaxResponseTokens) * 0.85
	HistoryThreshold int
	// ActiveSymbols are code symbols (functions, files) being actively edited.
	// Messages containing these symbols get RetainPriority level.
	ActiveSymbols []string
}

// NewCompactionConfig creates a CompactionConfig with derived values.
func NewCompactionConfig(contextWindow, maxResponseTokens int) CompactionConfig {
	available := contextWindow - maxResponseTokens
	if available <= 0 {
		available = contextWindow
	}
	return CompactionConfig{
		ContextWindow:     contextWindow,
		MaxResponseTokens: maxResponseTokens,
		HistoryThreshold:   available * 85 / 100,
	}
}

// Message represents a minimal message for compaction (decoupled from agent/message packages).
type Message interface {
	GetRole() string
	GetContent() string
}

// CompactionResult contains the result of context compaction.
type CompactionResult struct {
	Messages          []Message
	WasCompacted      bool
	AsyncRecommended  bool
	TokensBefore      int
	TokensAfter       int
}

// TruncationConfig holds configuration for tool output truncation.
type TruncationConfig struct {
	MaxChars   int
	HeadChars  int
	TailChars  int
	Marker    string
}

// DefaultTruncationConfig returns sensible defaults.
func DefaultTruncationConfig() TruncationConfig {
	return TruncationConfig{
		MaxChars:  2048,
		HeadChars: 800,
		TailChars: 2048 - 800 - len(TruncationMarker),
		Marker:    TruncationMarker,
	}
}

// TruncationMarker is inserted when content is truncated.
const TruncationMarker = "\n[...OUTPUT TRUNCATED...]\n"
