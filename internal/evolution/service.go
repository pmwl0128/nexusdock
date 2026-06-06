package evolution

import (
	"context"
	"fmt"
	"sort"
)

const (
	EventCandidateCreated    = "evolution.candidate.created"
	EventProposalReviewReady = "evolution.proposal.review_ready"
)

type ProcessResult struct {
	Observations []Observation
	Candidates   []EvolutionCandidate
	Proposals    []EvolutionProposal
}

type Service struct {
	repo       Repository
	publisher  EventPublisher
	triggers   *TriggerEngine
	aggregator *Aggregator
	proposals  *ProposalGenerator
	states     StateMachine
	clock      Clock
}

func NewService(repo Repository, publisher EventPublisher, clock Clock, ids IDGenerator) (*Service, error) {
	if repo == nil {
		return nil, validationError("repository is required")
	}
	clock = defaultClock(clock)
	ids = defaultIDs(ids)
	return &Service{repo: repo, publisher: publisher, triggers: NewTriggerEngine(clock, ids), aggregator: NewAggregator(clock, ids), proposals: NewProposalGenerator(clock, ids), clock: clock}, nil
}

func (s *Service) ProcessRun(ctx context.Context, run RunInput) (ProcessResult, error) {
	records, err := s.triggers.AnalyzeRun(run)
	if err != nil {
		return ProcessResult{}, err
	}
	result := ProcessResult{Observations: make([]Observation, 0, len(records))}
	keys := map[string]ObservationRecord{}
	for _, record := range records {
		if err := s.repo.SaveObservation(ctx, record); err != nil {
			return result, repoError("save observation", err)
		}
		result.Observations = append(result.Observations, record.Observation)
		keys[CandidateKey(record.Observation.SkillID, record.Observation.Signature)] = record
	}
	orderedKeys := make([]string, 0, len(keys))
	for key := range keys {
		orderedKeys = append(orderedKeys, key)
	}
	sort.Strings(orderedKeys)
	for _, key := range orderedKeys {
		seed := keys[key]
		all, err := s.repo.ListObservations(ctx, seed.Observation.SkillID, seed.Observation.Signature)
		if err != nil {
			return result, repoError("list observations", err)
		}
		existing, err := s.repo.GetCandidate(ctx, seed.Observation.SkillID, seed.Observation.Signature)
		if err != nil {
			return result, repoError("get candidate", err)
		}
		candidate, err := s.aggregator.Aggregate(all, existing)
		if err != nil {
			return result, err
		}
		created := existing == nil
		if err := s.repo.SaveCandidate(ctx, candidate); err != nil {
			return result, repoError("save candidate", err)
		}
		result.Candidates = append(result.Candidates, candidate)
		if created && candidate.Status == StatusCandidate {
			if err := s.publish(ctx, EventCandidateCreated, candidate.ID, candidate); err != nil {
				return result, err
			}
		}
		if candidate.Score < ProposalThreshold && candidate.Trigger != TriggerFalseSuccess && candidate.Trigger != TriggerSecurityViolation {
			continue
		}
		existingProposal, err := s.repo.GetProposalByCandidate(ctx, candidate.ID)
		if err != nil {
			return result, repoError("get proposal", err)
		}
		if existingProposal != nil {
			result.Proposals = append(result.Proposals, *existingProposal)
			continue
		}
		if candidate.Status == StatusCandidate {
			if err := s.states.Transition(candidate.Status, StatusProposalDraft); err != nil {
				return result, err
			}
			candidate.Status = StatusProposalDraft
			if err := s.states.Transition(candidate.Status, StatusReviewReady); err != nil {
				return result, err
			}
			candidate.Status = StatusReviewReady
			candidate.UpdatedAt = s.clock.Now().UTC()
			if err := s.repo.SaveCandidate(ctx, candidate); err != nil {
				return result, repoError("advance candidate", err)
			}
			result.Candidates[len(result.Candidates)-1] = candidate
		}
		proposal, err := s.proposals.Generate(candidate, all)
		if err != nil {
			return result, err
		}
		if err := s.repo.SaveProposal(ctx, proposal); err != nil {
			return result, repoError("save proposal", err)
		}
		result.Proposals = append(result.Proposals, proposal)
		if err := s.publish(ctx, EventProposalReviewReady, proposal.ID, proposal); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Service) publish(ctx context.Context, eventType, objectID string, payload any) error {
	if s.publisher == nil {
		return nil
	}
	e := Event{Type: eventType, ObjectID: objectID, OccurredAt: s.clock.Now().UTC().Format("2006-01-02T15:04:05Z07:00"), Payload: payload}
	if err := s.publisher.Publish(ctx, e); err != nil {
		return &Error{Code: ErrRepository, Message: fmt.Sprintf("publish %s", eventType), Cause: err}
	}
	return nil
}

func repoError(action string, err error) error {
	return &Error{Code: ErrRepository, Message: action, Cause: err}
}
