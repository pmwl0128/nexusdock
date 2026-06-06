package runs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/uvwt/agentdock-nexus/internal/core"
)

type Status string

const (
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type Run struct {
	ID             string          `json:"id"`
	Kind           string          `json:"kind"`
	Status         Status          `json:"status"`
	Actor          core.Actor      `json:"actor"`
	DeviceID       string          `json:"device_id,omitempty"`
	SkillID        string          `json:"skill_id,omitempty"`
	TaskID         string          `json:"task_id,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	Input          json.RawMessage `json:"input"`
	Output         json.RawMessage `json:"output,omitempty"`
	ErrorCode      string          `json:"error_code,omitempty"`
	ErrorMessage   string          `json:"error_message,omitempty"`
	StartedAt      time.Time       `json:"started_at"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	Version        int64           `json:"version"`
}

type Step struct {
	ID           string          `json:"id"`
	RunID        string          `json:"run_id"`
	Sequence     int             `json:"sequence"`
	Name         string          `json:"name"`
	Status       string          `json:"status"`
	Input        json.RawMessage `json:"input"`
	Output       json.RawMessage `json:"output,omitempty"`
	ErrorCode    string          `json:"error_code,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
	StartedAt    time.Time       `json:"started_at"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

type Evidence struct {
	ID        string          `json:"id"`
	RunID     string          `json:"run_id"`
	StepID    string          `json:"step_id,omitempty"`
	Kind      string          `json:"kind"`
	URI       string          `json:"uri,omitempty"`
	MediaType string          `json:"media_type,omitempty"`
	Digest    string          `json:"digest,omitempty"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

type Verification struct {
	ID         string    `json:"id"`
	RunID      string    `json:"run_id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Summary    string    `json:"summary"`
	EvidenceID string    `json:"evidence_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type CreateInput struct {
	Kind           string
	Actor          core.Actor
	DeviceID       string
	SkillID        string
	TaskID         string
	IdempotencyKey string
	Input          json.RawMessage
}

type CompleteInput struct {
	Status       Status
	Output       json.RawMessage
	ErrorCode    string
	ErrorMessage string
	Version      int64
}

type RunService interface {
	Create(context.Context, CreateInput) (Run, error)
	Get(context.Context, string) (Run, error)
	AppendStep(context.Context, Step) (Step, error)
	AddEvidence(context.Context, Evidence) (Evidence, error)
	AddVerification(context.Context, Verification) (Verification, error)
	Complete(context.Context, string, CompleteInput) (Run, error)
	Fail(context.Context, string, string, string, int64) (Run, error)
}

type Service struct {
	db     *sql.DB
	events core.EventBus
	now    func() time.Time
}

func NewService(db *sql.DB, events core.EventBus) *Service {
	return &Service{db: db, events: events, now: time.Now}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Run, error) {
	if input.Kind == "" || !input.Actor.Valid() {
		return Run{}, core.NewError(core.CodeValidation, "run kind and actor are required", nil)
	}
	if len(input.Input) == 0 {
		input.Input = json.RawMessage(`{}`)
	}
	if !json.Valid(input.Input) {
		return Run{}, core.NewError(core.CodeValidation, "run input must be valid JSON", nil)
	}
	if input.IdempotencyKey != "" {
		existing, err := s.getByIdempotencyKey(ctx, input.Actor, input.IdempotencyKey)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return Run{}, err
		}
	}
	id, err := core.NewID("run")
	if err != nil {
		return Run{}, err
	}
	now := s.now().UTC()
	run := Run{
		ID: id, Kind: input.Kind, Status: StatusRunning, Actor: input.Actor,
		DeviceID: input.DeviceID, SkillID: input.SkillID, TaskID: input.TaskID,
		IdempotencyKey: input.IdempotencyKey, Input: append(json.RawMessage(nil), input.Input...),
		StartedAt: now, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO runs(
		id, kind, status, actor_type, actor_id, device_id, skill_id, task_id, idempotency_key,
		input_json, started_at, created_at, updated_at, version
	) VALUES(?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?)`,
		run.ID, run.Kind, run.Status, run.Actor.Type, run.Actor.ID, run.DeviceID, run.SkillID,
		run.TaskID, run.IdempotencyKey, string(run.Input), formatTime(now), formatTime(now), formatTime(now), run.Version)
	if err != nil {
		if core.IsSQLiteConflict(err) && input.IdempotencyKey != "" {
			return s.getByIdempotencyKey(ctx, input.Actor, input.IdempotencyKey)
		}
		return Run{}, fmt.Errorf("create run: %w", err)
	}
	s.publish(ctx, "run.started", run.ID, map[string]any{"kind": run.Kind, "status": run.Status})
	return run, nil
}

func (s *Service) Get(ctx context.Context, id string) (Run, error) {
	if id == "" {
		return Run{}, core.NewError(core.CodeValidation, "run id is required", nil)
	}
	return scanRun(s.db.QueryRowContext(ctx, selectRun+` WHERE id = ?`, id))
}

func (s *Service) getByIdempotencyKey(ctx context.Context, actor core.Actor, key string) (Run, error) {
	return scanRun(s.db.QueryRowContext(ctx, selectRun+` WHERE actor_type = ? AND actor_id = ? AND idempotency_key = ?`, actor.Type, actor.ID, key))
}

func (s *Service) AppendStep(ctx context.Context, step Step) (Step, error) {
	if step.RunID == "" || step.Name == "" || step.Sequence < 1 {
		return Step{}, core.NewError(core.CodeValidation, "run id, step name and positive sequence are required", nil)
	}
	if step.Status == "" {
		step.Status = "running"
	}
	if len(step.Input) == 0 {
		step.Input = json.RawMessage(`{}`)
	}
	if !json.Valid(step.Input) || (len(step.Output) > 0 && !json.Valid(step.Output)) {
		return Step{}, core.NewError(core.CodeValidation, "step input and output must be valid JSON", nil)
	}
	if step.ID == "" {
		id, err := core.NewID("step")
		if err != nil {
			return Step{}, err
		}
		step.ID = id
	}
	now := s.now().UTC()
	if step.StartedAt.IsZero() {
		step.StartedAt = now
	}
	step.CreatedAt = now
	var completed any
	if step.CompletedAt != nil {
		completed = formatTime(*step.CompletedAt)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO run_steps(
		id, run_id, sequence, name, status, input_json, output_json, error_code, error_message,
		started_at, completed_at, created_at
	) VALUES(?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?)`,
		step.ID, step.RunID, step.Sequence, step.Name, step.Status, string(step.Input), string(step.Output),
		step.ErrorCode, step.ErrorMessage, formatTime(step.StartedAt), completed, formatTime(step.CreatedAt))
	if err != nil {
		if core.IsSQLiteConflict(err) {
			return Step{}, core.NewError(core.CodeDBConflict, "run step sequence already exists or run is missing", err)
		}
		return Step{}, fmt.Errorf("append run step: %w", err)
	}
	return step, nil
}

func (s *Service) AddEvidence(ctx context.Context, evidence Evidence) (Evidence, error) {
	if evidence.RunID == "" || evidence.Kind == "" {
		return Evidence{}, core.NewError(core.CodeValidation, "run id and evidence kind are required", nil)
	}
	if len(evidence.Payload) == 0 {
		evidence.Payload = json.RawMessage(`{}`)
	}
	if !json.Valid(evidence.Payload) {
		return Evidence{}, core.NewError(core.CodeValidation, "evidence payload must be valid JSON", nil)
	}
	if evidence.ID == "" {
		id, err := core.NewID("evd")
		if err != nil {
			return Evidence{}, err
		}
		evidence.ID = id
	}
	evidence.CreatedAt = s.now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO run_evidence(
		id, run_id, step_id, kind, uri, media_type, digest, payload_json, created_at
	) VALUES(?, ?, NULLIF(?, ''), ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?)`,
		evidence.ID, evidence.RunID, evidence.StepID, evidence.Kind, evidence.URI, evidence.MediaType,
		evidence.Digest, string(evidence.Payload), formatTime(evidence.CreatedAt))
	if err != nil {
		return Evidence{}, fmt.Errorf("add run evidence: %w", err)
	}
	return evidence, nil
}

func (s *Service) AddVerification(ctx context.Context, verification Verification) (Verification, error) {
	if verification.RunID == "" || verification.Name == "" || !validVerificationStatus(verification.Status) {
		return Verification{}, core.NewError(core.CodeValidation, "run id, verification name and valid status are required", nil)
	}
	if verification.ID == "" {
		id, err := core.NewID("verify")
		if err != nil {
			return Verification{}, err
		}
		verification.ID = id
	}
	verification.CreatedAt = s.now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO run_verifications(
		id, run_id, name, status, summary, evidence_id, created_at
	) VALUES(?, ?, ?, ?, ?, NULLIF(?, ''), ?)`, verification.ID, verification.RunID,
		verification.Name, verification.Status, verification.Summary, verification.EvidenceID, formatTime(verification.CreatedAt))
	if err != nil {
		return Verification{}, fmt.Errorf("add run verification: %w", err)
	}
	return verification, nil
}

func (s *Service) Complete(ctx context.Context, id string, input CompleteInput) (Run, error) {
	if id == "" || input.Version < 1 {
		return Run{}, core.NewError(core.CodeValidation, "run id and current version are required", nil)
	}
	if input.Status != StatusSucceeded && input.Status != StatusFailed && input.Status != StatusCancelled {
		return Run{}, core.NewError(core.CodeValidation, "terminal run status is required", nil)
	}
	if len(input.Output) > 0 && !json.Valid(input.Output) {
		return Run{}, core.NewError(core.CodeValidation, "run output must be valid JSON", nil)
	}
	now := s.now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE runs SET status = ?, output_json = NULLIF(?, ''),
		error_code = NULLIF(?, ''), error_message = NULLIF(?, ''), completed_at = ?, updated_at = ?, version = version + 1
		WHERE id = ? AND status = 'running' AND version = ?`, input.Status, string(input.Output), input.ErrorCode,
		input.ErrorMessage, formatTime(now), formatTime(now), id, input.Version)
	if err != nil {
		return Run{}, fmt.Errorf("complete run: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		if _, getErr := s.Get(ctx, id); getErr != nil {
			return Run{}, getErr
		}
		return Run{}, core.NewError(core.CodeVersionConflict, "run was already completed or version changed", nil)
	}
	run, err := s.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	s.publish(ctx, "run.completed", run.ID, map[string]any{"kind": run.Kind, "status": run.Status})
	return run, nil
}

func (s *Service) Fail(ctx context.Context, id, code, message string, version int64) (Run, error) {
	return s.Complete(ctx, id, CompleteInput{Status: StatusFailed, ErrorCode: code, ErrorMessage: message, Version: version})
}

func (s *Service) publish(ctx context.Context, eventType, runID string, data map[string]any) {
	if s.events == nil {
		return
	}
	data["run_id"] = runID
	_ = s.events.Publish(ctx, core.Event{Type: eventType, Data: data})
}

const selectRun = `SELECT id, kind, status, actor_type, actor_id, COALESCE(device_id, ''),
	COALESCE(skill_id, ''), COALESCE(task_id, ''), COALESCE(idempotency_key, ''), input_json,
	COALESCE(output_json, ''), COALESCE(error_code, ''), COALESCE(error_message, ''), started_at,
	completed_at, created_at, updated_at, version FROM runs`

type rowScanner interface{ Scan(...any) error }

func scanRun(row rowScanner) (Run, error) {
	var run Run
	var actorType string
	var input, output, started, created, updated string
	var completed sql.NullString
	err := row.Scan(&run.ID, &run.Kind, &run.Status, &actorType, &run.Actor.ID, &run.DeviceID, &run.SkillID,
		&run.TaskID, &run.IdempotencyKey, &input, &output, &run.ErrorCode, &run.ErrorMessage, &started,
		&completed, &created, &updated, &run.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, core.NewError(core.CodeNotFound, "run not found", err)
	}
	if err != nil {
		return Run{}, fmt.Errorf("scan run: %w", err)
	}
	run.Actor.Type = core.ActorType(actorType)
	run.Input = json.RawMessage(input)
	if output != "" {
		run.Output = json.RawMessage(output)
	}
	run.StartedAt, _ = time.Parse(time.RFC3339Nano, started)
	run.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	run.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if completed.Valid {
		value, _ := time.Parse(time.RFC3339Nano, completed.String)
		run.CompletedAt = &value
	}
	return run, nil
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func validVerificationStatus(status string) bool {
	return status == "passed" || status == "failed" || status == "skipped"
}
