package runtime

import "context"

// AgentRequest is the shared request shape used by CLI, Web, and Scheduler.
type AgentRequest struct {
	ProfileName string
	Channel     string
	UserID      string
	SessionID   string
	Message     string
	Stateless   bool
}

// AgentResponse is the successful final response tied to its resolved Session.
type AgentResponse struct {
	SessionID string
	Content   string
}

// AgentService is the single synchronous Agent invocation boundary.
type AgentService interface {
	Invoke(ctx context.Context, request AgentRequest) (AgentResponse, error)
}
