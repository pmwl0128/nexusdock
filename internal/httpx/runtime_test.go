package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uvwt/agentdock-nexus/internal/config"
)

func TestRuntimeTasksUsesAgentDockRuntimeAPI(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/runtime/tasks" {
			t.Fatalf("unexpected runtime path: %s", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "source": "agentdock-runtime-api", "tasks": []map[string]any{{"id": "tsk_demo", "title": "Demo Task", "goal": "show details", "status": "active", "phase": "execute", "review_status": "not_started", "condition_count": 1, "step_count": 2, "event_count": 3, "updated_at": "2026-07-06T01:02:03Z"}}, "count": 1})
	}))
	defer runtime.Close()
	server := &Server{cfg: config.Config{AgentDockEndpoint: runtime.URL}}
	request := httptest.NewRequest(http.MethodGet, "/v1/runtime/tasks?limit=20", nil)
	response := httptest.NewRecorder()

	server.runtimeTasks(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var decoded struct {
		Items []opsTaskSummary `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded.Items) != 1 || decoded.Items[0].ID != "tsk_demo" || decoded.Items[0].FileName != "tsk_demo" || decoded.Items[0].ConditionCount != 1 {
		t.Fatalf("unexpected tasks: %+v", decoded.Items)
	}
}

func TestRuntimeTaskDetailDoesNotReadFilesWhenRuntimeAPIUnconfigured(t *testing.T) {
	server := &Server{cfg: config.Config{AgentDockDir: t.TempDir()}}
	request := httptest.NewRequest(http.MethodGet, "/v1/runtime/tasks/tsk_demo", nil)
	request.SetPathValue("fileName", "tsk_demo")
	response := httptest.NewRecorder()

	server.runtimeTaskDetail(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
	if !strings.Contains(response.Body.String(), "AGENTDOCK_ENDPOINT_UNCONFIGURED") {
		t.Fatalf("body should explain missing runtime API: %s", response.Body.String())
	}
}

func TestRuntimeSkillsUsesAgentDockRuntimeAPI(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/runtime/skills" {
			t.Fatalf("unexpected runtime path: %s", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "source": "agentdock-runtime-api", "skills": []map[string]any{{"skill": "demo-skill", "versions": []string{"0.1.0", "0.2.0"}, "active_version": "0.2.0", "channels": map[string]string{"stable": "0.2.0"}, "updated_at": "2026-07-06T01:02:03Z"}}, "count": 1})
	}))
	defer runtime.Close()
	server := &Server{cfg: config.Config{AgentDockEndpoint: runtime.URL}}
	request := httptest.NewRequest(http.MethodGet, "/v1/runtime/skills", nil)
	response := httptest.NewRecorder()

	server.runtimeSkills(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var decoded struct {
		Items []opsSkillSummary `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded.Items) != 1 || decoded.Items[0].ID != "demo-skill" || decoded.Items[0].Source != "agentdock-api" || decoded.Items[0].ActiveVersion != "0.2.0" {
		t.Fatalf("unexpected skills: %+v", decoded.Items)
	}
}

func TestRuntimeSkillDetailUsesAgentDockRuntimeAPI(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/runtime/skills/demo-skill" {
			t.Fatalf("unexpected runtime path: %s", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "source": "agentdock-runtime-api", "skill": "demo-skill", "version": "0.2.0", "versions": []string{"0.1.0", "0.2.0"}, "selection": map[string]any{"active_version": "0.2.0", "channels": map[string]string{"stable": "0.2.0"}, "updated_at": "2026-07-06T01:02:03Z"}, "manifest": map[string]any{"metadata": map[string]any{"name": "demo-skill", "displayName": "Demo Skill", "description": "Runtime API skill"}}})
	}))
	defer runtime.Close()
	server := &Server{cfg: config.Config{AgentDockEndpoint: runtime.URL}}
	request := httptest.NewRequest(http.MethodGet, "/v1/runtime/skills/runtime/demo-skill", nil)
	request.SetPathValue("source", "runtime")
	request.SetPathValue("skillID", "demo-skill")
	response := httptest.NewRecorder()

	server.runtimeSkillDetail(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var decoded struct {
		Skill opsSkillDetail `json:"skill"`
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Skill.ID != "demo-skill" || decoded.Skill.Title != "Demo Skill" || decoded.Skill.ActiveVersion != "0.2.0" || decoded.Skill.Root != "agentdock-runtime-api" {
		t.Fatalf("unexpected skill detail: %+v", decoded.Skill)
	}
}
