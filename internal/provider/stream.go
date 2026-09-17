package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/Karlsk/oryxos-go/internal/llm"
	"github.com/Karlsk/oryxos-go/internal/profile"
)

// ChatStream resolves metadata-only Tool definitions and starts one
// Profile-bound incremental model call. Audit is written exactly once when the
// returned stream reaches a terminal outcome.
func (service *Service) ChatStream(ctx context.Context, sessionID string, selected *profile.Profile, messages []llm.Message) (llm.ResponseStream, error) {
	chatModel, definitions, err := service.prepareCall(ctx, sessionID, selected, "provider stream")
	if err != nil {
		return nil, err
	}
	startedAt := service.now()
	stream, callErr := chatModel.Stream(ctx, llm.Request{Messages: messages, Tools: definitions})
	if callErr == nil && stream == nil {
		callErr = fmt.Errorf("model returned nil response stream")
	}
	if callErr != nil {
		return nil, service.recordOutcome(ctx, callMetadataFrom(sessionID, selected), startedAt, nil, callErr)
	}
	return &auditedResponseStream{
		stream:    stream,
		service:   service,
		ctx:       ctx,
		metadata:  callMetadataFrom(sessionID, selected),
		startedAt: startedAt,
	}, nil
}

type auditedResponseStream struct {
	mu               sync.Mutex
	stream           llm.ResponseStream
	service          *Service
	ctx              context.Context
	metadata         callMetadata
	startedAt        time.Time
	terminal         bool
	callerClosed     bool
	upstreamClosed   bool
	upstreamCloseErr error
	closeResult      error
}

func (stream *auditedResponseStream) Recv() (llm.StreamEvent, error) {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if stream.callerClosed {
		return llm.StreamEvent{}, io.ErrClosedPipe
	}
	if stream.terminal {
		return llm.StreamEvent{}, io.EOF
	}

	event, recvErr := stream.stream.Recv()
	if recvErr != nil {
		if errors.Is(recvErr, io.EOF) {
			recvErr = fmt.Errorf("provider stream ended before completed response")
		}
		return llm.StreamEvent{}, stream.fail(recvErr)
	}
	switch event.Kind {
	case llm.StreamEventDelta:
		if event.Response != nil {
			return llm.StreamEvent{}, stream.fail(fmt.Errorf("provider stream delta contains completed response"))
		}
		return event, nil
	case llm.StreamEventCompleted:
		if event.Response == nil {
			return llm.StreamEvent{}, stream.fail(fmt.Errorf("provider stream completed without response"))
		}
		stream.terminal = true
		stream.closeUpstream()
		if err := stream.service.recordOutcome(stream.ctx, stream.metadata, stream.startedAt, event.Response, nil); err != nil {
			return llm.StreamEvent{}, err
		}
		return event, nil
	default:
		return llm.StreamEvent{}, stream.fail(fmt.Errorf("provider stream returned unknown event kind %q", event.Kind))
	}
}

func (stream *auditedResponseStream) Close() error {
	if stream == nil {
		return nil
	}
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if stream.callerClosed {
		return stream.closeResult
	}
	stream.callerClosed = true
	closeErr := stream.closeUpstream()
	if stream.terminal {
		stream.closeResult = closeErr
		return stream.closeResult
	}
	terminalErr := stream.service.recordOutcome(stream.ctx, stream.metadata, stream.startedAt, nil, fmt.Errorf("provider stream closed before completion"))
	stream.terminal = true
	stream.closeResult = joinStreamErrors(closeErr, terminalErr)
	return stream.closeResult
}

func (stream *auditedResponseStream) fail(cause error) error {
	stream.terminal = true
	closeErr := stream.closeUpstream()
	outcomeErr := stream.service.recordOutcome(stream.ctx, stream.metadata, stream.startedAt, nil, cause)
	return joinStreamErrors(closeErr, outcomeErr)
}

func (stream *auditedResponseStream) closeUpstream() error {
	if stream.upstreamClosed {
		return stream.upstreamCloseErr
	}
	stream.upstreamClosed = true
	if stream.stream == nil {
		return nil
	}
	stream.upstreamCloseErr = stream.stream.Close()
	return stream.upstreamCloseErr
}

func joinStreamErrors(closeErr, outcomeErr error) error {
	if closeErr == nil {
		return outcomeErr
	}
	safeCloseErr := safeWrap("close provider stream", closeErr)
	if outcomeErr == nil {
		return safeCloseErr
	}
	return errors.Join(safeCloseErr, outcomeErr)
}
