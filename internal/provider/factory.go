package provider

import (
	"context"

	"github.com/Karlsk/oryxos-go/internal/llm"
	deepseekmodel "github.com/cloudwego/eino-ext/components/model/deepseek"
	openaimodel "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

// ModelFactory constructs an OryxOS ChatModel for one Profile binding.
type ModelFactory func(context.Context, ProviderConfig) (llm.ChatModel, error)

type connectorConstructors struct {
	deepseek func(context.Context, *deepseekmodel.ChatModelConfig) (model.ToolCallingChatModel, error)
	openai   func(context.Context, *openaimodel.ChatModelConfig) (model.ToolCallingChatModel, error)
}

func productionConnectorConstructors() connectorConstructors {
	return connectorConstructors{
		deepseek: func(ctx context.Context, cfg *deepseekmodel.ChatModelConfig) (model.ToolCallingChatModel, error) {
			return deepseekmodel.NewChatModel(ctx, cfg)
		},
		openai: func(ctx context.Context, cfg *openaimodel.ChatModelConfig) (model.ToolCallingChatModel, error) {
			return openaimodel.NewChatModel(ctx, cfg)
		},
	}
}

func defaultModelFactories(constructors connectorConstructors) map[string]ModelFactory {
	return map[string]ModelFactory{
		DeepSeek: func(ctx context.Context, cfg ProviderConfig) (llm.ChatModel, error) {
			connector, err := constructors.deepseek(ctx, &deepseekmodel.ChatModelConfig{
				APIKey:      cfg.APIKey,
				Model:       cfg.Model,
				Temperature: cfg.Temperature,
			})
			return wrapEinoChatModel(connector, err)
		},
		MiniMax: func(ctx context.Context, cfg ProviderConfig) (llm.ChatModel, error) {
			temperature := cfg.Temperature
			connector, err := constructors.openai(ctx, &openaimodel.ChatModelConfig{
				APIKey:      cfg.APIKey,
				Model:       cfg.Model,
				BaseURL:     miniMaxOpenAIBaseURL,
				Temperature: &temperature,
			})
			return wrapEinoChatModel(connector, err)
		},
	}
}
