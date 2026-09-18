package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Karlsk/oryxos-go/internal/config"
	"github.com/Karlsk/oryxos-go/internal/observability"
	"github.com/Karlsk/oryxos-go/internal/profile"
	"github.com/Karlsk/oryxos-go/internal/session"
)

type profileResolver interface {
	Get(name string) (*profile.Profile, bool)
}

// sessionService is the lesson 18 collaboration seam. Identity and
// persistence policy remain outside the runtime orchestrator.
type sessionService interface {
	Resolve(ctx context.Context, request AgentRequest, profileName string) (*session.Session, error)
	Save(ctx context.Context, current *session.Session) error
}

type agentLoop interface {
	Run(ctx context.Context, current *session.Session, userMessage string, selected *profile.Profile) (string, error)
}

type service struct {
	profiles profileResolver
	sessions sessionService
	loop     agentLoop
}

// NewAgentService constructs the shared orchestration entry point.
func NewAgentService(profiles profileResolver, sessions sessionService, loop agentLoop) (AgentService, error) {
	if profiles == nil {
		return nil, fmt.Errorf("create agent service: profile resolver is nil")
	}
	if sessions == nil {
		return nil, fmt.Errorf("create agent service: session service is nil")
	}
	if loop == nil {
		return nil, fmt.Errorf("create agent service: react loop is nil")
	}
	return &service{profiles: profiles, sessions: sessions, loop: loop}, nil
}

// Invoke resolves explicit runtime selections, runs one ReAct cycle, and saves
// accumulated state after both successful and failed cycles.
func (service *service) Invoke(ctx context.Context, request AgentRequest) (AgentResponse, error) {
	if service == nil || service.profiles == nil || service.sessions == nil || service.loop == nil {
		return AgentResponse{}, fmt.Errorf("invoke agent: service is not initialized")
	}
	if ctx == nil {
		return AgentResponse{}, fmt.Errorf("invoke agent: context is nil")
	}
	profileName := strings.TrimSpace(request.ProfileName)
	if profileName == "" {
		return AgentResponse{}, fmt.Errorf("invoke agent: profile_name is required")
	}
	if strings.TrimSpace(request.Message) == "" {
		return AgentResponse{}, fmt.Errorf("invoke agent: message is required")
	}
	selected, ok := service.profiles.Get(profileName)
	if !ok {
		return AgentResponse{}, fmt.Errorf("invoke agent: profile %q not found", profileName)
	}
	current, err := service.sessions.Resolve(ctx, request, selected.Name)
	if err != nil {
		return AgentResponse{}, safeRuntimeWrap("invoke agent: resolve session", err)
	}
	if current == nil || strings.TrimSpace(current.ID) == "" {
		return AgentResponse{}, fmt.Errorf("invoke agent: session service returned an invalid session")
	}

	correlation := observability.CorrelationFromContext(ctx)
	correlation.SessionID = current.ID
	correlation.ProfileName = selected.Name
	correlation.Channel = request.Channel
	runContext := observability.WithCorrelation(ctx, correlation)
	content, runErr := service.loop.Run(runContext, current, request.Message, selected)
	saveErr := service.sessions.Save(context.WithoutCancel(runContext), current)
	response := AgentResponse{SessionID: current.ID, Content: content}
	safeRunErr := safeRuntimeWrap("invoke agent: run", runErr)
	safeSaveErr := safeRuntimeWrap("invoke agent: save session", saveErr)
	if safeRunErr != nil && safeSaveErr != nil {
		return response, errors.Join(safeRunErr, safeSaveErr)
	}
	if safeRunErr != nil {
		return response, safeRunErr
	}
	if safeSaveErr != nil {
		return response, safeSaveErr
	}
	return response, nil
}

func safeRuntimeWrap(prefix string, cause error) error {
	if cause == nil {
		return nil
	}
	message := config.SanitizeErrorString(cause.Error())
	if message != cause.Error() {
		return fmt.Errorf("%s: %s", prefix, message)
	}
	return fmt.Errorf("%s: %w", prefix, cause)
}
