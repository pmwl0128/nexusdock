package evolution

import (
	"fmt"
	"math"
	"sort"
)

var triggerWeights = map[Trigger]int{
	TriggerSecurityViolation:     100,
	TriggerFalseSuccess:          90,
	TriggerUserCorrection:        72,
	TriggerFalseFailure:          65,
	TriggerAgentRecovery:         55,
	TriggerMissingValidation:     45,
	TriggerPerformanceRegression: 40,
	TriggerUpstreamUpdate:        35,
	TriggerEnvironmentDrift:      30,
	TriggerRepeatedManualStep:    30,
	TriggerRepeatedFailure:       20,
}

type Scorer struct{}

func (Scorer) Score(records []ObservationRecord) (ScoreResult, error) {
	if len(records) == 0 {
		return ScoreResult{}, validationError("at least one observation is required")
	}
	primary := records[0].Observation.Trigger
	for _, record := range records {
		if !record.Observation.Trigger.Valid() {
			return ScoreResult{}, validationError("invalid trigger")
		}
		if triggerWeights[record.Observation.Trigger] > triggerWeights[primary] {
			primary = record.Observation.Trigger
		}
	}
	base := float64(triggerWeights[primary])
	runs, devices, evidence := sets(records)
	frequencyBonus := math.Min(24, float64(len(runs)-1)*12)
	deviceBonus := math.Min(12, float64(max(0, len(devices)-1))*6)
	evidenceBonus := math.Min(8, float64(evidence)*2)
	score := math.Min(100, base+frequencyBonus+deviceBonus+evidenceBonus)

	reasoning := []string{fmt.Sprintf("trigger %s contributes %.0f", primary, base)}
	if frequencyBonus > 0 {
		reasoning = append(reasoning, fmt.Sprintf("%d distinct runs add %.0f", len(runs), frequencyBonus))
	}
	if deviceBonus > 0 {
		reasoning = append(reasoning, fmt.Sprintf("%d devices add %.0f cross-device confidence", len(devices), deviceBonus))
	}
	if evidenceBonus > 0 {
		reasoning = append(reasoning, fmt.Sprintf("%d evidence references add %.0f", evidence, evidenceBonus))
	}

	confidence := 0.30 + math.Min(0.36, float64(len(runs))*0.12) + math.Min(0.30, float64(len(devices))*0.15)
	if primary == TriggerFalseSuccess {
		confidence = math.Max(confidence, 0.85)
	}
	if primary == TriggerSecurityViolation {
		confidence = math.Max(confidence, 0.95)
	}
	if singleTransientFailure(records, primary) {
		score = math.Min(score, 25)
		confidence = math.Min(confidence, 0.30)
		reasoning = append(reasoning, "single transient network failure is capped and remains under observation")
	}
	confidence = math.Min(0.99, confidence)
	status := statusForScore(score)
	sort.Strings(reasoning[1:])
	return ScoreResult{Score: round(score, 2), Confidence: round(confidence, 2), Reasoning: reasoning, Status: status}, nil
}

func sets(records []ObservationRecord) (map[string]struct{}, map[string]struct{}, int) {
	runs := map[string]struct{}{}
	devices := map[string]struct{}{}
	evidence := map[string]struct{}{}
	for _, record := range records {
		runs[record.Observation.RunID] = struct{}{}
		if record.Observation.DeviceID != nil && *record.Observation.DeviceID != "" {
			devices[*record.Observation.DeviceID] = struct{}{}
		}
		for _, id := range record.Observation.EvidenceIDs {
			evidence[id] = struct{}{}
		}
	}
	return runs, devices, len(evidence)
}

func singleTransientFailure(records []ObservationRecord, primary Trigger) bool {
	return len(records) == 1 && primary == TriggerRepeatedFailure && records[0].Context.Transient
}

func statusForScore(score float64) Status {
	switch {
	case score >= 80:
		return StatusCandidate
	case score >= 60:
		return StatusCandidate
	case score >= 35:
		return StatusWatching
	default:
		return StatusObserved
	}
}

func round(v float64, places int) float64 { p := math.Pow10(places); return math.Round(v*p) / p }
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
