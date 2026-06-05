package tasks_test

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/uvwt/memorydock/internal/tasks"
)

type ids struct {
	mu sync.Mutex
	n  int
}

func (s *ids) next() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.n++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", s.n), nil
}

func service(t *testing.T, authz tasks.Authorizer) (*tasks.Service, *[]tasks.AuditRecord, *[]tasks.Event) {
	t.Helper()
	seq := &ids{}
	audits := []tasks.AuditRecord{}
	events := []tasks.Event{}
	var mu sync.Mutex
	s := tasks.NewService(
		tasks.NewMemoryRepository(), authz,
		tasks.AuditSinkFunc(func(_ context.Context, r tasks.AuditRecord) error {
			mu.Lock()
			defer mu.Unlock()
			audits = append(audits, r)
			return nil
		}),
		tasks.EventSinkFunc(func(_ context.Context, e tasks.Event) error {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, e)
			return nil
		}),
	).WithClock(func() time.Time { return time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC) }).WithIDGenerator(seq.next)
	return s, &audits, &events
}

func input() tasks.CreateInput {
	return tasks.CreateInput{
		Type: tasks.TypeNeedsAgent, Title: "检查 DockMini 状态", Description: "设备心跳离线",
		Category: "device_alert", SourceType: "device_alert", SourceID: "alert-1", ObjectID: "device-1",
		Priority:           tasks.PriorityHigh,
		Links:              []tasks.Link{{Type: tasks.LinkDevice, ObjectID: "device-1", Relation: "affected_device"}},
		CompletionCriteria: []string{"设备恢复在线", "healthz 通过"},
		RiskConstraints:    []string{"禁止任意 shell"}, CreationReason: "设备超过 180 秒无心跳",
	}
}

func TestUUIDV4(t *testing.T) {
	id, err := tasks.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(id) {
		t.Fatalf("invalid uuid: %s", id)
	}
}

func TestConcurrentCreateDeduplicates(t *testing.T) {
	s, audits, events := service(t, nil)
	actor := tasks.Actor{Type: "system", ID: "monitor"}
	const n = 64
	var wg sync.WaitGroup
	results := make(chan tasks.CreateResult, n)
	errs := make(chan error, n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := s.Create(context.Background(), actor, input())
			if err != nil {
				errs <- err
			} else {
				results <- r
			}
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	created := 0
	unique := map[string]bool{}
	for r := range results {
		if r.Created {
			created++
		}
		unique[r.Task.ID] = true
	}
	if created != 1 || len(unique) != 1 {
		t.Fatalf("created=%d unique=%d", created, len(unique))
	}
	if len(*audits) != 1 || len(*events) != 1 {
		t.Fatalf("audits=%d events=%d", len(*audits), len(*events))
	}
}

func TestLifecycleVerificationIdempotencyAndHistory(t *testing.T) {
	s, audits, events := service(t, nil)
	ctx := context.Background()
	created, err := s.Create(ctx, tasks.Actor{Type: "system", ID: "monitor"}, input())
	if err != nil {
		t.Fatal(err)
	}
	agent := tasks.Actor{Type: "agent", ID: "agent-1"}
	claimed, err := s.Claim(ctx, agent, created.Task.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Complete(ctx, agent, claimed.ID, tasks.CompletionInput{Summary: "已恢复", ExpectedVersion: 2}); !tasks.IsCode(err, tasks.CodeVerification) {
		t.Fatalf("expected verification error, got %v", err)
	}
	done, err := s.Complete(ctx, agent, claimed.ID, tasks.CompletionInput{
		Summary: "已恢复", VerificationSummary: "healthz=200; status=online", EvidenceIDs: []string{"ev-1"},
		ExpectedVersion: 2, IdempotencyKey: "done-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.Complete(ctx, agent, claimed.ID, tasks.CompletionInput{Summary: "x", VerificationSummary: "x", IdempotencyKey: "done-1"})
	if err != nil || replay.Version != done.Version {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	inspection, err := s.Inspect(ctx, agent, done.ID)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.CreationReason != input().CreationReason || len(inspection.Activities) != 3 {
		t.Fatalf("inspection=%+v", inspection)
	}
	if len(*audits) != 3 || len(*events) != 3 {
		t.Fatalf("audits=%d events=%d", len(*audits), len(*events))
	}
}

func TestAuthorizationAndOptimisticLock(t *testing.T) {
	authz := tasks.AuthorizerFunc(func(_ context.Context, actor tasks.Actor, _ string, _ tasks.Task) bool { return actor.ID != "intruder" })
	s, _, _ := service(t, authz)
	owner := tasks.Actor{Type: "system", ID: "monitor"}
	created, err := s.Create(context.Background(), owner, input())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(context.Background(), tasks.Actor{Type: "agent", ID: "intruder"}, created.Task.ID); !tasks.IsCode(err, tasks.CodeForbidden) {
		t.Fatalf("unauthorized read: %v", err)
	}
	title := "新标题"
	if _, err := s.Update(context.Background(), owner, created.Task.ID, tasks.UpdateInput{Title: &title, ExpectedVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(context.Background(), owner, created.Task.ID, tasks.UpdateInput{Title: &title, ExpectedVersion: 1}); !tasks.IsCode(err, tasks.CodeVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
}

func TestAutomaticSources(t *testing.T) {
	s, _, _ := service(t, nil)
	inbox := tasks.NewInbox(s)
	actor := tasks.Actor{Type: "system", ID: "event-consumer"}
	cases := []struct {
		kind     tasks.SourceKind
		typ      tasks.Type
		category string
	}{
		{tasks.SourceDeviceAlert, tasks.TypeNeedsAgent, "device_alert"},
		{tasks.SourceSkillFailure, tasks.TypeNeedsAgent, "skill_failure"},
		{tasks.SourceEvolutionProposal, tasks.TypeReview, "evolution_review"},
		{tasks.SourceMemoryConflict, tasks.TypeNeedsUser, "memory_conflict"},
		{tasks.SourceUpstreamConflict, tasks.TypeNeedsAgent, "upstream_conflict"},
		{tasks.SourceUnfinishedRun, tasks.TypeNeedsAgent, "unfinished_run"},
	}
	for i, tc := range cases {
		r, err := inbox.Ingest(context.Background(), actor, tasks.SourceEvent{Kind: tc.kind, SourceID: fmt.Sprintf("s-%d", i), ObjectID: fmt.Sprintf("o-%d", i), CompletionCriteria: []string{"验证"}})
		if err != nil {
			t.Fatal(err)
		}
		if r.Task.Type != tc.typ || r.Task.Category != tc.category {
			t.Fatalf("mapped=%+v", r.Task)
		}
	}
}
