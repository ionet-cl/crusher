// Package ghostcount provides semantic token estimation and context window management.
//
// GhostCount estimates effective token count by detecting redundancy using MinHash
// and zlib compression ratio. Unlike simple char/4 approximation, it calculates
// "ghost tokens" - the actual information value after deduplication.
//
// Architecture (decoupled from agent/message/session):
//
//	TokenEstimator     → semantic token counting
//	ContextTruncator   → head+tail truncation for tool outputs
//	MessageCompactor   → selective retention with async compaction
//
// The package is fully decoupled and has zero imports from other crush packages.
package ghostcount
