package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/uvwt/nexusdock/internal/core"
)

type Event struct {
	ID         string         `json:"id"`
	OccurredAt time.Time      `json:"occurred_at"`
	Actor      core.Actor     `json:"actor"`
	Action     string         `json:"action"`
	ObjectType string         `json:"object_type"`
	ObjectID   string         `json:"object_id"`
	Result     string         `json:"result"`
	Risk       string         `json:"risk"`
	Approval   string         `json:"approval"`
	RunID      string         `json:"run_id,omitempty"`
	RequestID  string         `json:"request_id,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type AuditService interface {
	Record(context.Context, Event) (Event, error)
	List(context.Context, int) ([]Event, error)
}

type Service struct {
	db  *sql.DB
	now func() time.Time
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db, now: time.Now}
}

func (s *Service) Record(ctx context.Context, event Event) (Event, error) {
	if !event.Actor.Valid() || event.Action == "" || event.ObjectType == "" || event.ObjectID == "" || event.Result == "" {
		return Event{}, core.NewError(core.CodeValidation, "actor, action, object and result are required", nil)
	}
	if event.ID == "" {
		id, err := core.NewID("audit")
		if err != nil {
			return Event{}, err
		}
		event.ID = id
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = s.now().UTC()
	}
	if event.Risk == "" {
		event.Risk = "low"
	}
	if event.Approval == "" {
		event.Approval = "not_required"
	}
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return Event{}, core.NewError(core.CodeValidation, "audit metadata is not JSON serializable", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO audit_events(
		id, occurred_at, actor_type, actor_id, action, object_type, object_id,
		result, risk, approval, run_id, request_id, metadata_json
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?)`,
		event.ID, event.OccurredAt.Format(time.RFC3339Nano), event.Actor.Type, event.Actor.ID,
		event.Action, event.ObjectType, event.ObjectID, event.Result, event.Risk, event.Approval,
		event.RunID, event.RequestID, string(metadata))
	if err != nil {
		return Event{}, fmt.Errorf("insert audit event: %w", err)
	}
	return event, nil
}

func (s *Service) List(ctx context.Context, limit int) ([]Event, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, occurred_at, actor_type, actor_id, action,
		object_type, object_id, result, risk, approval, COALESCE(run_id, ''),
		COALESCE(request_id, ''), metadata_json FROM audit_events ORDER BY occurred_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()
	var events []Event
	for rows.Next() {
		var event Event
		var occurredAt, metadata string
		if err := rows.Scan(&event.ID, &occurredAt, &event.Actor.Type, &event.Actor.ID, &event.Action,
			&event.ObjectType, &event.ObjectID, &event.Result, &event.Risk, &event.Approval,
			&event.RunID, &event.RequestID, &metadata); err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		event.OccurredAt, _ = time.Parse(time.RFC3339Nano, occurredAt)
		_ = json.Unmarshal([]byte(metadata), &event.Metadata)
		events = append(events, event)
	}
	return events, rows.Err()
}
