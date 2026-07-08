package commands

import (
	"encoding/json"
	"time"

	"github.com/uvwt/agentdock-nexus/internal/devices"
)

type Type string

const (
	TypeHealthCheck        Type = "health.check"
	TypeSkillInstall       Type = "skill.install"
	TypeSkillRun           Type = "skill.run"
	TypeSkillRollback      Type = "skill.rollback"
	TypeRecallSync         Type = "recall.sync"
	TypeServiceInspect     Type = "service.inspect"
	TypeServiceRestart     Type = "service.restart"
	TypeDiagnosticsCollect Type = "diagnostics.collect"
	TypeAgentDockReload    Type = "agentdock.reload"
	TypeEnvManage          Type = "env.manage"
)

var allowedTypes = map[Type]struct{}{
	TypeHealthCheck: {}, TypeSkillInstall: {}, TypeSkillRun: {},
	TypeSkillRollback: {}, TypeRecallSync: {}, TypeServiceInspect: {},
	TypeServiceRestart: {}, TypeDiagnosticsCollect: {}, TypeAgentDockReload: {},
	TypeEnvManage: {},
}

func (t Type) Valid() bool {
	_, ok := allowedTypes[t]
	return ok
}

type Status string

const (
	StatusQueued    Status = "queued"
	StatusLeased    Status = "leased"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusExpired   Status = "expired"
	StatusCancelled Status = "cancelled"
)

func (s Status) Terminal() bool {
	return s == StatusSucceeded || s == StatusFailed || s == StatusExpired || s == StatusCancelled
}

type Command struct {
	ID             string            `json:"id"`
	DeviceID       string            `json:"device_id"`
	Type           Type              `json:"type"`
	Risk           devices.RiskLevel `json:"risk"`
	Payload        json.RawMessage   `json:"payload"`
	Status         Status            `json:"status"`
	IdempotencyKey string            `json:"idempotency_key"`
	Priority       int               `json:"priority"`
	MaxAttempts    int               `json:"max_attempts"`
	Attempts       int               `json:"attempts"`
	NotBefore      time.Time         `json:"not_before"`
	ExpiresAt      time.Time         `json:"expires_at"`
	LeaseID        string            `json:"lease_id,omitempty"`
	LeaseExpiresAt *time.Time        `json:"lease_expires_at,omitempty"`
	Progress       *Progress         `json:"progress,omitempty"`
	Result         *Result           `json:"result,omitempty"`
	CreatedBy      string            `json:"created_by"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	StartedAt      *time.Time        `json:"started_at,omitempty"`
	CompletedAt    *time.Time        `json:"completed_at,omitempty"`
	Version        int64             `json:"version"`
}

type Progress struct {
	Percent   int             `json:"percent"`
	Message   string          `json:"message,omitempty"`
	Details   json.RawMessage `json:"details,omitempty"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type Result struct {
	Success    bool            `json:"success"`
	Output     json.RawMessage `json:"output,omitempty"`
	ErrorCode  string          `json:"error_code,omitempty"`
	Error      string          `json:"error,omitempty"`
	Retryable  bool            `json:"retryable"`
	EvidenceID []string        `json:"evidence_ids,omitempty"`
	FinishedAt time.Time       `json:"finished_at"`
}

type EnqueueRequest struct {
	DeviceID       string
	Type           Type
	Risk           devices.RiskLevel
	Payload        json.RawMessage
	IdempotencyKey string
	Priority       int
	MaxAttempts    int
	NotBefore      time.Time
	ExpiresAt      time.Time
	CreatedBy      string
}

type Lease struct {
	ID                string    `json:"lease_id"`
	Command           Command   `json:"command"`
	LeasedAt          time.Time `json:"leased_at"`
	ExpiresAt         time.Time `json:"expires_at"`
	RenewAfterSeconds int       `json:"renew_after_seconds"`
}

func cloneCommand(command Command) Command {
	command.Payload = append(json.RawMessage(nil), command.Payload...)
	if command.Progress != nil {
		progress := *command.Progress
		progress.Details = append(json.RawMessage(nil), progress.Details...)
		command.Progress = &progress
	}
	if command.Result != nil {
		result := *command.Result
		result.Output = append(json.RawMessage(nil), result.Output...)
		result.EvidenceID = append([]string(nil), result.EvidenceID...)
		command.Result = &result
	}
	return command
}

func ptrTime(value time.Time) *time.Time {
	copyValue := value
	return &copyValue
}
