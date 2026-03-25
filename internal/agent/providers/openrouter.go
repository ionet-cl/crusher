package providers

import (
	"context"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/openrouter"
	"github.com/charmbracelet/crush/internal/log"
)

// openrouterBuilder implements Builder for OpenRouter.
type openrouterBuilder struct{}

func (b *openrouterBuilder) Build(ctx context.Context, cfg ProviderConfig) (fantasy.Provider, error) {
	opts := []openrouter.Option{
		openrouter.WithAPIKey(cfg.APIKey),
	}
	if cfg.Debug {
		opts = append(opts, openrouter.WithHTTPClient(log.NewHTTPClient()))
	}
	if len(cfg.Headers) > 0 {
		opts = append(opts, openrouter.WithHeaders(cfg.Headers))
	}
	return openrouter.New(opts...)
}
