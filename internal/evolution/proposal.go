package evolution

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

const ProposalThreshold = 80.0

type ProposalGenerator struct {
	clock Clock
	ids   IDGenerator
}

func NewProposalGenerator(clock Clock, ids IDGenerator) *ProposalGenerator {
	return &ProposalGenerator{clock: defaultClock(clock), ids: defaultIDs(ids)}
}

func (g *ProposalGenerator) Generate(candidate EvolutionCandidate, records []ObservationRecord) (EvolutionProposal, error) {
	if candidate.Score < ProposalThreshold && candidate.Trigger != TriggerFalseSuccess && candidate.Trigger != TriggerSecurityViolation {
		return EvolutionProposal{}, &Error{Code: ErrNotEligible, Message: "candidate has not reached proposal threshold"}
	}
	if len(records) == 0 {
		return EvolutionProposal{}, validationError("proposal requires observations")
	}
	now := g.clock.Now().UTC()
	scope := proposalScope(records)
	files := proposalFiles(records, candidate.Trigger)
	return EvolutionProposal{
		ID: g.ids.NewID(), CandidateID: candidate.ID, SkillID: candidate.SkillID,
		Status: StatusReviewReady, Problem: proposalProblem(candidate, records),
		Evidence: proposalEvidence(records), Scope: scope, SuggestedFiles: files,
		Risk: proposalRisk(candidate.Trigger), Tests: proposalTests(candidate.Trigger),
		ExpectedBenefit: proposalBenefit(candidate, records), CreatedAt: now, UpdatedAt: now,
	}, nil
}

func proposalScope(records []ObservationRecord) Scope {
	project := false
	for _, r := range records {
		if r.Observation.PrivateScope {
			return ScopeDevice
		}
		if r.Context.ProjectID != "" {
			project = true
		}
	}
	if project {
		return ScopeProject
	}
	return ScopeGlobal
}

func proposalFiles(records []ObservationRecord, trigger Trigger) []string {
	seen := map[string]struct{}{}
	for _, r := range records {
		for _, file := range r.Context.SuggestedFiles {
			clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(file)))
			if clean == "." || clean == "" || filepath.IsAbs(file) || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") || strings.HasPrefix(clean, "~") {
				continue
			}
			seen[clean] = struct{}{}
		}
	}
	if len(seen) == 0 {
		seen["SKILL.md"] = struct{}{}
		if trigger == TriggerMissingValidation || trigger == TriggerSecurityViolation {
			seen["agentdock.yaml"] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for file := range seen {
		out = append(out, file)
	}
	sort.Strings(out)
	return out
}

func proposalEvidence(records []ObservationRecord) []ObjectReference {
	seen := map[string]ObjectReference{}
	for _, r := range records {
		if _, ok := seen[r.Observation.RunID]; !ok {
			seen[r.Observation.RunID] = ObjectReference{Type: "run", ID: r.Observation.RunID, Label: r.Observation.Summary}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]ObjectReference, 0, len(ids))
	for _, id := range ids {
		out = append(out, seen[id])
	}
	return out
}

func proposalProblem(candidate EvolutionCandidate, records []ObservationRecord) string {
	return fmt.Sprintf("Skill produced %s across %d observation(s): %s", candidate.Trigger, len(records), records[len(records)-1].Observation.Summary)
}

func proposalRisk(trigger Trigger) Risk {
	switch trigger {
	case TriggerSecurityViolation:
		return RiskCritical
	case TriggerFalseSuccess, TriggerUserCorrection:
		return RiskHigh
	case TriggerFalseFailure, TriggerPerformanceRegression, TriggerEnvironmentDrift:
		return RiskMedium
	default:
		return RiskLow
	}
}

func proposalTests(trigger Trigger) []string {
	tests := []string{"replay every referenced run fixture", "validate operation input and output schemas", "verify current stable release remains unchanged"}
	switch trigger {
	case TriggerFalseSuccess, TriggerMissingValidation:
		tests = append(tests, "assert failure is reported when post-condition verification fails")
	case TriggerSecurityViolation:
		tests = append(tests, "run path traversal, secret redaction, and undeclared network security tests")
	case TriggerPerformanceRegression:
		tests = append(tests, "benchmark against the recorded baseline and enforce regression threshold")
	case TriggerEnvironmentDrift:
		tests = append(tests, "run compatibility matrix against affected device environment")
	}
	sort.Strings(tests)
	return tests
}

func proposalBenefit(candidate EvolutionCandidate, records []ObservationRecord) string {
	devices := map[string]struct{}{}
	for _, r := range records {
		if r.Observation.DeviceID != nil {
			devices[*r.Observation.DeviceID] = struct{}{}
		}
	}
	return fmt.Sprintf("reduce recurrence of %s with %.0f%% confidence across %d run(s) and %d device(s)", candidate.Signature, candidate.Confidence*100, len(records), len(devices))
}
