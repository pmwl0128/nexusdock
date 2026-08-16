package recall

import (
	"errors"
	"testing"
)

func TestLifecycleTransitionCASAndIdempotency(t *testing.T) {
	store := newTestStore(t)
	record := LifecycleRecord{
		EvolutionID: "evo_0123456789abcdef", Title: "Wait for readiness", Statement: "wait for tunnel readiness", Type: "runbook",
		Scope: "project", Project: "agentdock", Status: "provisional", PolicyVersion: "v1",
	}
	request := LifecycleTransition{OperationID: "op_0123456789abcdef", ExpectedRevision: 0, PolicyVersion: "v1", NextState: "provisional", Record: record}

	first, err := store.TransitionLifecycle(request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Record.Revision != 1 || first.Idempotent {
		t.Fatalf("first transition = %#v", first)
	}

	repeated, err := store.TransitionLifecycle(request)
	if err != nil {
		t.Fatal(err)
	}
	if !repeated.Idempotent || repeated.Record.Revision != 1 {
		t.Fatalf("repeated transition = %#v", repeated)
	}

	conflict := request
	conflict.OperationID = "op_1123456789abcdef"
	if _, err := store.TransitionLifecycle(conflict); !errors.Is(err, ErrLifecycleRevisionConflict) {
		t.Fatalf("conflict error = %v", err)
	}

	next := first.Record
	next.Status = "active"
	next.SupportCount = 2
	updated, err := store.TransitionLifecycle(LifecycleTransition{OperationID: "op_2123456789abcdef", ExpectedRevision: 1, PolicyVersion: "v1", NextState: "active", Record: next})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Record.Revision != 2 || updated.Record.Status != "active" || updated.Record.SupportCount != 2 {
		t.Fatalf("updated transition = %#v", updated)
	}
}

func TestLifecycleQueryAndGeneralRecallIsolation(t *testing.T) {
	store := newTestStore(t)
	for _, record := range []LifecycleRecord{
		{EvolutionID: "evo_aaaaaaaaaaaaaaaa", Title: "A", Statement: "tunnel readiness", Type: "runbook", Scope: "project", Project: "agentdock", Status: "active", PolicyVersion: "v1"},
		{EvolutionID: "evo_bbbbbbbbbbbbbbbb", Title: "B", Statement: "README user facing", Type: "preference", Scope: "project", Project: "agentdock", Status: "verified", PolicyVersion: "v1"},
	} {
		_, err := store.TransitionLifecycle(LifecycleTransition{OperationID: "op_" + record.EvolutionID[4:] + "00", ExpectedRevision: 0, PolicyVersion: "v1", NextState: record.Status, Record: record})
		if err != nil {
			t.Fatal(err)
		}
	}

	items, err := store.QueryLifecycle(LifecycleQuery{Query: "tunnel", Statuses: []string{"active"}, Project: "agentdock"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].EvolutionID != "evo_aaaaaaaaaaaaaaaa" {
		t.Fatalf("query = %#v", items)
	}
	if _, err := store.Read("recall/managed/lifecycle/evo_aaaaaaaaaaaaaaaa.json"); !errors.Is(err, ErrDisallowedPath) {
		t.Fatalf("general recall read should reject internal lifecycle path, got %v", err)
	}
}

func TestLifecycleRejectsPolicyVersionChange(t *testing.T) {
	store := newTestStore(t)
	record := LifecycleRecord{
		EvolutionID: "evo_1111111111111111", Title: "policy", Statement: "same policy", Type: "runbook",
		Scope: "project", Project: "agentdock", Status: "provisional", PolicyVersion: "v1",
	}
	created, err := store.TransitionLifecycle(LifecycleTransition{OperationID: "op_policyversion01", ExpectedRevision: 0, PolicyVersion: "v1", NextState: "provisional", Record: record})
	if err != nil {
		t.Fatal(err)
	}
	next := created.Record
	next.PolicyVersion = "v2"
	next.Status = "active"
	_, err = store.TransitionLifecycle(LifecycleTransition{OperationID: "op_policyversion02", ExpectedRevision: 1, PolicyVersion: "v2", NextState: "active", Record: next})
	if !errors.Is(err, ErrLifecyclePolicyVersionConflict) {
		t.Fatalf("policy version error = %v", err)
	}
	current, err := store.QueryLifecycle(LifecycleQuery{EvolutionID: record.EvolutionID})
	if err != nil {
		t.Fatal(err)
	}
	if len(current) != 1 || current[0].PolicyVersion != "v1" || current[0].Revision != 1 {
		t.Fatalf("current record = %#v", current)
	}
}

func TestLifecycleOperationIDIsBoundToPayload(t *testing.T) {
	store := newTestStore(t)
	record := LifecycleRecord{
		EvolutionID: "evo_2222222222222222", Title: "operation", Statement: "same operation", Type: "runbook",
		Scope: "project", Project: "agentdock", Status: "provisional", PolicyVersion: "v1",
	}
	request := LifecycleTransition{OperationID: "op_payloadbinding01", ExpectedRevision: 0, PolicyVersion: "v1", NextState: "provisional", Record: record}
	if _, err := store.TransitionLifecycle(request); err != nil {
		t.Fatal(err)
	}
	changed := request
	changed.Record.Statement = "different payload"
	if _, err := store.TransitionLifecycle(changed); !errors.Is(err, ErrLifecycleOperationConflict) {
		t.Fatalf("operation conflict error = %v", err)
	}
}

func TestLifecycleQueryRanksRichNaturalLanguageWithoutAllTermMatch(t *testing.T) {
	store := newTestStore(t)
	records := []LifecycleRecord{
		{
			EvolutionID: "evo_3333333333333333", Title: "Guidance 召回", Statement: "Evolution Guidance 自然语言召回应采用相关度排序",
			Type: "runbook", Scope: "project", Project: "agentdock", Status: "active", PolicyVersion: "v1",
		},
		{
			EvolutionID: "evo_4444444444444444", Title: "发布验证", Statement: "发布前执行构建和测试",
			Type: "runbook", Scope: "project", Project: "agentdock", Status: "verified", PolicyVersion: "v1",
		},
		{
			EvolutionID: "evo_5555555555555555", Title: "其他项目", Statement: "Evolution Guidance 自然语言召回",
			Type: "runbook", Scope: "project", Project: "other", Status: "verified", PolicyVersion: "v1",
		},
	}
	for i, record := range records {
		operationID := []string{"op_richquery00000001", "op_richquery00000002", "op_richquery00000003"}[i]
		if _, err := store.TransitionLifecycle(LifecycleTransition{OperationID: operationID, ExpectedRevision: 0, PolicyVersion: "v1", NextState: record.Status, Record: record}); err != nil {
			t.Fatal(err)
		}
	}

	items, err := store.QueryLifecycle(LifecycleQuery{
		Query:    "全面修复 AgentDock Evolution Guidance 的自然语言召回，并完成构建、测试和真实验证",
		Statuses: []string{"active", "verified"},
		Project:  "agentdock",
		Limit:    5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("query returned %d records: %#v", len(items), items)
	}
	if items[0].EvolutionID != "evo_3333333333333333" {
		t.Fatalf("relevant guidance was not ranked first: %#v", items)
	}
	if items[1].EvolutionID != "evo_4444444444444444" {
		t.Fatalf("unexpected second result: %#v", items)
	}
}

func TestLifecycleQueryMatchesChineseWithoutWhitespace(t *testing.T) {
	store := newTestStore(t)
	record := LifecycleRecord{
		EvolutionID: "evo_6666666666666666", Title: "自然语言召回", Statement: "相关经验应按语义词项召回而不是全词硬匹配",
		Type: "runbook", Scope: "project", Project: "agentdock", Status: "active", PolicyVersion: "v1",
	}
	if _, err := store.TransitionLifecycle(LifecycleTransition{OperationID: "op_chinesequery0001", ExpectedRevision: 0, PolicyVersion: "v1", NextState: record.Status, Record: record}); err != nil {
		t.Fatal(err)
	}

	items, err := store.QueryLifecycle(LifecycleQuery{Query: "修复自然语言召回并完成完整回归测试", Statuses: []string{"active"}, Project: "agentdock"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].EvolutionID != record.EvolutionID {
		t.Fatalf("Chinese query did not recall relevant record: %#v", items)
	}
}
