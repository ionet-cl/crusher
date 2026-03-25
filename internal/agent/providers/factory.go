// Package providers provides a factory pattern for building LLM providers.
// This decouples the coordinator from provider-specific construction logic.
//
// Supported providers: openai, anthropic, openrouter
package providers

import (
	"context"
	"fmt"
	"net/http"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/log"
)

// Factory creates provider instances.
type Factory struct {
	config *config.ConfigStore
}

// NewFactory creates a new provider factory.
func NewFactory(cfg *config.ConfigStore) *Factory {
	return &Factory{config: cfg}
}

// ProviderConfig holds the configuration needed to build a provider.
type ProviderConfig struct {
	Type        string
	ID          string
	APIKey      string
	BaseURL     string
	Headers     map[string]string
	ExtraBody   map[string]any
	ExtraParams map[string]string
	IsSubAgent  bool
	Debug       bool
}

// ErrUnsupportedProvider is returned when the provider type is not supported.
var ErrUnsupportedProvider = &unsupportedProviderError{}

type unsupportedProviderError struct{}

func (e *unsupportedProviderError) Error() string {
	return "unsupported provider type"
}

// Build routes to the appropriate builder based on provider type.
func (f *Factory) Build(ctx context.Context, cfg ProviderConfig) (fantasy.Provider, error) {
	cfg.Debug = f.config.Config().Options.Debug || log.AIDebugEnabled.Load()

	switch cfg.Type {
	case "openai":
		return (&openAIBuilder{}).Build(ctx, cfg)
	case "anthropic":
		return (&anthropicBuilder{}).Build(ctx, cfg)
	case "openrouter":
		return (&openrouterBuilder{}).Build(ctx, cfg)
	case "openaicompat", "openai-compat":
		return (&openaicompatBuilder{}).Build(ctx, cfg)
	default:
		return nil, fmt.Errorf("%w: %s (supported: openai, anthropic, openrouter, openaicompat)", ErrUnsupportedProvider, cfg.Type)
	}
}

// NewHTTPClient creates a debug HTTP client.
func NewHTTPClient() *http.Client {
	return log.NewHTTPClient()
}
