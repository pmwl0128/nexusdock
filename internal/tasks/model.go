package tasks

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Type is the stable Agent Inbox task type defined by the Nexus contract.
type Type string

const (
	TypeNeedsAgent Type = "needs_agent"
	TypeNeedsUser  Type = "needs_user"
	TypeAutomatic  Type = "automatic"
	TypeScheduled  Type = "scheduled"
	TypeReview     Type = "review"
)

// Status is the stable Task state defined by the Nexus contract.
type Status string

const (
	StatusInbox         Status = "inbox"
	StatusReady         Status = "ready"
	StatusInProgress    Status = "in_progress"
	StatusBlocked       Status = "blocked"
	StatusAwaitingUser  Status = "awaiting_user"
	StatusAwaitingAgent Status = "awaiting_agent"
	StatusCompleted     Status = "completed"
	StatusCancelled     Status = "cancelled"
	StatusFailed        Status = "failed"
)

// Priority is the stable task priority defined by the Nexus contract.
type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityNormal   Priority = "normal"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

// LinkType references an object owned by another Nexus module without copying it.
type LinkType string

const (
	LinkDevice   LinkType = "device"
	LinkMemory   LinkType = "memory"
	LinkSkill    LinkType = "skill"
	LinkRun      LinkType = "run"
	LinkProposal LinkType = "proposal"
	LinkProject  LinkType = "project"
)

// Actor intentionally mirrors the frozen public contract while remaining an
// internal domain type until generated DTOs are merged from T0.
type Actor struct {
	Type        string  `json:"type"`
	ID          string  `json:"id"`
	DisplayName *string `json:"display_name,omitempty"`
}

func (a Actor) Valid() bool {
	switch a.Type {
	case "user", "agent", "device", "system":
		return strings.TrimSpace(a.ID) != ""
	default:
		return false
	}
}

type Link struct {
	Type     LinkType `json:"type"`
	ObjectID string   `json:"object_id"`
	Relation string   `json:"relation"`
}

type Completion struct {
	Summary             string   `json:"summary"`
	VerificationSummary string   `json:"verification_summary"`
	RunID               *string  `json:"run_id,omitempty"`
	EvidenceIDs         []string `json:"evidence_ids"`
	CompletedAt         string   `json:"completed_at"`
}

// Task mirrors the T0 Task contract exactly. Internal provenance and history
// are returned through Inspection rather than leaking extra contract fields.
type Task struct {
	ID                 string      `json:"id"`
	Type               Type        `json:"type"`
	Status             Status      `json:"status"`
	Title              string      `json:"title"`
	Description        string      `json:"description"`
	Category           string      `json:"category"`
	SourceType         string      `json:"source_type"`
	SourceID           string      `json:"source_id"`
	ObjectID           string      `json:"object_id"`
	Priority           Priority    `json:"priority"`
	Links              []Link      `json:"links"`
	AssignedActor      *Actor      `json:"assigned_actor,omitempty"`
	CompletionCriteria []string    `json:"completion_criteria"`
	RiskConstraints    []string    `json:"risk_constraints"`
	Completion         *Completion `json:"completion,omitempty"`
	CreatedAt          string      `json:"created_at"`
	UpdatedAt          string      `json:"updated_at"`
	Version            int64       `json:"version"`
}

type Activity struct {
	ID        string         `json:"id"`
	TaskID    string         `json:"task_id"`
	Actor     Actor          `json:"actor"`
	Action    string         `json:"action"`
	From      Status         `json:"from_status,omitempty"`
	To        Status         `json:"to_status,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt string         `json:"created_at"`
}

type Inspection struct {
	Task           Task       `json:"task"`
	CreationReason string     `json:"creation_reason"`
	Activities     []Activity `json:"activities"`
}

type CreateInput struct {
	Type               Type
	Title              string
	Description        string
	Category           string
	SourceType         string
	SourceID           string
	ObjectID           string
	Priority           Priority
	Links              []Link
	CompletionCriteria []string
	RiskConstraints    []string
	CreationReason     string
}

type UpdateInput struct {
	Title              *string
	Description        *string
	Priority           *Priority
	CompletionCriteria *[]string
	RiskConstraints    *[]string
	ExpectedVersion    int64
}

type CompletionInput struct {
	Summary             string
	VerificationSummary string
	RunID               *string
	EvidenceIDs         []string
	ExpectedVersion     int64
	IdempotencyKey      string
}

type Filter struct {
	Statuses      []Status
	Types         []Type
	Category      string
	AssignedActor *Actor
	LinkType      LinkType
	LinkObjectID  string
	Limit         int
	Cursor        string
}

type Page struct {
	Items      []Task `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	Total      int    `json:"total"`
}

type CreateResult struct {
	Task    Task `json:"task"`
	Created bool `json:"created"`
}

func DedupKey(sourceType, sourceID, category, objectID string) string {
	raw := strings.Join([]string{
		strings.TrimSpace(sourceType),
		strings.TrimSpace(sourceID),
		strings.TrimSpace(category),
		strings.TrimSpace(objectID),
	}, "\x00")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func NewUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func parseTime(value string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, value)
	return t
}

func cloneTask(in Task) Task {
	out := in
	out.Links = append([]Link(nil), in.Links...)
	out.CompletionCriteria = append([]string(nil), in.CompletionCriteria...)
	out.RiskConstraints = append([]string(nil), in.RiskConstraints...)
	if in.AssignedActor != nil {
		actor := *in.AssignedActor
		out.AssignedActor = &actor
	}
	if in.Completion != nil {
		completion := *in.Completion
		completion.EvidenceIDs = append([]string(nil), in.Completion.EvidenceIDs...)
		out.Completion = &completion
	}
	return out
}
