package evolution_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/uvwt/agentdock-nexus/internal/evolution"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type ids struct{ n int }

func (g *ids) NewID() string {
	g.n++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", g.n)
}

type events struct{ values []evolution.Event }

func (e *events) Publish(_ context.Context, event evolution.Event) error {
	e.values = append(e.values, event)
	return nil
}

func TestFalseSuccessImmediatelyCreatesReviewProposal(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)}
	repo := evolution.NewMemoryRepository()
	pub := &events{}
	service, err := evolution.NewService(repo, pub, clock, &ids{})
	if err != nil {
		t.Fatal(err)
	}
	device := "dockair"
	result, err := service.ProcessRun(context.Background(), evolution.RunInput{
		ID: "run-1", SkillID: "skill-1", DeviceID: &device, Status: evolution.RunSucceeded,
		Verification: &evolution.Verification{Passed: false, Summary: "output file was not created"},
		Evidence:     []evolution.Evidence{{ID: "evidence-1", Kind: "verification", Summary: "missing output"}},
		CompletedAt:  clock.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Proposals) != 1 {
		t.Fatalf("expected proposal, got %d", len(result.Proposals))
	}
	if result.Proposals[0].Status != evolution.StatusReviewReady {
		t.Fatalf("unexpected status %s", result.Proposals[0].Status)
	}
	if result.Candidates[0].Score < evolution.ProposalThreshold {
		t.Fatalf("false success score too low: %.2f", result.Candidates[0].Score)
	}
	if len(result.Proposals[0].Tests) == 0 {
		t.Fatal("proposal must contain tests")
	}
	if len(pub.values) == 0 || pub.values[len(pub.values)-1].Type != evolution.EventProposalReviewReady {
		t.Fatalf("review event not published: %#v", pub.values)
	}
}

func TestSingleTransientNetworkFailureDoesNotEscalate(t *testing.T) {
	engine := evolution.NewTriggerEngine(fixedClock{now: time.Now().UTC()}, &ids{})
	records, err := engine.AnalyzeRun(evolution.RunInput{ID: "run-1", SkillID: "skill-1", Status: evolution.RunFailed, Error: "dial tcp: connection timed out", Verification: &evolution.Verification{Passed: false}})
	if err != nil {
		t.Fatal(err)
	}
	var failure evolution.ObservationRecord
	for _, r := range records {
		if r.Observation.Trigger == evolution.TriggerRepeatedFailure {
			failure = r
		}
	}
	candidate, err := evolution.NewAggregator(fixedClock{now: time.Now().UTC()}, &ids{}).Aggregate([]evolution.ObservationRecord{failure}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Status == evolution.StatusCandidate || candidate.Score > 25 {
		t.Fatalf("transient failure escalated: status=%s score=%.2f", candidate.Status, candidate.Score)
	}
}

func TestCrossDeviceFailuresIncreaseConfidence(t *testing.T) {
	clock := fixedClock{now: time.Now().UTC()}
	engine := evolution.NewTriggerEngine(clock, &ids{})
	devices := []string{"dockair", "dockmini"}
	var failures []evolution.ObservationRecord
	for i, device := range devices {
		records, err := engine.AnalyzeRun(evolution.RunInput{ID: "run-" + string(rune('1'+i)), SkillID: "skill-1", DeviceID: &device, Status: evolution.RunFailed, Error: "schema validation failed at field 42", Verification: &evolution.Verification{Passed: false}})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range records {
			if r.Observation.Trigger == evolution.TriggerRepeatedFailure {
				failures = append(failures, r)
			}
		}
	}
	agg := evolution.NewAggregator(clock, &ids{})
	one, _ := agg.Aggregate(failures[:1], nil)
	two, _ := agg.Aggregate(failures, nil)
	if two.Confidence <= one.Confidence || two.Score <= one.Score {
		t.Fatalf("cross-device evidence did not increase score/confidence: one=%+v two=%+v", one, two)
	}
}

func TestPrivatePathsStayDeviceScopedAndOutOfSuggestedFiles(t *testing.T) {
	clock := fixedClock{now: time.Now().UTC()}
	repo := evolution.NewMemoryRepository()
	service, _ := evolution.NewService(repo, nil, clock, &ids{})
	device := "dockmini"
	result, err := service.ProcessRun(context.Background(), evolution.RunInput{
		ID: "run-private", SkillID: "skill-private", DeviceID: &device, Status: evolution.RunSucceeded,
		Verification:   &evolution.Verification{Passed: false, Summary: "missing /Users/alice/secrets/output.json"},
		SuggestedFiles: []string{"/Users/alice/skill/private.sh", "scripts/check.sh"},
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal := result.Proposals[0]
	if proposal.Scope != evolution.ScopeDevice {
		t.Fatalf("expected device scope, got %s", proposal.Scope)
	}
	b, _ := json.Marshal(proposal)
	if strings.Contains(string(b), "/Users/alice") {
		t.Fatalf("private path leaked: %s", b)
	}
	if len(proposal.SuggestedFiles) != 1 || proposal.SuggestedFiles[0] != "scripts/check.sh" {
		t.Fatalf("unexpected files: %#v", proposal.SuggestedFiles)
	}
}

func TestPlanDeltaAndStateMachine(t *testing.T) {
	delta := evolution.ComputePlanDelta(
		[]evolution.PlanStep{{Name: "deploy", Action: "restart", Outcome: "failed"}},
		[]evolution.PlanStep{{Name: "deploy", Action: "reload", Outcome: "succeeded"}, {Name: "verify", Action: "healthz", Validation: true}},
	)
	if len(delta.Added) != 1 || len(delta.Changed) != 1 {
		t.Fatalf("unexpected delta: %+v", delta)
	}
	machine := evolution.StateMachine{}
	if err := machine.Transition(evolution.StatusCandidate, evolution.StatusProposalDraft); err != nil {
		t.Fatal(err)
	}
	if err := machine.Transition(evolution.StatusReleased, evolution.StatusWatching); err == nil {
		t.Fatal("invalid transition accepted")
	}
}

func TestTriggerEngineCoversFrozenV1Triggers(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)}
	engine := evolution.NewTriggerEngine(clock, &ids{})
	run := evolution.RunInput{
		ID: "run-all", SkillID: "skill-all", Status: evolution.RunFailed,
		Error: "schema validation failed at field 42",
		Steps: []evolution.Step{
			{Name: "inspect", Manual: true, Outcome: "failed", Error: "first attempt failed"},
			{Name: "repair", Manual: true, Outcome: "succeeded"},
		},
		Evidence: []evolution.Evidence{
			{ID: "ev-security", Kind: evolution.EvidenceSecurityViolation, Summary: "path traversal detected"},
			{ID: "ev-drift", Kind: evolution.EvidenceEnvironmentDrift, Summary: "binary version drift"},
		},
		Verification: &evolution.Verification{Passed: true},
		Duration:     3 * time.Second, BaselineDuration: time.Second,
		OriginalPlan:    []evolution.PlanStep{{Name: "execute", Action: "run", Outcome: "failed"}},
		FinalPlan:       []evolution.PlanStep{{Name: "execute", Action: "repair", Outcome: "succeeded"}},
		UserCorrections: []evolution.UserCorrectionEvent{{ID: "user-1", Summary: "use the verified endpoint"}},
		UpstreamUpdates: []evolution.UpstreamUpdateEvent{{ID: "upstream-1", Summary: "upstream changed the command"}},
	}
	records, err := engine.AnalyzeRun(run)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[evolution.Trigger]bool{}
	for _, record := range records {
		seen[record.Observation.Trigger] = true
	}
	for _, trigger := range []evolution.Trigger{
		evolution.TriggerUserCorrection,
		evolution.TriggerFalseFailure,
		evolution.TriggerRepeatedFailure,
		evolution.TriggerRepeatedManualStep,
		evolution.TriggerEnvironmentDrift,
		evolution.TriggerPerformanceRegression,
		evolution.TriggerSecurityViolation,
		evolution.TriggerUpstreamUpdate,
	} {
		if !seen[trigger] {
			t.Errorf("missing trigger %s", trigger)
		}
	}

	recovery, err := engine.AnalyzeRun(evolution.RunInput{
		ID: "run-recovery", SkillID: "skill-all", Status: evolution.RunSucceeded,
		Steps:        []evolution.Step{{Name: "execute", Outcome: "failed", Error: "failed"}},
		Verification: &evolution.Verification{Passed: true},
		OriginalPlan: []evolution.PlanStep{{Name: "execute", Action: "run", Outcome: "failed"}},
		FinalPlan:    []evolution.PlanStep{{Name: "execute", Action: "repair", Outcome: "succeeded"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	seenRecovery := map[evolution.Trigger]bool{}
	for _, record := range recovery {
		seenRecovery[record.Observation.Trigger] = true
	}
	if !seenRecovery[evolution.TriggerAgentRecovery] {
		t.Error("missing agent recovery trigger")
	}

	missingValidation, err := engine.AnalyzeRun(evolution.RunInput{
		ID: "run-unverified", SkillID: "skill-all", Status: evolution.RunSucceeded,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(missingValidation) != 1 || missingValidation[0].Observation.Trigger != evolution.TriggerMissingValidation {
		t.Fatalf("missing validation trigger mismatch: %#v", missingValidation)
	}
}
