package log

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// AIDebugEnabled when true makes HTTP logging output to stderr directly
// with full transparency for --ai-debug mode.
var AIDebugEnabled atomic.Bool

// RawModeEnabled when true makes HTTP logging output unredacted headers.
var RawModeEnabled atomic.Bool

// SetAIDebug enables or disables AI debug HTTP logging.
func SetAIDebug(enabled bool) {
	AIDebugEnabled.Store(enabled)
}

// SetRawMode enables or disables raw (unredacted) HTTP debug output.
func SetRawMode(enabled bool) {
	RawModeEnabled.Store(enabled)
}

// NewHTTPClient creates an HTTP client with debug logging enabled when debug mode is on.
func NewHTTPClient() *http.Client {
	return &http.Client{
		Transport: &HTTPRoundTripLogger{
			Transport: http.DefaultTransport,
		},
	}
}

// HTTPRoundTripLogger is an http.RoundTripper that logs requests and responses.
type HTTPRoundTripLogger struct {
	Transport http.RoundTripper
}

// RoundTrip implements http.RoundTripper interface with logging.
func (h *HTTPRoundTripLogger) RoundTrip(req *http.Request) (*http.Response, error) {
	var err error
	var save io.ReadCloser
	save, req.Body, err = drainBody(req.Body)
	if err != nil {
		slog.Error("HTTP request failed", "method", req.Method, "url", req.URL, "error", err)
		return nil, err
	}

	if AIDebugEnabled.Load() {
		logAIDebugRequest(req, save)
	} else if slog.Default().Enabled(req.Context(), slog.LevelDebug) {
		slog.Debug("HTTP Request", "method", req.Method, "url", req.URL, "body", bodyToString(save))
	}

	start := time.Now()
	resp, err := h.Transport.RoundTrip(req)
	duration := time.Since(start)
	if err != nil {
		slog.Error("HTTP request failed", "method", req.Method, "url", req.URL, "duration_ms", duration.Milliseconds(), "error", err)
		return nil, err
	}

	save, resp.Body, err = drainBody(resp.Body)
	if err != nil {
		slog.Error("Failed to drain response body", "error", err)
		return resp, err
	}

	if AIDebugEnabled.Load() {
		logAIDebugResponse(resp, duration, save)
	} else if slog.Default().Enabled(req.Context(), slog.LevelDebug) {
		slog.Debug("HTTP Response", "status_code", resp.StatusCode, "status", resp.Status, "headers", formatHeaders(resp.Header), "body", bodyToString(save), "content_length", resp.ContentLength, "duration_ms", duration.Milliseconds())
	}
	return resp, nil
}

func bodyToString(body io.ReadCloser) string {
	if body == nil {
		return ""
	}
	src, err := io.ReadAll(body)
	if err != nil {
		slog.Error("Failed to read body", "error", err)
		return ""
	}
	var b bytes.Buffer
	if json.Indent(&b, bytes.TrimSpace(src), "", "  ") != nil {
		return string(src)
	}
	return b.String()
}

func formatHeaders(headers http.Header) map[string][]string {
	filtered := make(map[string][]string)
	for key, values := range headers {
		lowerKey := strings.ToLower(key)
		if strings.Contains(lowerKey, "authorization") ||
			strings.Contains(lowerKey, "api-key") ||
			strings.Contains(lowerKey, "token") ||
			strings.Contains(lowerKey, "secret") {
			filtered[key] = []string{"[REDACTED]"}
		} else {
			filtered[key] = values
		}
	}
	return filtered
}

func drainBody(b io.ReadCloser) (r1, r2 io.ReadCloser, err error) {
	if b == nil || b == http.NoBody {
		return http.NoBody, http.NoBody, nil
	}
	var buf bytes.Buffer
	if _, err = buf.ReadFrom(b); err != nil {
		return nil, b, err
	}
	if err = b.Close(); err != nil {
		return nil, b, err
	}
	return io.NopCloser(&buf), io.NopCloser(bytes.NewReader(buf.Bytes())), nil
}

// AIDebug logging functions - output directly to stderr for --ai-debug mode
func logAIDebugRequest(req *http.Request, body io.ReadCloser) {
	rawMode := RawModeEnabled.Load()
	fmt.Fprintf(stderr, "\n%s═══ HTTP REQUEST ════════════════════════════════════════════%s\n", cyan, reset)
	fmt.Fprintf(stderr, "%s▶%s %s %s%s%s\n", magenta, reset, bold, req.Method, reset, cyan)
	fmt.Fprintf(stderr, "   URL: %s%s%s\n", white, req.URL.String(), reset)

	fmt.Fprintf(stderr, "   Headers:\n")
	for key, values := range req.Header {
		if !rawMode && isSensitive(key) {
			fmt.Fprintf(stderr, "      %s: %s[REDACTED]%s\n", key, red, reset)
		} else {
			fmt.Fprintf(stderr, "      %s: %s\n", key, strings.Join(values, ", "))
		}
	}

	if body != nil {
		data, _ := io.ReadAll(body)
		if len(data) > 0 {
			fmt.Fprintf(stderr, "   Body (%d bytes):\n%s\n", len(data), formatJSON(data))
		}
	}
	fmt.Fprintf(stderr, "%s═══════════════════════════════════════════════════════════════%s\n", cyan, reset)
}

func logAIDebugResponse(resp *http.Response, duration time.Duration, body io.ReadCloser) {
	statusColor := green
	if resp.StatusCode >= 500 {
		statusColor = red
	} else if resp.StatusCode >= 400 {
		statusColor = red
	} else if resp.StatusCode >= 300 {
		statusColor = yellow
	}

	fmt.Fprintf(stderr, "\n%s═══ HTTP RESPONSE ═══════════════════════════════════════════%s\n", cyan, reset)
	fmt.Fprintf(stderr, "%s◀%s %s%s%s %s(%d)%s in %s%dms%s\n",
		magenta, reset, statusColor, resp.Status, reset, cyan, resp.StatusCode, reset, yellow, duration.Milliseconds(), reset)

	fmt.Fprintf(stderr, "   Headers:\n")
	for key, values := range resp.Header {
		fmt.Fprintf(stderr, "      %s: %s\n", key, strings.Join(values, ", "))
	}

	if body != nil {
		data, _ := io.ReadAll(body)
		if len(data) > 0 {
			fmt.Fprintf(stderr, "   Body (%d bytes):\n%s\n", len(data), formatJSON(data))
		}
	}
	fmt.Fprintf(stderr, "%s═══════════════════════════════════════════════════════════════%s\n", cyan, reset)
}

func isSensitive(key string) bool {
	lower := strings.ToLower(key)
	sensitive := []string{"authorization", "api-key", "apikey", "token", "secret", "password", "key"}
	for _, s := range sensitive {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

func formatJSON(data []byte) string {
	var jsonData interface{}
	if json.Unmarshal(data, &jsonData) == nil {
		var buf bytes.Buffer
		writer := json.NewEncoder(&buf)
		writer.SetIndent("", "   ")
		writer.Encode(jsonData)
		return buf.String()
	}
	return string(data)
}

// stderr is for testing - defaults to os.Stderr
var stderr io.Writer = os.Stderr

// SetOutput sets the output writer for HTTP debug logs.
func SetOutput(w io.Writer) {
	stderr = w
}

// Color codes
const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	red     = "\033[38;5;196m"
	green   = "\033[38;5;78m"
	yellow  = "\033[38;5;220m"
	cyan    = "\033[38;5;75m"
	magenta = "\033[38;5;176m"
	white   = "\033[97m"
)
