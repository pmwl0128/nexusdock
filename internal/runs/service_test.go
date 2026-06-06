package runs

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/uvwt/agentdock-nexus/internal/core"
)

func TestRunLifecycleEvidenceVerificationAndVersionConflict(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := core.OpenSQLite(ctx, filepath.Join(t.TempDir(), "nexus.db"), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := core.NewMigrationRunner(db, nil).Run(ctx); err != nil {
		t.Fatal(err)
	}
	bus := core.NewEventBus()
	defer bus.Close()
	events, cancel := bus.Subscribe(4)
	defer cancel()
	service := NewService(db, bus)

	created, err := service.Create(ctx, CreateInput{
		Kind: "skill.run", Actor: core.Actor{Type: core.ActorAgent, ID: "agent-1"},
		IdempotencyKey: "idem-1", Input: json.RawMessage(`{"input":1}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.Create(ctx, CreateInput{Kind: "ignored", Actor: core.Actor{Type: core.ActorAgent, ID: "agent-1"}, IdempotencyKey: "idem-1"})
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != created.ID {
		t.Fatalf("idempotent run id = %s, want %s", again.ID, created.ID)
	}
	otherActor, err := service.Create(ctx, CreateInput{Kind: "other", Actor: core.Actor{Type: core.ActorAgent, ID: "agent-2"}, IdempotencyKey: "idem-1"})
	if err != nil {
		t.Fatal(err)
	}
	if otherActor.ID == created.ID {
		t.Fatal("idempotency key leaked across actors")
	}
	step, err := service.AppendStep(ctx, Step{RunID: created.ID, Sequence: 1, Name: "execute", Status: "succeeded", Input: json.RawMessage(`{}`), Output: json.RawMessage(`{"ok":true}`)})
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := service.AddEvidence(ctx, Evidence{RunID: created.ID, StepID: step.ID, Kind: "log", Payload: json.RawMessage(`{"line":"verified"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddVerification(ctx, Verification{RunID: created.ID, Name: "health", Status: "passed", Summary: "ok", EvidenceID: evidence.ID}); err != nil {
		t.Fatal(err)
	}
	completed, err := service.Complete(ctx, created.ID, CompleteInput{Status: StatusSucceeded, Output: json.RawMessage(`{"done":true}`), Version: created.Version})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != StatusSucceeded || completed.Version != 2 || completed.CompletedAt == nil {
		t.Fatalf("completed run = %#v", completed)
	}
	if _, err := service.Complete(ctx, created.ID, CompleteInput{Status: StatusSucceeded, Version: created.Version}); core.ErrorCodeOf(err) != core.CodeVersionConflict {
		t.Fatalf("second completion error = %v, want VERSION_CONFLICT", err)
	}
	first := <-events
	second := <-events
	third := <-events
	if first.Type != "run.started" || second.Type != "run.started" || third.Type != "run.completed" {
		t.Fatalf("events = %s, %s, %s", first.Type, second.Type, third.Type)
	}
}
