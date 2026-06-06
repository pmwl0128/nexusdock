package tasks_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/uvwt/agentdock-nexus/internal/contextpack"
	"github.com/uvwt/agentdock-nexus/internal/tasks"
)

type memProvider struct{ payload json.RawMessage }

func (p memProvider) BuildTaskMemoryContext(context.Context, tasks.Task, int) (json.RawMessage, error) {
	return p.payload, nil
}

type devProvider struct{ calls int }

func (p *devProvider) GetDeviceSnapshot(context.Context, string) (json.RawMessage, error) {
	p.calls++
	return json.RawMessage(`{"id":"device-1","status":"offline"}`), nil
}

type skProvider struct{ calls int }

func (p *skProvider) GetSkillDetail(context.Context, string) (json.RawMessage, error) {
	p.calls++
	return json.RawMessage(`{"id":"skill-1","name":"diagnostics"}`), nil
}

type runProvider struct {
	runs     []json.RawMessage
	evidence []json.RawMessage
}

func (p runProvider) ListRecentRuns(context.Context, tasks.Task, int) ([]json.RawMessage, error) {
	return p.runs, nil
}

func (p runProvider) ListEvidence(context.Context, tasks.Task, int) ([]json.RawMessage, error) {
	return p.evidence, nil
}

func TestContextPackAuthorizationAndBudget(t *testing.T) {
	s, _, _ := service(t, nil)
	in := input()
	in.Links = append(in.Links, tasks.Link{Type: tasks.LinkSkill, ObjectID: "skill-1", Relation: "diagnostic_skill"})
	created, err := s.Create(context.Background(), tasks.Actor{Type: "system", ID: "monitor"}, in)
	if err != nil {
		t.Fatal(err)
	}
	dev := &devProvider{}
	sk := &skProvider{}
	large := json.RawMessage(`{"text":"` + strings.Repeat("x", 700) + `"}`)
	builder := contextpack.NewBuilder(
		s,
		memProvider{payload: json.RawMessage(`{"entries":[],"conflicts":[],"truncated":false,"total_bytes":0,"generated_at":"2026-06-05T12:00:00Z"}`)},
		dev,
		sk,
		runProvider{runs: []json.RawMessage{large, large, large}, evidence: []json.RawMessage{large, large, large, large}},
		contextpack.AccessCheckerFunc(func(_ context.Context, _ tasks.Actor, link tasks.Link) bool { return link.Type != tasks.LinkDevice }),
	)
	pack, err := builder.Build(context.Background(), tasks.Actor{Type: "agent", ID: "agent-1"}, created.Task.ID, 2048)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	if dev.calls != 0 || len(pack.Device) != 0 {
		t.Fatalf("unauthorized device fetched: calls=%d", dev.calls)
	}
	if sk.calls != 1 || len(pack.Skill) == 0 {
		t.Fatalf("authorized skill missing: calls=%d", sk.calls)
	}
	if len(data) > 2048 || !pack.Truncated {
		t.Fatalf("bytes=%d truncated=%v", len(data), pack.Truncated)
	}
}

func TestNexusTaskMCP(t *testing.T) {
	s, _, _ := service(t, nil)
	created, err := s.Create(context.Background(), tasks.Actor{Type: "system", ID: "monitor"}, input())
	if err != nil {
		t.Fatal(err)
	}
	handler := tasks.NewMCPHandler(s, contextpack.NewBuilder(s, nil, nil, nil, nil, nil))
	agent := tasks.Actor{Type: "agent", ID: "agent-1"}
	claimed, err := handler.Call(context.Background(), agent, tasks.MCPRequest{Action: "claim", TaskID: created.Task.ID, ExpectedVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if claimed.(tasks.Task).Status != tasks.StatusInProgress {
		t.Fatalf("claim=%+v", claimed)
	}
	pack, err := handler.Call(context.Background(), agent, tasks.MCPRequest{Action: "context", TaskID: created.Task.ID, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if pack.(contextpack.Pack).Task.ID != created.Task.ID {
		t.Fatalf("pack=%+v", pack)
	}
}
