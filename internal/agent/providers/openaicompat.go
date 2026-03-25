package providers

import (
	"context"
	"net/http"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/openaicompat"
	openaisdk "github.com/charmbracelet/openai-go/option"
	"github.com/charmbracelet/crush/internal/log"
)

// openaicompatBuilder implements Builder for OpenAI Compatible providers.
type openaicompatBuilder struct{}

func (b *openaicompatBuilder) Build(ctx context.Context, cfg ProviderConfig) (fantasy.Provider, error) {
	opts := []openaicompat.Option{
		openaicompat.WithBaseURL(cfg.BaseURL),
		openaicompat.WithAPIKey(cfg.APIKey),
	}

	var httpClient *http.Client
	if cfg.ID == "copilot" {
		opts = append(opts, openaicompat.WithUseResponsesAPI())
		httpClient = log.NewHTTPClient()
	} else if cfg.Debug {
		httpClient = log.NewHTTPClient()
	}
	if httpClient != nil {
		opts = append(opts, openaicompat.WithHTTPClient(httpClient))
	}

	if len(cfg.Headers) > 0 {
		opts = append(opts, openaicompat.WithHeaders(cfg.Headers))
	}

	for extraKey, extraValue := range cfg.ExtraBody {
		opts = append(opts, openaicompat.WithSDKOptions(openaisdk.WithJSONSet(extraKey, extraValue)))
	}

	return openaicompat.New(opts...)
}
