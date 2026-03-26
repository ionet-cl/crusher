// Package agent provides debug utilities for AI debugging with full transparency.
package agent

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// ── ANSI Color Constants ──────────────────────────────────────────────

const (
	_RESET     = "\033[0m"
	_BOLD      = "\033[1m"
	_DIM       = "\033[2m"
	_ITALIC    = "\033[3m"
	_UNDERLINE = "\033[4m"

	_CYAN    = "\033[38;5;75m"
	_GREEN   = "\033[38;5;78m"
	_YELLOW  = "\033[38;5;220m"
	_RED     = "\033[38;5;196m"
	_MAGENTA = "\033[38;5;176m"
	_WHITE   = "\033[97m"
	_GRAY    = "\033[38;5;245m"
	_L_GRAY  = "\033[38;5;250m"
)

// Status icons for visual clarity
const (
	_ICON_SUCCESS = _GREEN + "✓" + _RESET
	_ICON_ERROR   = _RED + "✗" + _RESET
	_ICON_WARN    = _YELLOW + "⚠" + _RESET
	_ICON_INFO    = _CYAN + "●" + _RESET
	_ICON_THINK   = _MAGENTA + "◆" + _RESET
	_ICON_TOOL    = _YELLOW + "▶" + _RESET
	_ICON_TOKEN   = _CYAN + "◇" + _RESET
	_ICON_COMPACT = _RED + "⚡" + _RESET
	_ICON_CLOCK   = _CYAN + "⏱" + _RESET
)

// ── Verbosity Levels ──────────────────────────────────────────────

// VerbosityLevel defines how much detail to show in debug output.
type VerbosityLevel int

const (
	VerbosityMinimal VerbosityLevel = iota // Only Pensamientos + Respuesta
	VerbosityNormal                        // + Tool calls
	VerbosityFull                          // Everything including audit trail
	VerbosityTokens                        // Only context bar
	VerbosityRaw                           // Extreme debugging: unredacted, everything visible
)

func (v VerbosityLevel) String() string {
	switch v {
	case VerbosityMinimal:
		return "minimal"
	case VerbosityNormal:
		return "normal"
	case VerbosityFull:
		return "full"
	case VerbosityTokens:
		return "tokens"
	case VerbosityRaw:
		return "raw"
	default:
		return "unknown"
	}
}

// ParseVerbosity parses a verbosity string like "minimal", "normal", "full", "tokens", "raw".
func ParseVerbosity(s string) VerbosityLevel {
	switch strings.ToLower(s) {
	case "minimal":
		return VerbosityMinimal
	case "normal":
		return VerbosityNormal
	case "full":
		return VerbosityFull
	case "tokens":
		return VerbosityTokens
	case "raw":
		return VerbosityRaw
	default:
		return VerbosityNormal
	}
}

// ── DebugConfig ──────────────────────────────────────────────

// DebugConfig holds configuration for debug output.
type DebugConfig struct {
	Verbosity         VerbosityLevel
	ShowPensamientos   bool
	ShowRespuesta     bool
	ShowToolCalls     bool
	ShowContextBar    bool
	ShowTimeline      bool
	ShowAuditTrail    bool
	ShowGhostCompact  bool
	ShowTokenUsage    bool
	ShowState         bool
	RawMode           bool // When true, show unredacted headers and full details
}

// DefaultDebugConfig returns a config with all debug features enabled.
func DefaultDebugConfig() DebugConfig {
	return DebugConfig{
		Verbosity:        VerbosityNormal,
		ShowPensamientos:  true,
		ShowRespuesta:    true,
		ShowToolCalls:    true,
		ShowContextBar:   true,
		ShowTimeline:     true,
		ShowAuditTrail:   true,
		ShowGhostCompact: true,
		ShowTokenUsage:   true,
		ShowState:        true,
	}
}

// MinimalDebugConfig returns a minimal config (Pensamientos + Respuesta only).
func MinimalDebugConfig() DebugConfig {
	return DebugConfig{
		Verbosity:        VerbosityMinimal,
		ShowPensamientos:  true,
		ShowRespuesta:    true,
		ShowToolCalls:    false,
		ShowContextBar:   true,
		ShowTimeline:     false,
		ShowAuditTrail:   false,
		ShowGhostCompact: false,
		ShowTokenUsage:   false,
		ShowState:        false,
	}
}

// TokensDebugConfig returns a tokens-only config.
func TokensDebugConfig() DebugConfig {
	return DebugConfig{
		Verbosity:        VerbosityTokens,
		ShowPensamientos:  false,
		ShowRespuesta:    false,
		ShowToolCalls:    false,
		ShowContextBar:   true,
		ShowTimeline:     false,
		ShowAuditTrail:   false,
		ShowGhostCompact: false,
		ShowTokenUsage:   false,
		ShowState:        false,
	}
}

// FullDebugConfig returns a config with absolutely everything enabled.
func FullDebugConfig() DebugConfig {
	return DebugConfig{
		Verbosity:        VerbosityFull,
		ShowPensamientos:  true,
		ShowRespuesta:    true,
		ShowToolCalls:    true,
		ShowContextBar:   true,
		ShowTimeline:     true,
		ShowAuditTrail:   true,
		ShowGhostCompact: true,
		ShowTokenUsage:   true,
		ShowState:        true,
	}
}

// DebugConfigFromVerbosity returns the appropriate DebugConfig based on verbosity string.
func DebugConfigFromVerbosity(verbosity string) DebugConfig {
	switch verbosity {
	case "minimal":
		return MinimalDebugConfig()
	case "full":
		return FullDebugConfig()
	case "tokens":
		return TokensDebugConfig()
	case "raw":
		return RawDebugConfig()
	default: // "normal"
		return DefaultDebugConfig()
	}
}

// RawDebugConfig returns a config with absolutely everything enabled plus unredacted output.
func RawDebugConfig() DebugConfig {
	cfg := FullDebugConfig()
	cfg.Verbosity = VerbosityRaw
	cfg.RawMode = true
	return cfg
}

// ── Timeline Event ──────────────────────────────────────────────

// TimelineEvent represents a timestamped event in the debug timeline.
type TimelineEvent struct {
	Timestamp time.Time
	Type      string
	Message   string
	Duration  time.Duration
	Success   bool
}

// ── AIDebugger ──────────────────────────────────────────────

// AIDebugger provides structured debug output for AI operations.
type AIDebugger struct {
	config         DebugConfig
	output         *strings.Builder
	thinkBuffer    *strings.Builder
	events         []TimelineEvent
	contextPct     float64
	contextUsed    int
	contextMax     int
	sessionID      string
	thinkStart    time.Time
	respStart     time.Time
	toolCount     int
	typewriterMode bool
	pensamientosCollapsed bool
}

// NewAIDebugger creates a new AI debugger.
func NewAIDebugger(config DebugConfig) *AIDebugger {
	return &AIDebugger{
		config:      config,
		output:      &strings.Builder{},
		thinkBuffer: &strings.Builder{},
		events:      make([]TimelineEvent, 0),
	}
}

// SetPensamientosCollapsed toggles whether pensamientos are collapsed.
func (d *AIDebugger) SetPensamientosCollapsed(collapsed bool) {
	d.pensamientosCollapsed = collapsed
}

// IsPensamientosCollapsed returns whether pensamientos are currently collapsed.
func (d *AIDebugger) IsPensamientosCollapsed() bool {
	return d.pensamientosCollapsed
}

// ── Context Bar (Live Token Counter) ──────────────────────────────────────────

// PrintContextBar prints the live context usage bar.
// This should be called periodically during streaming.
func (d *AIDebugger) PrintContextBar() {
	if !d.config.ShowContextBar {
		return
	}
	barWidth := 20
	filledBars := int(float64(barWidth) * d.contextPct / 100)
	emptyBars := barWidth - filledBars

	// Color based on usage level
	barColor := _GREEN
	if d.contextPct > 75 {
		barColor = _YELLOW
	}
	if d.contextPct > 90 {
		barColor = _RED
	}

	bar := barColor + strings.Repeat("█", filledBars) + _DIM + strings.Repeat("░", emptyBars) + _RESET

	d.output.WriteString(fmt.Sprintf("\r  %s %s %s %.0f%% (%d/%d)    ",
		_ICON_TOKEN,
		_GRAY+"Context:"+_RESET,
		bar,
		d.contextPct,
		d.contextUsed,
		d.contextMax))
}

// UpdateContext updates the context usage for the live bar.
func (d *AIDebugger) UpdateContext(used, max int) {
	d.contextUsed = used
	d.contextMax = max
	if max > 0 {
		d.contextPct = float64(used) / float64(max) * 100
	}
}

// ── Timeline ──────────────────────────────────────────────

// AddTimelineEvent adds an event to the debug timeline.
func (d *AIDebugger) AddTimelineEvent(eventType, message string, success bool) {
	if !d.config.ShowTimeline {
		return
	}
	d.events = append(d.events, TimelineEvent{
		Timestamp: time.Now(),
		Type:      eventType,
		Message:   message,
		Success:   success,
	})
}

// AddTimelineEventWithDuration adds an event with a duration.
func (d *AIDebugger) AddTimelineEventWithDuration(eventType, message string, duration time.Duration, success bool) {
	if !d.config.ShowTimeline {
		return
	}
	d.events = append(d.events, TimelineEvent{
		Timestamp: time.Now(),
		Type:      eventType,
		Message:   message,
		Duration:  duration,
		Success:   success,
	})
}

// PrintTimeline prints all collected timeline events.
func (d *AIDebugger) PrintTimeline() {
	if !d.config.ShowTimeline || len(d.events) == 0 {
		return
	}
	d.Header("TIMELINE")
	for _, e := range d.events {
		icon := _ICON_SUCCESS
		if !e.Success {
			icon = _ICON_ERROR
		}
		durationStr := ""
		if e.Duration > 0 {
			durationStr = fmt.Sprintf(" (%.3fs)", e.Duration.Seconds())
		}
		d.output.WriteString(fmt.Sprintf("  %s %s %s%s\n",
			icon,
			e.Timestamp.Format("15:04:05"),
			_GRAY+e.Message+_RESET,
			durationStr))
	}
}

// ── Pensamientos (Thoughts) Streaming ──────────────────────────────────────────

// StartPensamientos marks the start of thinking stream.
func (d *AIDebugger) StartPensamientos() {
	if !d.config.ShowPensamientos {
		return
	}
	d.thinkStart = time.Now()
	d.thinkBuffer.Reset()
	d.Header(fmt.Sprintf("Pensamientos [%s]", d.thinkStart.Format("15:04:05")))
}

// PrintPensamientoChunk prints a chunk of thinking content inline (for streaming).
// In typewriter mode, this overwrites the same line instead of appending.
// When collapsed, shows a minimal indicator instead of full content.
func (d *AIDebugger) PrintPensamientoChunk(content string) {
	if !d.config.ShowPensamientos {
		return
	}
	if d.pensamientosCollapsed {
		// Show minimal indicator when collapsed
		d.output.WriteString(fmt.Sprintf("%s◇%s", _MAGENTA, _RESET))
	} else {
		if d.typewriterMode {
			// Overwrite same line (typewriter effect)
			clearLine := "\r" + strings.Repeat(" ", 120) + "\r"
			d.output.WriteString(clearLine)
		}
		d.output.WriteString(fmt.Sprintf("%s", _MAGENTA+content+_RESET))
	}
	d.thinkBuffer.WriteString(content)
}

// PrintPensamientoLine prints a full line of thinking content.
func (d *AIDebugger) PrintPensamientoLine(line string) {
	if !d.config.ShowPensamientos {
		return
	}
	d.output.WriteString(fmt.Sprintf("  %s %s%s%s\n", _ICON_THINK, _MAGENTA, line, _RESET))
	d.thinkBuffer.WriteString(line)
}

// EndPensamientos marks the end of thinking and prints duration.
func (d *AIDebugger) EndPensamientos() {
	if !d.config.ShowPensamientos {
		return
	}
	if d.thinkStart.IsZero() {
		return
	}
	duration := time.Since(d.thinkStart)
	d.output.WriteString(fmt.Sprintf("\n  %s %s (%.3fs)\n", _ICON_CLOCK, _GRAY+"Pensamiento completado"+_RESET, duration.Seconds()))
}

// SetTypewriterMode enables or disables typewriter mode for streaming.
func (d *AIDebugger) SetTypewriterMode(enabled bool) {
	d.typewriterMode = enabled
}

// ── Respuesta ──────────────────────────────────────────────

// StartRespuesta marks the start of response output.
func (d *AIDebugger) StartRespuesta() {
	if !d.config.ShowRespuesta {
		return
	}
	d.respStart = time.Now()
	d.Header(fmt.Sprintf("Respuesta [%s]", d.respStart.Format("15:04:05")))
}

// EndRespuesta marks the end of response output.
func (d *AIDebugger) EndRespuesta() {
	if !d.config.ShowRespuesta {
		return
	}
	if d.respStart.IsZero() {
		return
	}
	duration := time.Since(d.respStart)
	d.output.WriteString(fmt.Sprintf("\n  %s %s (%.3fs)\n", _ICON_CLOCK, _GRAY+"Respuesta completada"+_RESET, duration.Seconds()))
}

// ── Tool Calls ──────────────────────────────────────────────

// PrintToolCall prints a tool call with styled output.
func (d *AIDebugger) PrintToolCall(name string, input map[string]interface{}, output string, err error) {
	if !d.config.ShowToolCalls {
		return
	}
	d.toolCount++
	d.AddTimelineEvent("TOOL", fmt.Sprintf("[%d] %s", d.toolCount, name), err == nil)

	icon := _ICON_TOOL
	if err != nil {
		icon = _ICON_ERROR
	}

	d.output.WriteString(fmt.Sprintf("\n%s %s%s%s #%d: %s\n",
		_MAGENTA+"┌─"+_RESET,
		icon,
		_BOLD+name+_RESET,
		_MAGENTA+" ──────────────────────────────"+_RESET,
		d.toolCount,
		_GRAY+time.Now().Format("15:04:05")+_RESET))

	inputJSON, _ := json.MarshalIndent(input, "  ", "  ")
	d.output.WriteString(fmt.Sprintf("%s  %sInput:\n%s%s\n",
		_MAGENTA+"│"+_RESET,
		_GRAY,
		indentString(string(inputJSON), "  │   "),
		_RESET))

	if err != nil {
		d.output.WriteString(fmt.Sprintf("%s  %sError: %s%s\n",
			_MAGENTA+"│"+_RESET,
			_RED,
			err.Error(),
			_RESET))
		d.output.WriteString(fmt.Sprintf("%s└─ %s%ds%s\n",
			_MAGENTA,
			_RED,
			d.toolCount,
			strings.Repeat("─", 30)))
	} else {
		d.output.WriteString(fmt.Sprintf("%s  %sOutput: %s%s\n",
			_MAGENTA+"│"+_RESET,
			_GREEN,
			truncateString(output, 200),
			_RESET))
		d.output.WriteString(fmt.Sprintf("%s└─ %s%ds%s\n",
			_MAGENTA,
			_GREEN,
			d.toolCount,
			strings.Repeat("─", 30)))
	}
}

// PrintToolStart prints just the tool start (for streaming tools).
func (d *AIDebugger) PrintToolStart(name string) {
	if !d.config.ShowToolCalls {
		return
	}
	d.toolCount++
	d.AddTimelineEvent("TOOL_START", fmt.Sprintf("[%d] %s started", d.toolCount, name), true)
	d.output.WriteString(fmt.Sprintf("\n%s %s%s #%d: %s\n",
		_MAGENTA+"┌─"+_RESET,
		_YELLOW+"▶"+_RESET,
		_BOLD+name+_RESET,
		d.toolCount,
		_GRAY+"(ejecutando...)"+_RESET))
}

// PrintToolEnd prints just the tool end (for streaming tools).
func (d *AIDebugger) PrintToolEnd(name string, duration time.Duration, err error) {
	if !d.config.ShowToolCalls {
		return
	}
	success := err == nil
	d.AddTimelineEventWithDuration("TOOL_END", fmt.Sprintf("[%d] %s", d.toolCount, name), duration, success)

	if err != nil {
		d.output.WriteString(fmt.Sprintf("%s└─ %s%s #%d: %s (%.3fs)\n",
			_MAGENTA,
			_RED,
			name,
			d.toolCount,
			truncateString(err.Error(), 50),
			duration.Seconds()))
	} else {
		d.output.WriteString(fmt.Sprintf("%s└─ %s%s #%d: %s (%.3fs)\n",
			_MAGENTA,
			_GREEN,
			name,
			d.toolCount,
			"completado",
			duration.Seconds()))
	}
}

// ── Ghost Compact Alert ──────────────────────────────────────────────

// PrintGhostCompactAlert prints a prominent ghost compact alert.
func (d *AIDebugger) PrintGhostCompactAlert(beforeTokens, afterTokens int, err error) {
	if !d.config.ShowGhostCompact {
		return
	}
	d.AddTimelineEvent("GHOST_COMPACT", fmt.Sprintf("Tokens: %d → %d", beforeTokens, afterTokens), err == nil)

	removed := beforeTokens - afterTokens
	reductionPct := 0.0
	if beforeTokens > 0 {
		reductionPct = float64(removed) / float64(beforeTokens) * 100
	}

	// Build visual bars
	barWidth := 15
	beforeBars := barWidth
	afterBars := 0
	if beforeTokens > 0 {
		afterBars = int(float64(barWidth) * float64(afterTokens) / float64(beforeTokens))
		beforeBars = barWidth - afterBars
	}

	d.output.WriteString(fmt.Sprintf(`
%s ╔═══════════════════════════════════════════════════════════════╗%s
%s ║%s  %s⚡ GHOST COMPACT%s                                     %s║%s
%s ╠═══════════════════════════════════════════════════════════════╣%s
%s ║%s  Antes:  %s%s%s  %d tokens                        %s║%s
%s ║%s  Después: %s%s%s  %d tokens                        %s║%s
%s ║%s  %s▲ %.0f%% reduction%s (%d tokens saved)                   %s║%s
%s ╚═══════════════════════════════════════════════════════════════╝%s
`,
		_RED, _RESET,
		_RED, _RESET, _BOLD, _RESET, _RED, _RESET,
		_RED, _RESET,
		_RED, _RESET, _RED, strings.Repeat("█", beforeBars), _RESET, beforeTokens, _RED, _RESET,
		_RED, _RESET, _GREEN, strings.Repeat("█", afterBars), _RESET, afterTokens, _GREEN, _RESET,
		_RED, _RESET, _GREEN, reductionPct, _GREEN, removed, _RED, _RESET,
		_RED, _RESET))

	if err != nil {
		d.output.WriteString(fmt.Sprintf("  %s Error: %s\n", _ICON_ERROR, err.Error()))
	}
}

// ── Provider Error ──────────────────────────────────────────────

// ProviderErrorWrapper wraps fantasy.ProviderError for debug display.
type ProviderErrorWrapper struct {
	Title             string
	Message           string
	StatusCode        int
	ContextTooLarge   bool
	Retryable         bool
	ContextUsedTokens int
	ContextMaxTokens  int
}

// PrintProviderError prints a provider error with full details.
func (d *AIDebugger) PrintProviderError(sessionID string, err *ProviderErrorWrapper) {
	if err == nil {
		return
	}
	d.Header("PROVIDER ERROR")
	d.KV("SessionID", sessionID)
	d.KV("Title", err.Title)
	d.KV("Message", err.Message)
	d.KV("StatusCode", err.StatusCode)

	if err.ContextTooLarge {
		d.KVWarn("ContextTooLarge", "Context window exceeded")
		d.KV("ContextUsed", fmt.Sprintf("%d / %d tokens (%.1f%%)",
			err.ContextUsedTokens, err.ContextMaxTokens,
			float64(err.ContextUsedTokens)/float64(err.ContextMaxTokens)*100))
	}

	if err.Retryable {
		d.KVInfo("Retryable", "This error may be resolved by retrying")
	} else {
		d.KV("Retryable", "No")
	}
}

// ── Header/Section Methods ──────────────────────────────────────────────

// Header prints a section header with brand styling.
func (d *AIDebugger) Header(section string) {
	d.output.WriteString(fmt.Sprintf("\n%s %s %s\n", _CYAN+"═══"+_RESET, _BOLD+section+_RESET, _DIM+strings.Repeat("═", max(0, 50-len(section)-4))+_RESET))
}

// SubHeader prints a subsection header.
func (d *AIDebugger) SubHeader(section string) {
	d.output.WriteString(fmt.Sprintf("\n%s %s %s\n", _MAGENTA+"───"+_RESET, _BOLD+section+_RESET, _DIM+"───"+_RESET))
}

// KV prints a key-value pair.
func (d *AIDebugger) KV(key string, value interface{}) {
	d.output.WriteString(fmt.Sprintf("  %-30s %v\n", _GRAY+key+":"+_RESET, value))
}

// KVSuccess prints a key-value pair with success styling.
func (d *AIDebugger) KVSuccess(key string, value interface{}) {
	d.output.WriteString(fmt.Sprintf("  %s %-28s %s\n", _ICON_SUCCESS, _GRAY+key+":"+_RESET, _GREEN+fmt.Sprintf("%v", value)+_RESET))
}

// KVError prints a key-value pair with error styling.
func (d *AIDebugger) KVError(key string, value interface{}) {
	d.output.WriteString(fmt.Sprintf("  %s %-28s %s\n", _ICON_ERROR, _GRAY+key+":"+_RESET, _RED+fmt.Sprintf("%v", value)+_RESET))
}

// KVWarn prints a key-value pair with warning styling.
func (d *AIDebugger) KVWarn(key string, value interface{}) {
	d.output.WriteString(fmt.Sprintf("  %s %-28s %s\n", _ICON_WARN, _GRAY+key+":"+_RESET, _YELLOW+fmt.Sprintf("%v", value)+_RESET))
}

// KVInfo prints a key-value pair with info styling.
func (d *AIDebugger) KVInfo(key string, value interface{}) {
	d.output.WriteString(fmt.Sprintf("  %s %-28s %s\n", _ICON_INFO, _GRAY+key+":"+_RESET, _CYAN+fmt.Sprintf("%v", value)+_RESET))
}

// Separator prints a separator line.
func (d *AIDebugger) Separator() {
	d.output.WriteString(_DIM + "  " + strings.Repeat("─", 58) + _RESET + "\n")
}

// ── Legacy/Compatibility Methods ──────────────────────────────────────────────

// PrintBanner prints an initial banner for AI debug mode.
func (d *AIDebugger) PrintBanner(sessionID string, model string) {
	d.sessionID = sessionID
	d.output.WriteString(fmt.Sprintf("\n%s ╔══════════════════════════════════════════════════════════╗%s\n%s ║%s  CRUSHER AI DEBUG  ║%s\n%s ╠══════════════════════════════════════════════════════════╣%s\n%s ║  Session: %-8s  Model: %s%s\n%s ║  Type 'quit' to exit  •  'clear' to clear screen           ║%s\n%s ╚══════════════════════════════════════════════════════════╝%s\n",
		_CYAN, _RESET,
		_CYAN, _RESET,
		_CYAN, _RESET,
		_CYAN, _RESET, sessionID[:8], _CYAN, _WHITE+model+_RESET,
		_CYAN, _RESET,
		_CYAN, _RESET))
}

// PrintAuditEntry prints a recovery audit entry with full detail.
func (d *AIDebugger) PrintAuditEntry(entry AuditEntry) {
	if !d.config.ShowAuditTrail {
		return
	}
	d.SubHeader("RECOVERY AUDIT ENTRY")
	if entry.Success {
		d.KVSuccess("Success", "Recovery succeeded")
	} else {
		d.KVError("Success", "Recovery failed")
	}
	d.KV("SessionID", entry.SessionID)
	d.KV("Timestamp", entry.Timestamp.Format("15:04:05.000"))
	d.KV("Action", entry.Action)
	d.KV("Strategy", GetStrategyName(entry.Strategy))
	d.KV("Error", truncateString(entry.Error, 60))
	d.KV("Details", truncateString(entry.Details, 80))
}

// PrintAllAuditTrail prints the entire audit trail as a visual table.
func (d *AIDebugger) PrintAllAuditTrail(entries []AuditEntry) {
	if !d.config.ShowAuditTrail {
		return
	}
	d.Header("RECOVERY AUDIT TRAIL")
	if len(entries) == 0 {
		d.output.WriteString(_GRAY + "  (no audit entries)\n" + _RESET)
		return
	}

	// Table header
	d.output.WriteString(fmt.Sprintf("  %s  %-12s %-18s %-10s %s\n",
		_DIM+"#"+
			_RESET, _GRAY+"TIME"+_RESET, _GRAY+"ACTION"+_RESET, _GRAY+"STRATEGY"+_RESET, _GRAY+"STATUS"+_RESET))
	d.output.WriteString(_DIM + "  " + strings.Repeat("─", 65) + _RESET + "\n")

	for i, entry := range entries {
		statusIcon := _ICON_SUCCESS
		statusColor := _GREEN
		if !entry.Success {
			statusIcon = _ICON_ERROR
			statusColor = _RED
		}
		statusStr := fmt.Sprintf("%s %s", statusIcon, entry.Action)

		d.output.WriteString(fmt.Sprintf("  %s%2d%s  %-12s %-18s %-10s %s\n",
			_CYAN, i+1, _RESET,
			entry.Timestamp.Format("15:04:05"),
			truncateString(entry.Action, 17),
			truncateString(GetStrategyName(entry.Strategy), 10),
			statusColor+statusStr+_RESET))
	}
}

// PrintTokenUsage prints token usage with a visual progress bar.
func (d *AIDebugger) PrintTokenUsage(promptTokens, completionTokens, totalTokens, contextWindow int) {
	if !d.config.ShowTokenUsage {
		return
	}
	d.Header("TOKEN USAGE")

	// Visual progress bar
	barWidth := 40
	usagePct := 0.0
	if contextWindow > 0 {
		usagePct = float64(totalTokens) / float64(contextWindow) * 100
	}
	filledBars := int(float64(barWidth) * usagePct / 100)
	emptyBars := barWidth - filledBars

	// Color based on usage level
	barColor := _GREEN
	if usagePct > 75 {
		barColor = _YELLOW
	}
	if usagePct > 90 {
		barColor = _RED
	}

	bar := barColor + strings.Repeat("█", filledBars) + _DIM + strings.Repeat("░", emptyBars) + _RESET
	d.output.WriteString(fmt.Sprintf("  %s %s %s %s %.1f%%\n", _ICON_TOKEN, _GRAY+"Context:"+_RESET, bar, _GRAY, usagePct))

	// Token breakdown
	d.output.WriteString(fmt.Sprintf("  %s %-15s %s %d\n", _ICON_INFO, _GRAY+"Prompt:"+_RESET, _CYAN, promptTokens))
	d.output.WriteString(fmt.Sprintf("  %s %-15s %s %d\n", _ICON_INFO, _GRAY+"Completion:"+_RESET, _CYAN, completionTokens))
	d.output.WriteString(fmt.Sprintf("  %s %-15s %s %d\n", _ICON_THINK, _GRAY+"Total:"+_RESET, _WHITE+_BOLD, totalTokens))
	d.output.WriteString(fmt.Sprintf("  %s %-15s %d\n", _ICON_TOKEN, _GRAY+"Remaining:"+_RESET, contextWindow-totalTokens))
}

// PrintSummary prints a summary of all debug information.
func (d *AIDebugger) PrintSummary(entries []AuditEntry, totalRetries int) {
	d.Header("DEBUG SUMMARY")

	if len(entries) > 0 {
		successCount := 0
		for _, e := range entries {
			if e.Success {
				successCount++
			}
		}
		failedCount := len(entries) - successCount
		d.KV("TotalAuditEntries", len(entries))
		d.KV("TotalRecoveryAttempts", totalRetries)
		d.output.WriteString("\n")
		d.KVSuccess("SuccessfulRecoveries", successCount)
		if failedCount > 0 {
			d.KVError("FailedRecoveries", failedCount)
		} else {
			d.KV("FailedRecoveries", failedCount)
		}
	} else {
		d.KV("TotalAuditEntries", 0)
		d.KV("TotalRecoveryAttempts", totalRetries)
	}

	// Print final context bar
	if d.config.ShowContextBar && d.contextMax > 0 {
		d.output.WriteString("\n")
		d.KV("ContextUsage", fmt.Sprintf("%d/%d (%.1f%%)", d.contextUsed, d.contextMax, d.contextPct))
	}
}

// PrintSessionConfig prints session configuration.
func (d *AIDebugger) PrintSessionConfig(provider string, contextWindow int) {
	d.SubHeader("SESSION CONFIG")
	d.KV("Mode", _CYAN+"X-RAY VISION"+_RESET)
	d.KV("Verbosity", d.config.Verbosity.String())
	d.KV("Provider", _CYAN+provider+_RESET)
	d.KV("ContextWindow", contextWindow)
}

// PrintPensamientos is alias for StartPensamientos (for compatibility).
func (d *AIDebugger) PrintPensamientos() {
	d.StartPensamientos()
}

// ── String/Output Methods ──────────────────────────────────────────────

// String returns the debug output as a string.
func (d *AIDebugger) String() string {
	return d.output.String()
}

// Reset clears the debug output.
func (d *AIDebugger) Reset() {
	d.output.Reset()
	d.thinkBuffer.Reset()
	d.events = d.events[:0]
	d.contextPct = 0
	d.contextUsed = 0
	d.contextMax = 0
	d.thinkStart = time.Time{}
	d.respStart = time.Time{}
	d.toolCount = 0
}

// GetEvents returns the collected timeline events.
func (d *AIDebugger) GetEvents() []TimelineEvent {
	return d.events
}

// ── Helper Functions ──────────────────────────────────────────────

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func indentString(s, indent string) string {
	lines := strings.Split(s, "\n")
	result := ""
	for _, line := range lines {
		result += indent + line + "\n"
	}
	return result
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── Log Functions (for real-time debugging) ──────────────────────────────────────────

// LogAuditEntry logs an audit entry to slog for real-time debugging.
func LogAuditEntry(entry AuditEntry) {
	slog.Debug("audit_entry",
		"sid", entry.SessionID,
		"timestamp", entry.Timestamp.Format(time.RFC3339Nano),
		"action", entry.Action,
		"strategy", GetStrategyName(entry.Strategy),
		"error", entry.Error,
		"success", entry.Success,
		"details", entry.Details,
	)
}

// LogGhostCompact logs ghost compact operations in real-time.
func LogGhostCompact(sessionID string, beforeTokens, afterTokens int, err error) {
	if err != nil {
		slog.Info("ghost_compact",
			"sid", sessionID,
			"before_tokens", beforeTokens,
			"after_tokens", afterTokens,
			"tokens_removed", beforeTokens-afterTokens,
			"status", "error",
			"error", err.Error(),
		)
	} else {
		slog.Info("ghost_compact",
			"sid", sessionID,
			"before_tokens", beforeTokens,
			"after_tokens", afterTokens,
			"tokens_removed", beforeTokens-afterTokens,
			"reduction_pct", fmt.Sprintf("%.1f%%", float64(beforeTokens-afterTokens)/float64(beforeTokens)*100),
			"status", "success",
		)
	}
}

// LogTokenUsage logs token usage in real-time.
func LogTokenUsage(sessionID string, promptTokens, completionTokens, totalTokens, contextWindow int) {
	slog.Debug("token_usage",
		"sid", sessionID,
		"prompt_tokens", promptTokens,
		"completion_tokens", completionTokens,
		"total_tokens", totalTokens,
		"context_window", contextWindow,
		"usage_pct", fmt.Sprintf("%.1f%%", float64(totalTokens)/float64(contextWindow)*100),
		"remaining", contextWindow-totalTokens,
	)
}

// GetCircuitBreakerRetryCount returns the current retry count for a session.
func GetCircuitBreakerRetryCount(sessionID string) int {
	return circuitBreakerRetryCount[sessionID]
}

// CircuitBreakerState holds the current state of the circuit breaker for display.
type CircuitBreakerState struct {
	Enabled    bool
	Retries    int
	MaxRetries int
}

// GetCircuitBreakerState returns the current circuit breaker state for a session.
func GetCircuitBreakerState(sessionID string, enabled bool) CircuitBreakerState {
	return CircuitBreakerState{
		Enabled:    enabled,
		Retries:    GetCircuitBreakerRetryCount(sessionID),
		MaxRetries: MaxCircuitBreakerRetries(),
	}
}

// PrintCircuitBreakerState prints the circuit breaker state in debug output.
func (d *AIDebugger) PrintCircuitBreakerState(state CircuitBreakerState) {
	if !d.config.ShowState {
		return
	}
	d.Header("CIRCUIT BREAKER STATE")
	d.KV("Enabled", state.Enabled)
	if state.Enabled {
		remaining := state.MaxRetries - state.Retries
		statusColor := _GREEN
		if remaining <= 0 {
			statusColor = _RED
		} else if remaining == 1 {
			statusColor = _YELLOW
		}
		d.output.WriteString(fmt.Sprintf("  %s %-28s %s%d / %d%s\n",
			_ICON_INFO,
			_GRAY+"Retries:"+_RESET,
			statusColor,
			state.Retries,
			state.MaxRetries,
			_RESET))
		d.output.WriteString(fmt.Sprintf("  %s %-28s %s%d remaining%s\n",
			_ICON_INFO,
			_GRAY+"Remaining:"+_RESET,
			statusColor,
			remaining,
			_RESET))
	}
}
