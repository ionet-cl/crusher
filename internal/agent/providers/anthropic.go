package providers

import (
	"context"
	"os"
	"strings"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
)

// anthropicBuilder implements Builder for Anthropic.
type anthropicBuilder struct{}

func (b *anthropicBuilder) Build(ctx context.Context, cfg ProviderConfig, debug bool) (fantasy.Provider, error) {
	var opts []anthropic.Option

	switch {
	case strings.HasPrefix(cfg.APIKey, "Bearer "):
		// NOTE: Prevent the SDK from picking up the API key from env.
		os.Setenv("ANTHROPIC_API_KEY", "")
		cfg.Headers["Authorization"] = cfg.APIKey
	case cfg.ID == "minimax" || cfg.ID == "minimax-china":
		// NOTE: Prevent the SDK from picking up the API key from env.
		os.Setenv("ANTHROPIC_API_KEY", "")
		cfg.Headers["Authorization"] = "Bearer " + cfg.APIKey
	case cfg.APIKey != "":
		opts = append(opts, anthropic.WithAPIKey(cfg.APIKey))
	}

	if len(cfg.Headers) > 0 {
		opts = append(opts, anthropic.WithHeaders(cfg.Headers))
	}

	if cfg.BaseURL != "" {
		opts = append(opts, anthropic.WithBaseURL(cfg.BaseURL))
	}

	if debug {
		opts = append(opts, anthropic.WithHTTPClient(NewHTTPClient()))
	}

	return anthropic.New(opts...)
}
