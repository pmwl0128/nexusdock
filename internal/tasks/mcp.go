package tasks

import (
	"context"
	"fmt"
	"strings"
)

type ContextBuilder interface {
	BuildTaskContext(context.Context, Actor, string, int) (any, error)
}

type MCPRequest struct {
	Action              string    `json:"action"`
	TaskID              string    `json:"task_id,omitempty"`
	Filter              Filter    `json:"filter,omitempty"`
	ExpectedVersion     int64     `json:"expected_version,omitempty"`
	Title               *string   `json:"title,omitempty"`
	Description         *string   `json:"description,omitempty"`
	Priority            *Priority `json:"priority,omitempty"`
	CompletionCriteria  *[]string `json:"completion_criteria,omitempty"`
	RiskConstraints     *[]string `json:"risk_constraints,omitempty"`
	Reason              string    `json:"reason,omitempty"`
	Summary             string    `json:"summary,omitempty"`
	VerificationSummary string    `json:"verification_summary,omitempty"`
	RunID               *string   `json:"run_id,omitempty"`
	EvidenceIDs         []string  `json:"evidence_ids,omitempty"`
	IdempotencyKey      string    `json:"idempotency_key,omitempty"`
	MaxBytes            int       `json:"max_bytes,omitempty"`
}

type MCPHandler struct {
	tasks   *Service
	context ContextBuilder
}

func NewMCPHandler(service *Service, contextBuilder ContextBuilder) *MCPHandler {
	return &MCPHandler{tasks: service, context: contextBuilder}
}

// Call implements the frozen nexus_task actions without depending on a
// particular MCP transport. T9/T1 can register this adapter in their server.
func (h *MCPHandler) Call(ctx context.Context, actor Actor, request MCPRequest) (any, error) {
	if h.tasks == nil {
		return nil, taskError(CodeRepository, "task service is not configured", nil)
	}
	action := strings.TrimSpace(request.Action)
	switch action {
	case "list":
		return h.tasks.List(ctx, actor, request.Filter)
	case "inspect":
		return h.tasks.Inspect(ctx, actor, request.TaskID)
	case "claim":
		return h.tasks.Claim(ctx, actor, request.TaskID, request.ExpectedVersion)
	case "context":
		if h.context == nil {
			return nil, fmt.Errorf("context pack builder is not configured")
		}
		return h.context.BuildTaskContext(ctx, actor, request.TaskID, request.MaxBytes)
	case "update":
		return h.tasks.Update(ctx, actor, request.TaskID, UpdateInput{
			Title: request.Title, Description: request.Description, Priority: request.Priority,
			CompletionCriteria: request.CompletionCriteria, RiskConstraints: request.RiskConstraints,
			ExpectedVersion: request.ExpectedVersion,
		})
	case "progress":
		return h.tasks.Progress(ctx, actor, request.TaskID, request.Reason, request.ExpectedVersion)
	case "block":
		return h.tasks.Block(ctx, actor, request.TaskID, request.Reason, request.ExpectedVersion)
	case "await_user":
		return h.tasks.AwaitUser(ctx, actor, request.TaskID, request.Reason, request.ExpectedVersion)
	case "await_agent":
		return h.tasks.AwaitAgent(ctx, actor, request.TaskID, request.Reason, request.ExpectedVersion)
	case "complete":
		return h.tasks.Complete(ctx, actor, request.TaskID, CompletionInput{
			Summary: request.Summary, VerificationSummary: request.VerificationSummary,
			RunID: request.RunID, EvidenceIDs: request.EvidenceIDs,
			ExpectedVersion: request.ExpectedVersion, IdempotencyKey: request.IdempotencyKey,
		})
	case "cancel":
		return h.tasks.Cancel(ctx, actor, request.TaskID, request.Reason, request.ExpectedVersion)
	default:
		return nil, taskError(CodeValidation, fmt.Sprintf("unsupported nexus_task action %q", action), nil)
	}
}
