package evolution

import (
	"context"
	"sync"
)

// Repository is implemented by the T1 persistence adapter. T7 owns no migrations.
type Repository interface {
	SaveObservation(context.Context, ObservationRecord) error
	ListObservations(context.Context, string, string) ([]ObservationRecord, error)
	GetCandidate(context.Context, string, string) (*EvolutionCandidate, error)
	SaveCandidate(context.Context, EvolutionCandidate) error
	GetProposalByCandidate(context.Context, string) (*EvolutionProposal, error)
	SaveProposal(context.Context, EvolutionProposal) error
}

type Event struct {
	Type       string
	ObjectID   string
	OccurredAt string
	Payload    any
}

type EventPublisher interface {
	Publish(context.Context, Event) error
}

// MemoryRepository is deterministic and useful for unit/integration adapters; production uses T1 storage.
type MemoryRepository struct {
	mu           sync.RWMutex
	observations map[string][]ObservationRecord
	candidates   map[string]EvolutionCandidate
	proposals    map[string]EvolutionProposal
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{observations: map[string][]ObservationRecord{}, candidates: map[string]EvolutionCandidate{}, proposals: map[string]EvolutionProposal{}}
}

func (r *MemoryRepository) SaveObservation(_ context.Context, record ObservationRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := CandidateKey(record.Observation.SkillID, record.Observation.Signature)
	for _, current := range r.observations[key] {
		if current.Observation.ID == record.Observation.ID {
			return nil
		}
	}
	r.observations[key] = append(r.observations[key], record)
	return nil
}

func (r *MemoryRepository) ListObservations(_ context.Context, skillID, signature string) ([]ObservationRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]ObservationRecord(nil), r.observations[CandidateKey(skillID, signature)]...), nil
}

func (r *MemoryRepository) GetCandidate(_ context.Context, skillID, signature string) (*EvolutionCandidate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.candidates[CandidateKey(skillID, signature)]
	if !ok {
		return nil, nil
	}
	copy := c
	return &copy, nil
}

func (r *MemoryRepository) SaveCandidate(_ context.Context, candidate EvolutionCandidate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.candidates[CandidateKey(candidate.SkillID, candidate.Signature)] = candidate
	return nil
}

func (r *MemoryRepository) GetProposalByCandidate(_ context.Context, candidateID string) (*EvolutionProposal, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.proposals[candidateID]
	if !ok {
		return nil, nil
	}
	copy := p
	return &copy, nil
}

func (r *MemoryRepository) SaveProposal(_ context.Context, proposal EvolutionProposal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.proposals[proposal.CandidateID] = proposal
	return nil
}
