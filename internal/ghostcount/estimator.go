package ghostcount

import (
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"sync"
)

// TokenEstimator estimates semantic token count with redundancy detection.
type TokenEstimator interface {
	// Estimate returns the effective token count (ghost tokens).
	// High redundancy → low value, low redundancy → high value.
	Estimate(text string) TokenEstimate

	// EstimateMessages estimates total ghost tokens for a message list.
	EstimateMessages(texts []string) int

	// Reset clears the estimator's cache (for new sessions).
	Reset()
}

// estimator implements TokenEstimator using MinHash and zlib compression.
type estimator struct {
	config     Config
	minhasher  *minhasher
	cache      *jaccardCache
	previewLen int
	mu         sync.RWMutex
}

// NewEstimator creates a new TokenEstimator with default config.
func NewEstimator() TokenEstimator {
	return NewEstimatorWithConfig(DefaultConfig())
}

// NewEstimatorWithConfig creates a new TokenEstimator with custom config.
func NewEstimatorWithConfig(cfg Config) TokenEstimator {
	if cfg.Permutations <= 0 {
		cfg.Permutations = 64
	}
	if cfg.CacheSize <= 0 {
		cfg.CacheSize = 20
	}
	if cfg.CacheThreshold <= 0 {
		cfg.CacheThreshold = 0.8
	}
	if cfg.CharsPerToken <= 0 {
		cfg.CharsPerToken = 3.5
	}
	return &estimator{
		config:     cfg,
		minhasher:  newMinhasher(cfg.Permutations),
		cache:      newJaccardCache(cfg.CacheSize),
		previewLen: 50,
	}
}

func (e *estimator) Estimate(text string) TokenEstimate {
	if text == "" {
		return TokenEstimate{RealTokens: 0, GhostTokens: 0, Value: 1.0}
	}

	// 1. Base estimation using zlib compression ratio
	rawBytes := len(text)
	ratio := compressRatio(text)

	realTokens := int(float64(rawBytes) / e.config.CharsPerToken * ratio)
	if realTokens < 1 {
		realTokens = 1
	}

	// 2. MinHash signature
	signature := e.minhasher.signature(text)

	// 3. Jaccard similarity with cache
	e.mu.Lock()
	maxSimilarity := e.cache.maxSimilarity(signature, e.minhasher)
	e.mu.Unlock()

	// 4. Calculate semantic value
	// Formula: (1 - similarity * 0.7) * ratio
	infoValue := (1.0 - (maxSimilarity * 0.7)) * ratio
	if infoValue < 0.1 {
		infoValue = 0.1
	}
	if infoValue > 1.0 {
		infoValue = 1.0
	}

	// 5. Ghost tokens
	ghostTokens := int(float64(realTokens) * infoValue)
	if ghostTokens < 1 {
		ghostTokens = 1
	}

	// 6. Cache if novel enough
	if maxSimilarity < e.config.CacheThreshold {
		e.mu.Lock()
		preview := text
		if len(preview) > e.previewLen {
			preview = preview[:e.previewLen]
		}
		e.cache.push(preview, signature)
		e.mu.Unlock()
	}

	return TokenEstimate{
		RealTokens:  realTokens,
		GhostTokens: ghostTokens,
		Value:       infoValue,
		Signature:   signature,
	}
}

func (e *estimator) EstimateMessages(texts []string) int {
	total := 0
	for _, text := range texts {
		est := e.Estimate(text)
		total += est.GhostTokens
		total += 6 // overhead per message
	}
	return total
}

func (e *estimator) Reset() {
	e.mu.Lock()
	e.cache.clear()
	e.mu.Unlock()
}

// minhasher generates MinHash signatures for text deduplication.
type minhasher struct {
	permutations int
	shingleSize int
}

func newMinhasher(permutations int) *minhasher {
	return &minhasher{
		permutations: permutations,
		shingleSize:  5,
	}
}

// signature returns a MinHash signature for the text.
func (m *minhasher) signature(text string) []uint64 {
	if text == "" {
		return make([]uint64, m.permutations)
	}

	// Generate shingles (n-grams of characters)
	shingles := make(map[string]struct{})
	for i := 0; i <= len(text)-m.shingleSize; i++ {
		shingles[text[i:i+m.shingleSize]] = struct{}{}
	}

	if len(shingles) == 0 {
		return make([]uint64, m.permutations)
	}

	// Generate MinHash signature
	sig := make([]uint64, m.permutations)
	for i := 0; i < m.permutations; i++ {
		minHash := uint64(^uint64(0)) // max uint64
		salt := []byte("ghost_salt_" + string(rune('a'+i)))

		for shingle := range shingles {
			h := md5.Sum(append(salt, shingle...))
			// Use first 8 bytes as uint64
			v := uint64(h[0]) | uint64(h[1])<<8 | uint64(h[2])<<16 |
				uint64(h[3])<<24 | uint64(h[4])<<32 | uint64(h[5])<<40 |
				uint64(h[6])<<48 | uint64(h[7])<<56
			if v < minHash {
				minHash = v
			}
		}
		sig[i] = minHash
	}

	return sig
}

// jaccardCache stores recent signatures for similarity comparison.
type jaccardCache struct {
	entries    []cacheEntry
	size       int
	hashValues []uint64
}

type cacheEntry struct {
	preview   string
	signature []uint64
}

func newJaccardCache(size int) *jaccardCache {
	return &jaccardCache{
		entries:    make([]cacheEntry, 0, size),
		size:       size,
		hashValues: make([]uint64, 128), // max permutations
	}
}

func (c *jaccardCache) push(preview string, signature []uint64) {
	if len(c.entries) >= c.size {
		// Remove oldest (FIFO)
		copy(c.entries, c.entries[1:])
		c.entries = c.entries[:len(c.entries)-1]
	}
	c.entries = append(c.entries, cacheEntry{preview: preview, signature: signature})
}

func (c *jaccardCache) maxSimilarity(sig []uint64, m *minhasher) float64 {
	if len(c.entries) == 0 || len(sig) == 0 {
		return 0.0
	}

	maxSim := 0.0
	for _, entry := range c.entries {
		sim := jaccardSimilarity(sig, entry.signature)
		if sim > maxSim {
			maxSim = sim
		}
	}
	return maxSim
}

func (c *jaccardCache) clear() {
	c.entries = c.entries[:0]
}

func (c *jaccardCache) len() int {
	return len(c.entries)
}

// jaccardSimilarity estimates Jaccard similarity between two signatures.
func jaccardSimilarity(sig1, sig2 []uint64) float64 {
	if len(sig1) == 0 || len(sig2) == 0 {
		return 0.0
	}
	matching := 0
	minLen := len(sig1)
	if len(sig2) < minLen {
		minLen = len(sig2)
	}
	for i := 0; i < minLen; i++ {
		if sig1[i] == sig2[i] {
			matching++
		}
	}
	return float64(matching) / float64(minLen)
}

// compressRatio returns the zlib compression ratio for text.
func compressRatio(text string) float64 {
	if len(text) == 0 {
		return 1.0
	}

	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	w.Write([]byte(text))
	w.Close()

	compressedLen := float64(buf.Len())
	rawLen := float64(len(text))

	ratio := compressedLen / rawLen
	if ratio < 0.1 {
		ratio = 0.1 // minimum 10% of original (high redundancy)
	}
	if ratio > 1.0 {
		ratio = 1.0 // no compression or expansion
	}
	return ratio
}
