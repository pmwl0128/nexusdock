package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uvwt/nexusdock/internal/config"
	"github.com/uvwt/nexusdock/internal/recall"
	"github.com/uvwt/nexusdock/internal/stage3"
)

func TestStage3ProjectsFinalReviewAndProposesOnly(t *testing.T) {
	var evolvePayload map[string]any
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/internal/runtime/tasks":
			_ = json.NewEncoder(w).Encode(map[string]any{"tasks": []any{map[string]any{
				"id": "tsk_demo", "title": "deploy", "goal": "deploy safely", "status": "completed", "review_status": "pass", "summary": "done", "updated_at": "2026-08-16T01:00:00Z",
			}}})
		case "/internal/runtime/tasks/tsk_demo":
			_ = json.NewEncoder(w).Encode(map[string]any{"task": map[string]any{
				"id": "tsk_demo", "title": "deploy", "goal": "deploy safely", "status": "completed", "review_status": "pass", "summary": "done", "updated_at": "2026-08-16T01:00:00Z",
				"final_review": map[string]any{"status": "pass", "review_revision": "rev_123", "verified_facts": []string{"readiness passed"}, "open_risks": []string{}, "missing_checks": []string{}},
				"events":       []any{map[string]any{"summary": "raw event must not leave Nexus"}},
			}})
		case "/internal/runtime/evolve":
			data, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(data, &evolvePayload); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"intent": "propose", "changed": true, "evolution_id": "evo_0123456789abcdef", "status": "provisional"})
		default:
			t.Fatalf("unexpected runtime path %s", r.URL.Path)
		}
	}))
	defer runtime.Close()

	server := newRuntimeTestServer(t, runtime.URL, "runtime-secret")
	store, err := recall.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server.store = store
	server.cfg = config.Config{NexusDataDir: t.TempDir()}
	server.logger = slog.Default()

	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if string(data) == "" || bytes.Contains(data, []byte("raw event must not leave Nexus")) {
			t.Fatalf("Stage 3 projection leaked raw task event: %s", data)
		}
		content := `{"candidates":[{"type":"runbook","statement":"wait for readiness","scope":"project","project":"agentdock","evidence_refs":["task:tsk_demo:review:rev_123:verified:0","task:invented:review:bad:verified:0"]}]}`
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
	}))
	defer model.Close()
	client, err := stage3.NewClient(stage3.Config{Endpoint: model.URL, Model: "test", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.runEvolutionStage3(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	if evolvePayload["intent"] != "propose" {
		t.Fatalf("evolve payload = %#v", evolvePayload)
	}
	refs, _ := evolvePayload["evidence_refs"].([]any)
	if len(refs) != 1 || refs[0] != "task:tsk_demo:review:rev_123:verified:0" {
		t.Fatalf("filtered evidence refs = %#v", refs)
	}
}

func TestStage3ExcludesLocalOnlyLifecycleScope(t *testing.T) {
	store, err := recall.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.TransitionLifecycle(recall.LifecycleTransition{
		OperationID: "op_localonlyscope01", ExpectedRevision: 0, PolicyVersion: "v1", NextState: "verified",
		Record: recall.LifecycleRecord{EvolutionID: "evo_3333333333333333", Title: "private", Statement: "must stay local", Type: "constraint", Scope: "local_only", Project: "agentdock", Status: "verified", PolicyVersion: "v1"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var modelBody []byte
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelBody, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": `{"candidates":[]}`}}}})
	}))
	defer model.Close()
	client, err := stage3.NewClient(stage3.Config{Endpoint: model.URL, Model: "test", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}

	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/runtime/tasks" {
			_ = json.NewEncoder(w).Encode(map[string]any{"tasks": []any{map[string]any{
				"id": "tsk_local_filter", "title": "safe", "goal": "safe", "status": "completed", "review_status": "pass", "summary": "safe", "updated_at": "2026-08-16T01:00:00Z",
			}}})
			return
		}
		if r.URL.Path == "/internal/runtime/tasks/tsk_local_filter" {
			_ = json.NewEncoder(w).Encode(map[string]any{"task": map[string]any{
				"id": "tsk_local_filter", "title": "safe", "goal": "safe", "status": "completed", "review_status": "pass", "summary": "safe", "updated_at": "2026-08-16T01:00:00Z",
				"final_review": map[string]any{"status": "pass", "review_revision": "rev_safe", "verified_facts": []string{"safe"}},
			}})
			return
		}
		http.NotFound(w, r)
	}))
	defer runtime.Close()

	server := newRuntimeTestServer(t, runtime.URL, "runtime-secret")
	server.store = store
	server.cfg = config.Config{NexusDataDir: t.TempDir()}
	server.logger = slog.Default()
	if err := server.runEvolutionStage3(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(modelBody, []byte("must stay local")) || bytes.Contains(modelBody, []byte("evo_3333333333333333")) {
		t.Fatalf("local_only lifecycle leaked to model: %s", modelBody)
	}
}

func TestStage3SchedulerRunsImmediatelyAndWakeKeepsOriginalDeadline(t *testing.T) {
	base := time.Date(2026, 8, 16, 6, 0, 0, 0, time.UTC)
	var nowNanos atomic.Int64
	nowNanos.Store(base.UnixNano())
	now := func() time.Time { return time.Unix(0, nowNanos.Load()).UTC() }

	cfg := config.Config{
		EvolutionEnabled:  true,
		ModelEndpoint:     "http://model.invalid",
		ModelName:         "test",
		EvolutionInterval: 2 * time.Hour,
	}
	server := &Server{cfg: cfg, aiCfg: cfg, aiCfgSet: true, stage3Wake: make(chan struct{}, 1)}
	runs := make(chan config.Config, 8)
	delays := make(chan time.Duration, 8)
	ticks := make(chan time.Time, 8)
	newTimer := func(wait time.Duration) evolutionStage3Timer {
		delays <- wait
		return evolutionStage3Timer{c: ticks, stop: func() {}}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go server.evolutionStage3Loop(ctx, now, newTimer, func(_ context.Context, got config.Config) { runs <- got })

	select {
	case got := <-runs:
		if got.ModelName != "test" {
			t.Fatalf("initial run config = %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("Stage 3 did not run immediately on startup")
	}
	select {
	case wait := <-delays:
		if wait != 2*time.Hour {
			t.Fatalf("initial wait = %s, want 2h", wait)
		}
	case <-time.After(time.Second):
		t.Fatal("scheduler did not arm interval after initial run")
	}

	// 30 分钟后配置被唤醒时，下一次执行仍锚定首次尝试时间，不重新延后完整 2 小时。
	nowNanos.Store(base.Add(30 * time.Minute).UnixNano())
	server.stage3Wake <- struct{}{}
	select {
	case wait := <-delays:
		if wait != 90*time.Minute {
			t.Fatalf("wait after wake = %s, want 90m", wait)
		}
	case <-time.After(time.Second):
		t.Fatal("scheduler did not re-arm after config wake")
	}
	select {
	case <-runs:
		t.Fatal("config wake triggered an unintended immediate Stage 3 run")
	default:
	}

	ticks <- now()
	select {
	case <-runs:
	case <-time.After(time.Second):
		t.Fatal("scheduled Stage 3 run did not execute")
	}
}

func TestStage3SchedulerRunsImmediatelyWhenReenabled(t *testing.T) {
	cfg := config.Config{
		EvolutionEnabled:  false,
		ModelEndpoint:     "http://model.invalid",
		ModelName:         "test",
		EvolutionInterval: 2 * time.Hour,
	}
	server := &Server{cfg: cfg, aiCfg: cfg, aiCfgSet: true, stage3Wake: make(chan struct{}, 1)}
	runs := make(chan struct{}, 2)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go server.evolutionStage3Loop(
		ctx,
		time.Now,
		func(time.Duration) evolutionStage3Timer {
			return evolutionStage3Timer{c: make(chan time.Time), stop: func() {}}
		},
		func(context.Context, config.Config) { runs <- struct{}{} },
	)

	server.mu.Lock()
	server.aiCfg.EvolutionEnabled = true
	server.mu.Unlock()
	server.stage3Wake <- struct{}{}
	select {
	case <-runs:
	case <-time.After(time.Second):
		t.Fatal("Stage 3 did not run immediately after disabled -> enabled")
	}
}
