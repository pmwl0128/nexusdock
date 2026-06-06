package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type ConflictSource string

const (
	ConflictSourceDeviceSnapshot ConflictSource = "device_snapshot"
	ConflictSourceSkillRun       ConflictSource = "skill_run"
	ConflictSourceUserEdit       ConflictSource = "user_edit"
	ConflictSourceGitMerge       ConflictSource = "git_merge"
	ConflictSourceAgentRepair    ConflictSource = "agent_repair"
)

func (s ConflictSource) Valid() bool {
	switch s {
	case ConflictSourceDeviceSnapshot, ConflictSourceSkillRun, ConflictSourceUserEdit, ConflictSourceGitMerge, ConflictSourceAgentRepair:
		return true
	default:
		return false
	}
}

type ConflictStatus string

const (
	ConflictOpen     ConflictStatus = "open"
	ConflictResolved ConflictStatus = "resolved"
	ConflictIgnored  ConflictStatus = "ignored"
)

type ObservedFact struct {
	MemoryPath    string         `json:"memory_path"`
	Key           string         `json:"key"`
	MemoryValue   string         `json:"memory_value"`
	ObservedValue string         `json:"observed_value"`
	Source        ConflictSource `json:"source"`
	SourceID      string         `json:"source_id,omitempty"`
	Device        string         `json:"device,omitempty"`
	Agent         string         `json:"agent,omitempty"`
	RunID         string         `json:"run_id,omitempty"`
	Confidence    Confidence     `json:"confidence"`
	ObservedAt    time.Time      `json:"observed_at"`
}

type DetectConflictRequest struct {
	Facts []ObservedFact `json:"facts"`
}

type MemoryConflict struct {
	ID            string         `json:"id"`
	MemoryPath    string         `json:"memory_path"`
	Key           string         `json:"key"`
	MemoryValue   string         `json:"memory_value"`
	ObservedValue string         `json:"observed_value"`
	Source        ConflictSource `json:"source"`
	SourceID      string         `json:"source_id,omitempty"`
	Device        string         `json:"device,omitempty"`
	Agent         string         `json:"agent,omitempty"`
	RunID         string         `json:"run_id,omitempty"`
	Confidence    Confidence     `json:"confidence"`
	Status        ConflictStatus `json:"status"`
	DetectedAt    time.Time      `json:"detected_at"`
	ResolvedAt    *time.Time     `json:"resolved_at,omitempty"`
}

type ConflictRepository interface {
	Upsert(context.Context, MemoryConflict) error
	ListOpen(context.Context, ContextPackRequest) ([]MemoryConflict, error)
	Resolve(context.Context, string, ConflictStatus) error
}

type InMemoryConflictRepository struct {
	mu    sync.RWMutex
	items map[string]MemoryConflict
}

func NewInMemoryConflictRepository() *InMemoryConflictRepository {
	return &InMemoryConflictRepository{items: map[string]MemoryConflict{}}
}

func (r *InMemoryConflictRepository) Upsert(_ context.Context, conflict MemoryConflict) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.items[conflict.ID]; ok && existing.Status == ConflictResolved {
		return nil
	}
	r.items[conflict.ID] = conflict
	return nil
}

func (r *InMemoryConflictRepository) ListOpen(_ context.Context, req ContextPackRequest) ([]MemoryConflict, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]MemoryConflict, 0, len(r.items))
	for _, item := range r.items {
		if item.Status != ConflictOpen {
			continue
		}
		if req.Device != "" && !strings.EqualFold(req.Device, item.Device) {
			continue
		}
		if req.Agent != "" && !strings.EqualFold(req.Agent, item.Agent) {
			continue
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DetectedAt.Equal(out[j].DetectedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].DetectedAt.After(out[j].DetectedAt)
	})
	return out, nil
}

func (r *InMemoryConflictRepository) Resolve(_ context.Context, id string, status ConflictStatus) error {
	if status != ConflictResolved && status != ConflictIgnored {
		return errors.New("conflict can only be resolved or ignored")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[id]
	if !ok {
		return errors.New("memory conflict not found")
	}
	now := time.Now().UTC()
	item.Status = status
	item.ResolvedAt = &now
	r.items[id] = item
	return nil
}

func conflictFromFact(fact ObservedFact, now time.Time) (MemoryConflict, bool) {
	if strings.TrimSpace(fact.MemoryPath) == "" || strings.TrimSpace(fact.Key) == "" || !fact.Source.Valid() {
		return MemoryConflict{}, false
	}
	if fact.Confidence == "" {
		fact.Confidence = ConfidenceMedium
	}
	if !fact.Confidence.Valid() || fact.Confidence == ConfidenceLow || fact.Confidence == ConfidenceUnknown {
		return MemoryConflict{}, false
	}
	if normalizeFactValue(fact.MemoryValue) == normalizeFactValue(fact.ObservedValue) {
		return MemoryConflict{}, false
	}
	if fact.ObservedAt.IsZero() {
		fact.ObservedAt = now
	}
	fingerprint := strings.Join([]string{
		strings.TrimSpace(fact.MemoryPath), strings.TrimSpace(fact.Key), normalizeFactValue(fact.MemoryValue),
		normalizeFactValue(fact.ObservedValue), string(fact.Source), strings.TrimSpace(fact.SourceID),
	}, "\x00")
	sum := sha256.Sum256([]byte(fingerprint))
	return MemoryConflict{
		ID: "mc_" + hex.EncodeToString(sum[:12]), MemoryPath: fact.MemoryPath, Key: fact.Key,
		MemoryValue: fact.MemoryValue, ObservedValue: fact.ObservedValue, Source: fact.Source,
		SourceID: fact.SourceID, Device: fact.Device, Agent: fact.Agent, RunID: fact.RunID,
		Confidence: fact.Confidence, Status: ConflictOpen, DetectedAt: fact.ObservedAt.UTC(),
	}, true
}

func normalizeFactValue(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}
