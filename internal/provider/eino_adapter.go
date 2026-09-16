package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Karlsk/oryxos-go/internal/llm"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	einojsonschema "github.com/eino-contrib/jsonschema"
)

type einoChatModelAdapter struct {
	connector model.ToolCallingChatModel
}

func newEinoChatModelAdapter(connector model.ToolCallingChatModel) (llm.ChatModel, error) {
	if connector == nil {
		return nil, fmt.Errorf("create Eino chat model adapter: connector is nil")
	}
	return &einoChatModelAdapter{connector: connector}, nil
}

func wrapEinoChatModel(connector model.ToolCallingChatModel, err error) (llm.ChatModel, error) {
	if err != nil {
		return nil, err
	}
	return newEinoChatModelAdapter(connector)
}

func (adapter *einoChatModelAdapter) Generate(ctx context.Context, request llm.Request) (llm.Response, error) {
	if adapter == nil || adapter.connector == nil {
		return llm.Response{}, fmt.Errorf("generate with Eino adapter: adapter is not initialized")
	}
	if ctx == nil {
		return llm.Response{}, fmt.Errorf("generate with Eino adapter: context is nil")
	}

	messages, err := toEinoMessages(request.Messages)
	if err != nil {
		return llm.Response{}, err
	}
	tools, err := toEinoToolInfos(request.Tools)
	if err != nil {
		return llm.Response{}, err
	}

	connector := adapter.connector
	if len(tools) > 0 {
		connector, err = connector.WithTools(tools)
		if err != nil {
			return llm.Response{}, fmt.Errorf("bind Eino Tool schemas: %w", err)
		}
	}
	message, err := connector.Generate(ctx, messages)
	if err != nil {
		return llm.Response{}, err
	}
	if message == nil {
		return llm.Response{}, fmt.Errorf("Eino connector returned nil response")
	}
	return fromEinoResponse(message)
}

func toEinoMessages(messages []llm.Message) ([]*schema.Message, error) {
	converted := make([]*schema.Message, 0, len(messages))
	for index := range messages {
		message, err := toEinoMessage(messages[index])
		if err != nil {
			return nil, fmt.Errorf("convert message %d to Eino: %w", index, err)
		}
		converted = append(converted, message)
	}
	return converted, nil
}

func toEinoMessage(message llm.Message) (*schema.Message, error) {
	role, err := toEinoRole(message.Role)
	if err != nil {
		return nil, err
	}
	toolCalls := make([]schema.ToolCall, len(message.ToolCalls))
	for index, call := range message.ToolCalls {
		toolCalls[index] = schema.ToolCall{
			Index: call.Index,
			ID:    call.ID,
			Type:  call.Type,
			Function: schema.FunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
			Extra: call.Extra,
		}
	}
	return &schema.Message{
		Role:             role,
		Content:          message.Content,
		Name:             message.Name,
		ToolCalls:        toolCalls,
		ToolCallID:       message.ToolCallID,
		ToolName:         message.ToolName,
		ReasoningContent: message.ReasoningContent,
		Extra:            message.Extra,
	}, nil
}

func toEinoRole(role llm.Role) (schema.RoleType, error) {
	switch role {
	case llm.RoleSystem:
		return schema.System, nil
	case llm.RoleUser:
		return schema.User, nil
	case llm.RoleAssistant:
		return schema.Assistant, nil
	case llm.RoleTool:
		return schema.Tool, nil
	default:
		return "", fmt.Errorf("unsupported OryxOS message role %q", role)
	}
}

func toEinoToolInfos(definitions []llm.ToolDefinition) ([]*schema.ToolInfo, error) {
	infos := make([]*schema.ToolInfo, 0, len(definitions))
	for index, definition := range definitions {
		if strings.TrimSpace(definition.Name) == "" {
			return nil, fmt.Errorf("convert Tool definition %d to Eino: name is required", index)
		}
		info := &schema.ToolInfo{
			Name:  definition.Name,
			Desc:  definition.Description,
			Extra: definition.Extra,
		}
		if len(definition.InputSchema) > 0 {
			var inputSchema einojsonschema.Schema
			if err := json.Unmarshal(definition.InputSchema, &inputSchema); err != nil {
				return nil, fmt.Errorf("convert Tool definition %q JSON Schema: %w", definition.Name, err)
			}
			info.ParamsOneOf = schema.NewParamsOneOfByJSONSchema(&inputSchema)
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func fromEinoResponse(message *schema.Message) (llm.Response, error) {
	converted, err := fromEinoMessage(message)
	if err != nil {
		return llm.Response{}, err
	}
	response := llm.Response{Message: converted}
	if message.ResponseMeta != nil {
		response.FinishReason = message.ResponseMeta.FinishReason
		if message.ResponseMeta.Usage != nil {
			response.Usage = llm.Usage{
				PromptTokens:     message.ResponseMeta.Usage.PromptTokens,
				CompletionTokens: message.ResponseMeta.Usage.CompletionTokens,
				TotalTokens:      message.ResponseMeta.Usage.TotalTokens,
			}
		}
	}
	return response, nil
}

func fromEinoMessage(message *schema.Message) (llm.Message, error) {
	if message == nil {
		return llm.Message{}, fmt.Errorf("convert Eino message: message is nil")
	}
	role, err := fromEinoRole(message.Role)
	if err != nil {
		return llm.Message{}, err
	}
	toolCalls := make([]llm.ToolCall, len(message.ToolCalls))
	for index, call := range message.ToolCalls {
		toolCalls[index] = llm.ToolCall{
			Index: call.Index,
			ID:    call.ID,
			Type:  call.Type,
			Function: llm.FunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
			Extra: call.Extra,
		}
	}
	return llm.Message{
		Role:             role,
		Content:          message.Content,
		Name:             message.Name,
		ToolCalls:        toolCalls,
		ToolCallID:       message.ToolCallID,
		ToolName:         message.ToolName,
		ReasoningContent: message.ReasoningContent,
		Extra:            message.Extra,
	}, nil
}

func fromEinoRole(role schema.RoleType) (llm.Role, error) {
	switch role {
	case schema.System:
		return llm.RoleSystem, nil
	case schema.User:
		return llm.RoleUser, nil
	case schema.Assistant:
		return llm.RoleAssistant, nil
	case schema.Tool:
		return llm.RoleTool, nil
	default:
		return "", fmt.Errorf("unsupported Eino message role %q", role)
	}
}
