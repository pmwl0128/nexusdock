package evolution

import (
	"fmt"
	"sort"
	"strings"
)

const (
	EvidenceEnvironmentDrift  = "environment_drift"
	EvidenceSecurityViolation = "security_violation"
)

type TriggerEngine struct {
	clock      Clock
	ids        IDGenerator
	normalizer ErrorNormalizer
}

func NewTriggerEngine(clock Clock, ids IDGenerator) *TriggerEngine {
	return &TriggerEngine{clock: defaultClock(clock), ids: defaultIDs(ids)}
}

func (e *TriggerEngine) AnalyzeRun(run RunInput) ([]ObservationRecord, error) {
	if strings.TrimSpace(run.ID) == "" {
		return nil, validationError("run id is required")
	}
	if strings.TrimSpace(run.SkillID) == "" {
		return nil, validationError("skill id is required")
	}
	if run.Status != RunSucceeded && run.Status != RunFailed && run.Status != RunCancelled {
		return nil, validationError("unsupported run status")
	}
	observedAt := run.CompletedAt.UTC()
	if observedAt.IsZero() {
		observedAt = e.clock.Now().UTC()
	}
	evidenceIDs := collectEvidenceIDs(run.Evidence)
	private := evidencePrivate(run.Evidence)
	norm := e.normalizer.Normalize(run.Error)
	private = private || norm.PrivatePath
	ctx := ObservationContext{
		ProjectID: run.ProjectID, SuggestedFiles: append([]string(nil), run.SuggestedFiles...),
		OriginalPlan: append([]PlanStep(nil), run.OriginalPlan...), FinalPlan: append([]PlanStep(nil), run.FinalPlan...),
		Transient: norm.Transient, ErrorCategory: norm.Category,
	}

	var out []ObservationRecord
	seen := map[string]struct{}{}
	add := func(trigger Trigger, signature, summary string, forcePrivate bool, ids []string) {
		key := string(trigger) + "|" + signature
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		summary = e.normalizer.Redact(summary)
		out = append(out, ObservationRecord{Observation: Observation{
			ID: e.ids.NewID(), SkillID: run.SkillID, RunID: run.ID, DeviceID: cloneString(run.DeviceID),
			Trigger: trigger, Signature: signature, Summary: summary,
			EvidenceIDs: append([]string(nil), ids...), PrivateScope: private || forcePrivate, ObservedAt: observedAt,
		}, Context: ctx})
	}

	for _, correction := range run.UserCorrections {
		n := e.normalizer.Normalize(correction.Summary)
		add(TriggerUserCorrection, "user_correction:"+suffix(n.Signature), nonempty(correction.Summary, "user corrected the skill execution"), n.PrivatePath, mergeIDs(evidenceIDs, correction.ID))
	}
	for _, update := range run.UpstreamUpdates {
		n := e.normalizer.Normalize(update.Summary)
		add(TriggerUpstreamUpdate, "upstream_update:"+suffix(n.Signature), nonempty(update.Summary, "upstream skill changed"), n.PrivatePath, mergeIDs(evidenceIDs, update.ID))
	}
	for _, ev := range run.Evidence {
		switch ev.Kind {
		case EvidenceSecurityViolation:
			n := e.normalizer.Normalize(ev.Summary)
			add(TriggerSecurityViolation, "security_violation:"+suffix(n.Signature), nonempty(ev.Summary, "security policy violation detected"), ev.Private || n.PrivatePath, mergeIDs(evidenceIDs, ev.ID))
		case EvidenceEnvironmentDrift:
			n := e.normalizer.Normalize(ev.Summary)
			add(TriggerEnvironmentDrift, "environment_drift:"+suffix(n.Signature), nonempty(ev.Summary, "runtime environment drift detected"), ev.Private || n.PrivatePath, mergeIDs(evidenceIDs, ev.ID))
		}
	}

	if run.Status == RunSucceeded && run.Verification != nil && !run.Verification.Passed {
		n := e.normalizer.Normalize(run.Verification.Summary)
		add(TriggerFalseSuccess, "false_success:"+suffix(n.Signature), nonempty(run.Verification.Summary, "run reported success but verification failed"), n.PrivatePath, evidenceIDs)
	}
	if run.Status == RunFailed && run.Verification != nil && run.Verification.Passed {
		add(TriggerFalseFailure, "false_failure:"+suffix(norm.Signature), "run reported failure but verification passed", false, evidenceIDs)
	}
	if run.Status == RunFailed {
		add(TriggerRepeatedFailure, norm.Signature, nonempty(run.Error, "skill run failed"), norm.PrivatePath, evidenceIDs)
	}
	if run.Status == RunSucceeded && hasFailedStep(run.Steps) && hasPlanDelta(run.OriginalPlan, run.FinalPlan) {
		delta := ComputePlanDelta(run.OriginalPlan, run.FinalPlan)
		add(TriggerAgentRecovery, "agent_recovery:"+suffix(norm.Signature), delta.Summary, false, evidenceIDs)
	}
	if countManualSteps(run.Steps) >= 2 {
		add(TriggerRepeatedManualStep, manualStepSignature(run.Steps), "multiple manual steps were required to complete the run", false, evidenceIDs)
	}
	if run.Verification == nil {
		add(TriggerMissingValidation, "missing_validation:run_completion", "run completed without a verification result", false, evidenceIDs)
	}
	if run.BaselineDuration > 0 && run.Duration > 0 && float64(run.Duration)/float64(run.BaselineDuration) >= 1.5 {
		add(TriggerPerformanceRegression, "performance_regression:duration", fmt.Sprintf("run duration regressed to %.2fx baseline", float64(run.Duration)/float64(run.BaselineDuration)), false, evidenceIDs)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return triggerPriority(out[i].Observation.Trigger) > triggerPriority(out[j].Observation.Trigger)
	})
	return out, nil
}

func cloneString(v *string) *string {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}
func nonempty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
func suffix(signature string) string {
	if i := strings.IndexByte(signature, ':'); i >= 0 {
		return signature[i+1:]
	}
	return signature
}
func collectEvidenceIDs(evidence []Evidence) []string {
	ids := make([]string, 0, len(evidence))
	for _, e := range evidence {
		if e.ID != "" {
			ids = append(ids, e.ID)
		}
	}
	sort.Strings(ids)
	return unique(ids)
}
func evidencePrivate(evidence []Evidence) bool {
	for _, e := range evidence {
		if e.Private {
			return true
		}
	}
	return false
}
func mergeIDs(ids []string, extra string) []string {
	out := append([]string(nil), ids...)
	if extra != "" {
		out = append(out, extra)
	}
	sort.Strings(out)
	return unique(out)
}
func unique(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := values[:0]
	var prev string
	for i, v := range values {
		if i == 0 || v != prev {
			out = append(out, v)
			prev = v
		}
	}
	return out
}
func hasFailedStep(steps []Step) bool {
	for _, s := range steps {
		if s.Error != "" || strings.EqualFold(s.Outcome, "failed") {
			return true
		}
	}
	return false
}
func countManualSteps(steps []Step) int {
	n := 0
	for _, s := range steps {
		if s.Manual {
			n++
		}
	}
	return n
}
func manualStepSignature(steps []Step) string {
	names := make([]string, 0)
	for _, s := range steps {
		if s.Manual {
			names = append(names, strings.ToLower(strings.TrimSpace(s.Name)))
		}
	}
	sort.Strings(names)
	return "repeated_manual_step:" + suffix((ErrorNormalizer{}).Normalize(strings.Join(names, "|")).Signature)
}
func triggerPriority(t Trigger) int { return triggerWeights[t] }
