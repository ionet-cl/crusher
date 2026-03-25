package providers

import (
	"context"
	"os"
	"strings"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
	"github.com/charmbracelet/crush/internal/log"
)

// anthropicBuilder implements Builder for Anthropic.
type anthropicBuilder struct{}

func (b *anthropicBuilder) Build(ctx context.Context, cfg ProviderConfig) (fantasy.Provider, error) {
	var opts []anthropic.Option

	switch {
	case strings.HasPrefix(cfg.APIKey, "Bearer "):
		os.Setenv("ANTHROPIC_API_KEY", "")
		cfg.Headers["Authorization"] = cfg.APIKey
	case cfg.ID == "minimax" || cfg.ID == "minimax-china":
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

	if cfg.Debug {
		opts = append(opts, anthropic.WithHTTPClient(log.NewHTTPClient()))
	}

	return anthropic.New(opts...)
}
