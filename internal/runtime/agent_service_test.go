package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Karlsk/oryxos-go/internal/observability"
	"github.com/Karlsk/oryxos-go/internal/profile"
	"github.com/Karlsk/oryxos-go/internal/session"
)

func TestAgentServiceValidatesRequestAndMissingProfile(t *testing.T) {
	service := newAgentServiceForTest(t, fakeProfiles{}, &fakeSessions{}, &fakeAgentLoop{})
	cases := []AgentRequest{
		{},
		{ProfileName: "missing", Message: "hello"},
		{ProfileName: "known", Message: "  "},
	}
	for _, request := range cases {
		if _, err := service.Invoke(context.Background(), request); err == nil {
			t.Errorf("Invoke(%#v) error = nil, want validation failure", request)
		}
	}
	if _, err := service.Invoke(nil, AgentRequest{ProfileName: "known", Message: "hello"}); err == nil {
		t.Fatal("Invoke(nil) error = nil")
	}
}

func TestAgentServiceUsesOnePathForCallerShapesAndProfileIsolation(t *testing.T) {
	profiles := fakeProfiles{
		"ops":  {Name: "ops", Identity: profile.IdentityConfig{AgentName: "Ops"}},
		"news": {Name: "news", Identity: profile.IdentityConfig{AgentName: "News"}},
	}
	sessions := &fakeSessions{}
	loop := &fakeAgentLoop{content: "done"}
	service := newAgentServiceForTest(t, profiles, sessions, loop)
	requests := []AgentRequest{
		{ProfileName: "ops", Channel: "cli", UserID: "u1", Message: "one"},
		{ProfileName: "news", Channel: "web", UserID: "u2", SessionID: "explicit", Message: "two"},
		{ProfileName: "ops", Channel: "scheduler", UserID: "daily", Message: "three", Stateless: true},
	}
	ctx := observability.WithCorrelation(context.Background(), observability.Correlation{
		RequestID: "request-existing", ScheduleID: "schedule-existing",
	})
	for index, request := range requests {
		response, err := service.Invoke(ctx, request)
		if err != nil || response.Content != "done" || response.SessionID != sessions.resolved[index].ID {
			t.Fatalf("Invoke(%d) = %#v, %v", index, response, err)
		}
	}
	if !reflect.DeepEqual(sessions.requests, requests) {
		t.Fatalf("session requests = %#v, want unchanged %#v", sessions.requests, requests)
	}
	wantProfiles := []string{"ops", "news", "ops"}
	if !reflect.DeepEqual(loop.profiles, wantProfiles) || len(sessions.saved) != 3 {
		t.Fatalf("loop profiles=%v saved=%d", loop.profiles, len(sessions.saved))
	}
	for index, correlation := range loop.correlations {
		if correlation.ProfileName != wantProfiles[index] || correlation.Channel != requests[index].Channel || correlation.SessionID != sessions.resolved[index].ID || correlation.RequestID != "request-existing" || correlation.ScheduleID != "schedule-existing" {
			t.Fatalf("correlation %d = %#v", index, correlation)
		}
	}
	for index, correlation := range sessions.saveCorrelations {
		if correlation != loop.correlations[index] {
			t.Fatalf("save correlation %d = %#v, want %#v", index, correlation, loop.correlations[index])
		}
	}
}

func TestAgentServiceSavesAfterFailureWithDetachedContextAndJoinsErrors(t *testing.T) {
	runFailure := errors.New("run failure")
	saveFailure := errors.New("save failure")
	profiles := fakeProfiles{"ops": {Name: "ops"}}
	sessions := &fakeSessions{saveErr: saveFailure}
	loop := &fakeAgentLoop{err: runFailure}
	service := newAgentServiceForTest(t, profiles, sessions, loop)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := service.Invoke(ctx, AgentRequest{ProfileName: "ops", Channel: "cli", UserID: "u", Message: "work"})
	if !errors.Is(err, runFailure) || !errors.Is(err, saveFailure) {
		t.Fatalf("Invoke() error = %v, want both causes", err)
	}
	if len(sessions.saved) != 1 || sessions.saveContextErr != nil {
		t.Fatalf("saved=%d save context err=%v, want detached save", len(sessions.saved), sessions.saveContextErr)
	}
}

func TestAgentServiceSavesAfterSuccessAndReturnsSaveFailure(t *testing.T) {
	saveFailure := errors.New("save failure")
	sessions := &fakeSessions{saveErr: saveFailure}
	service := newAgentServiceForTest(t, fakeProfiles{"ops": {Name: "ops"}}, sessions, &fakeAgentLoop{content: "done"})
	_, err := service.Invoke(context.Background(), AgentRequest{ProfileName: "ops", Message: "work"})
	if !errors.Is(err, saveFailure) || len(sessions.saved) != 1 {
		t.Fatalf("Invoke() error = %v saved=%d", err, len(sessions.saved))
	}
}

func newAgentServiceForTest(t *testing.T, profiles profileResolver, sessions sessionService, loop agentLoop) AgentService {
	t.Helper()
	service, err := NewAgentService(profiles, sessions, loop)
	if err != nil {
		t.Fatalf("NewAgentService() error = %v", err)
	}
	return service
}

type fakeProfiles map[string]*profile.Profile

func (profiles fakeProfiles) Get(name string) (*profile.Profile, bool) {
	selected, ok := profiles[name]
	if !ok {
		return nil, false
	}
	cloned := *selected
	return &cloned, true
}

type fakeSessions struct {
	requests         []AgentRequest
	resolved         []*session.Session
	saved            []*session.Session
	resolveErr       error
	saveErr          error
	saveContextErr   error
	saveCorrelations []observability.Correlation
}

func (sessions *fakeSessions) Resolve(_ context.Context, request AgentRequest, _ string) (*session.Session, error) {
	sessions.requests = append(sessions.requests, request)
	if sessions.resolveErr != nil {
		return nil, sessions.resolveErr
	}
	id := request.SessionID
	if id == "" {
		id = request.Channel + "-resolved"
	}
	resolved := session.New(id)
	sessions.resolved = append(sessions.resolved, resolved)
	return resolved, nil
}

func (sessions *fakeSessions) Save(ctx context.Context, current *session.Session) error {
	sessions.saveContextErr = ctx.Err()
	sessions.saveCorrelations = append(sessions.saveCorrelations, observability.CorrelationFromContext(ctx))
	sessions.saved = append(sessions.saved, current)
	return sessions.saveErr
}

type fakeAgentLoop struct {
	content      string
	err          error
	profiles     []string
	correlations []observability.Correlation
}

func (loop *fakeAgentLoop) Run(ctx context.Context, _ *session.Session, _ string, selected *profile.Profile) (string, error) {
	loop.profiles = append(loop.profiles, selected.Name)
	loop.correlations = append(loop.correlations, observability.CorrelationFromContext(ctx))
	return loop.content, loop.err
}
