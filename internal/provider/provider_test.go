package provider

import (
	"context"
	"errors"
	"sync"

	"github.com/Karlsk/oryxos-go/internal/llm"
	"github.com/Karlsk/oryxos-go/internal/store"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fakeModelState struct {
	mu              sync.Mutex
	response        *schema.Message
	err             error
	withToolsErr    error
	generateCalls   int
	withToolsCalls  int
	boundTools      []*schema.ToolInfo
	inputs          [][]*schema.Message
	options         []*model.Options
	contextObserved context.Context
}

type fakeModel struct{ state *fakeModelState }

func (fake *fakeModel) Generate(ctx context.Context, input []*schema.Message, options ...model.Option) (*schema.Message, error) {
	fake.state.mu.Lock()
	defer fake.state.mu.Unlock()
	fake.state.generateCalls++
	fake.state.contextObserved = ctx
	fake.state.inputs = append(fake.state.inputs, append([]*schema.Message(nil), input...))
	fake.state.options = append(fake.state.options, model.GetCommonOptions(nil, options...))
	return fake.state.response, fake.state.err
}

func (fake *fakeModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("stream is not supported by fake model")
}

func (fake *fakeModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	fake.state.mu.Lock()
	defer fake.state.mu.Unlock()
	fake.state.withToolsCalls++
	if fake.state.withToolsErr != nil {
		return nil, fake.state.withToolsErr
	}
	fake.state.boundTools = append([]*schema.ToolInfo(nil), tools...)
	return &fakeModel{state: fake.state}, nil
}

type fakeRecorder struct {
	mu              sync.Mutex
	calls           []store.LlmCall
	err             error
	contextObserved context.Context
}

func (recorder *fakeRecorder) Create(ctx context.Context, call *store.LlmCall) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.contextObserved = ctx
	recorder.calls = append(recorder.calls, *call)
	return recorder.err
}

func newFakeModel(response *schema.Message, err error) (*fakeModel, *fakeModelState) {
	state := &fakeModelState{response: response, err: err}
	return &fakeModel{state: state}, state
}

type fakeOryxModelState struct {
	mu              sync.Mutex
	response        llm.Response
	err             error
	generateCalls   int
	requests        []llm.Request
	contextObserved context.Context
}

type fakeOryxModel struct{ state *fakeOryxModelState }

func (fake *fakeOryxModel) Generate(ctx context.Context, request llm.Request) (llm.Response, error) {
	fake.state.mu.Lock()
	defer fake.state.mu.Unlock()
	fake.state.generateCalls++
	fake.state.contextObserved = ctx
	fake.state.requests = append(fake.state.requests, request)
	return fake.state.response, fake.state.err
}

func newFakeOryxModel(response llm.Response, err error) (*fakeOryxModel, *fakeOryxModelState) {
	state := &fakeOryxModelState{response: response, err: err}
	return &fakeOryxModel{state: state}, state
}

type fakeToolDefinitionSource struct {
	definitions map[string]llm.ToolDefinition
	calls       []string
}

func (source *fakeToolDefinitionSource) Info(_ context.Context, name string) (llm.ToolDefinition, bool) {
	source.calls = append(source.calls, name)
	definition, ok := source.definitions[name]
	return definition, ok
}
