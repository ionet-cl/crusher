// Package agent provides title generation for sessions.
package agent

import (
	"context"
	"strings"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/openrouter"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/session"
)

// TitleGenerator handles session title generation.
type TitleGenerator struct {
	sessions session.Service
	messages message.Service
}

// NewTitleGenerator creates a title generator.
func NewTitleGenerator(opts TitleGeneratorOptions) *TitleGenerator {
	return &TitleGenerator{
		sessions: opts.Sessions,
		messages: opts.Messages,
	}
}

// TitleGeneratorOptions configures the title generator.
type TitleGeneratorOptions struct {
	Sessions session.Service
	Messages message.Service
}

// GenerateTitle generates a title for a session.
func (tg *TitleGenerator) GenerateTitle(ctx context.Context, sessionID, prompt string, largeModel, smallModel Model, maxOutputTokens int64, systemPromptPrefix string) {
	userPrompt := thinkTagRegex.ReplaceAllString(prompt, "")

	streamCall := fantasy.AgentStreamCall{
		Prompt: userPrompt,
		PrepareStep: func(callCtx context.Context, opts fantasy.PrepareStepFunctionOptions) (_ context.Context, prepared fantasy.PrepareStepResult, err error) {
			prepared.Messages = opts.Messages
			if systemPromptPrefix != "" {
				prepared.Messages = append([]fantasy.Message{
					fantasy.NewSystemMessage(systemPromptPrefix),
				}, prepared.Messages...)
			}
			return callCtx, prepared, nil
		},
	}

	model := smallModel
	agent := fantasy.NewAgent(model.Model,
		fantasy.WithSystemPrompt(string(titlePrompt)),
		fantasy.WithMaxOutputTokens(maxOutputTokens),
		fantasy.WithUserAgent(userAgent),
	)
	resp, err := agent.Stream(ctx, streamCall)
	if err != nil {
		model = largeModel
		agent = fantasy.NewAgent(model.Model,
			fantasy.WithSystemPrompt(string(titlePrompt)),
			fantasy.WithMaxOutputTokens(maxOutputTokens),
			fantasy.WithUserAgent(userAgent),
		)
		resp, err = agent.Stream(ctx, streamCall)
		if err != nil {
			_ = tg.sessions.Rename(ctx, sessionID, DefaultSessionName)
			return
		}
	}

	if resp == nil {
		_ = tg.sessions.Rename(ctx, sessionID, DefaultSessionName)
		return
	}

	var title string
	title = strings.ReplaceAll(resp.Response.Content.Text(), "\n", " ")
	title = thinkTagRegex.ReplaceAllString(title, "")
	title = strings.TrimSpace(title)
	if title == "" {
		title = DefaultSessionName
	}

	var openrouterCost *float64
	for _, step := range resp.Steps {
		stepCost := openrouterCostFromMetadata(step.ProviderMetadata)
		if stepCost != nil {
			newCost := *stepCost
			if openrouterCost != nil {
				newCost += *openrouterCost
			}
			openrouterCost = &newCost
		}
	}

	modelConfig := model.CatwalkCfg
	cost := modelConfig.CostPer1MInCached/1e6*float64(resp.TotalUsage.CacheCreationTokens) +
		modelConfig.CostPer1MOutCached/1e6*float64(resp.TotalUsage.CacheReadTokens) +
		modelConfig.CostPer1MIn/1e6*float64(resp.TotalUsage.InputTokens) +
		modelConfig.CostPer1MOut/1e6*float64(resp.TotalUsage.OutputTokens)

	if openrouterCost != nil {
		cost = *openrouterCost
	}

	promptTokens := resp.TotalUsage.InputTokens + resp.TotalUsage.CacheCreationTokens
	completionTokens := resp.TotalUsage.OutputTokens

	_ = tg.sessions.UpdateTitleAndUsage(ctx, sessionID, title, promptTokens, completionTokens, cost)
}

func openrouterCostFromMetadata(metadata fantasy.ProviderMetadata) *float64 {
	openrouterMetadata, ok := metadata[openrouter.Name]
	if !ok {
		return nil
	}
	opts, ok := openrouterMetadata.(*openrouter.ProviderMetadata)
	if !ok {
		return nil
	}
	return &opts.Usage.Cost
}
