package providers

import (
	"context"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/openai"
	"github.com/charmbracelet/crush/internal/log"
)

// openAIBuilder implements Builder for OpenAI.
type openAIBuilder struct{}

func (b *openAIBuilder) Build(ctx context.Context, cfg ProviderConfig) (fantasy.Provider, error) {
	opts := []openai.Option{
		openai.WithAPIKey(cfg.APIKey),
		openai.WithUseResponsesAPI(),
	}

	if cfg.Debug {
		httpClient := log.NewHTTPClient()
		opts = append(opts, openai.WithHTTPClient(httpClient))
	}

	if len(cfg.Headers) > 0 {
		opts = append(opts, openai.WithHeaders(cfg.Headers))
	}

	if cfg.BaseURL != "" {
		opts = append(opts, openai.WithBaseURL(cfg.BaseURL))
	}

	return openai.New(opts...)
}
