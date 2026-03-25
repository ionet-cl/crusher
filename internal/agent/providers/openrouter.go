package providers

import (
	"context"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/openrouter"
)

// openrouterBuilder implements Builder for OpenRouter.
type openrouterBuilder struct{}

func (b *openrouterBuilder) Build(ctx context.Context, cfg ProviderConfig, debug bool) (fantasy.Provider, error) {
	opts := []openrouter.Option{
		openrouter.WithAPIKey(cfg.APIKey),
	}
	if debug {
		opts = append(opts, openrouter.WithHTTPClient(NewHTTPClient()))
	}
	if len(cfg.Headers) > 0 {
		opts = append(opts, openrouter.WithHeaders(cfg.Headers))
	}
	return openrouter.New(opts...)
}
