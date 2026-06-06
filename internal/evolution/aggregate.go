package evolution

import (
	"sort"
	"strings"
)

type Aggregator struct {
	clock  Clock
	ids    IDGenerator
	scorer Scorer
}

func NewAggregator(clock Clock, ids IDGenerator) *Aggregator {
	return &Aggregator{clock: defaultClock(clock), ids: defaultIDs(ids)}
}

func CandidateKey(skillID, signature string) string {
	return strings.TrimSpace(skillID) + "|" + strings.TrimSpace(signature)
}

func (a *Aggregator) Aggregate(records []ObservationRecord, existing *EvolutionCandidate) (EvolutionCandidate, error) {
	if len(records) == 0 {
		return EvolutionCandidate{}, validationError("at least one observation is required")
	}
	skillID := records[0].Observation.SkillID
	signature := records[0].Observation.Signature
	for _, record := range records {
		if record.Observation.SkillID != skillID || record.Observation.Signature != signature {
			return EvolutionCandidate{}, validationError("observations must have the same skill and signature")
		}
	}
	score, err := a.scorer.Score(records)
	if err != nil {
		return EvolutionCandidate{}, err
	}
	now := a.clock.Now().UTC()
	candidate := EvolutionCandidate{
		ID: a.ids.NewID(), SkillID: skillID, Status: score.Status, Signature: signature,
		Trigger: primaryTrigger(records), ObservationIDs: observationIDs(records), Score: score.Score,
		Confidence: score.Confidence, Reasoning: score.Reasoning, CreatedAt: now, UpdatedAt: now,
	}
	if existing != nil {
		candidate.ID = existing.ID
		candidate.CreatedAt = existing.CreatedAt
		if isWorkflowStatus(existing.Status) {
			candidate.Status = existing.Status
		}
	}
	return candidate, nil
}

func primaryTrigger(records []ObservationRecord) Trigger {
	t := records[0].Observation.Trigger
	for _, r := range records[1:] {
		if triggerPriority(r.Observation.Trigger) > triggerPriority(t) {
			t = r.Observation.Trigger
		}
	}
	return t
}
func observationIDs(records []ObservationRecord) []string {
	ids := make([]string, 0, len(records))
	for _, r := range records {
		ids = append(ids, r.Observation.ID)
	}
	sort.Strings(ids)
	return unique(ids)
}
func isWorkflowStatus(s Status) bool {
	switch s {
	case StatusProposalDraft, StatusReviewReady, StatusApproved, StatusTesting, StatusCanary, StatusReleased, StatusRejected, StatusDeferred, StatusRolledBack:
		return true
	}
	return false
}
