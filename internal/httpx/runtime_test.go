package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uvwt/nexusdock/internal/agentdock"
	"github.com/uvwt/nexusdock/internal/core"
)

const runtimeTestNodeID = "dockmini"

func newRuntimeTestServer(t *testing.T, endpoint, token string) *Server {
	t.Helper()
	db, err := core.OpenSQLite(context.Background(), ":memory:", 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := core.NewMigrationRunner(db, nil).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	store, err := agentdock.NewStoreWithKey(db, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "" {
		if token == "" {
			token = "runtime-secret"
		}
		if _, err := store.Create(context.Background(), agentdock.CreateInput{
			ID: runtimeTestNodeID, Name: "DockMini", Endpoint: endpoint, Token: token,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return &Server{agentDock: store}
}

func setRuntimeNode(request *http.Request) *http.Request {
	request.SetPathValue("nodeID", runtimeTestNodeID)
	return request
}

func TestRuntimeTasksUsesAgentDockRuntimeAPI(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/runtime/tasks" {
			t.Fatalf("unexpected runtime path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "20" {
			t.Fatalf("unexpected runtime limit: %s", r.URL.RawQuery)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "source": "agentdock-runtime-api", "tasks": []map[string]any{{"id": "tsk_demo", "title": "Demo Task", "goal": "show details", "status": "active", "phase": "execute", "review_status": "not_started", "summary": "正在验证页面", "condition_count": 1, "completed_step_count": 1, "step_count": 2, "current_step": map[string]any{"id": "verify", "title": "验证页面", "status": "in_progress"}, "event_count": 3, "updated_at": "2026-07-06T01:02:03Z"}}, "count": 1})
	}))
	defer runtime.Close()
	server := newRuntimeTestServer(t, runtime.URL, "runtime-secret")
	request := setRuntimeNode(httptest.NewRequest(http.MethodGet, "/v1/runtime/nodes/dockmini/tasks?limit=20", nil))
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
	item := decoded.Items[0]
	if item.CompletedStepCount != 1 || item.StepCount != 2 || item.Summary != "正在验证页面" || item.CurrentStep == nil || item.CurrentStep.ID != "verify" || item.CurrentStep.Status != "in_progress" {
		t.Fatalf("task progress was not preserved: %+v", item)
	}
}

func TestRuntimeTasksClampsLimitToRuntimeMaximum(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("limit"); got != "200" {
			t.Fatalf("runtime limit = %q, want 200", got)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tasks": []any{}, "count": 0})
	}))
	defer runtime.Close()

	server := newRuntimeTestServer(t, runtime.URL, "runtime-secret")
	request := setRuntimeNode(httptest.NewRequest(http.MethodGet, "/v1/runtime/nodes/dockmini/tasks?limit=300", nil))
	response := httptest.NewRecorder()

	server.runtimeTasks(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestRuntimeTaskDetailDerivesProgressFromSteps(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/runtime/tasks/tsk_demo" {
			t.Fatalf("unexpected runtime path: %s", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "task": map[string]any{
			"id": "tsk_demo", "title": "Demo Task", "goal": "show progress", "status": "active", "phase": "execute", "summary": "正在实现进度条",
			"steps": []map[string]any{
				{"id": "check", "title": "检查现状", "status": "completed"},
				{"id": "implement", "title": "实现进度条", "status": "in_progress"},
				{"id": "verify", "title": "验证页面", "status": "pending"},
			},
		}})
	}))
	defer runtime.Close()

	server := newRuntimeTestServer(t, runtime.URL, "runtime-secret")
	request := setRuntimeNode(httptest.NewRequest(http.MethodGet, "/v1/runtime/nodes/dockmini/tasks/tsk_demo", nil))
	request.SetPathValue("fileName", "tsk_demo")
	response := httptest.NewRecorder()

	server.runtimeTaskDetail(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var decoded struct {
		Task opsTaskDetail `json:"task"`
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Task.CompletedStepCount != 1 || decoded.Task.StepCount != 3 || decoded.Task.CurrentStep == nil || decoded.Task.CurrentStep.ID != "implement" || decoded.Task.Summary != "正在实现进度条" {
		t.Fatalf("unexpected task progress: %+v", decoded.Task)
	}
}

func TestRuntimeTaskDetailRequiresExistingNode(t *testing.T) {
	server := newRuntimeTestServer(t, "", "")
	request := setRuntimeNode(httptest.NewRequest(http.MethodGet, "/v1/runtime/nodes/dockmini/tasks/tsk_demo", nil))
	request.SetPathValue("fileName", "tsk_demo")
	response := httptest.NewRecorder()

	server.runtimeTaskDetail(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
	if !strings.Contains(response.Body.String(), "AGENTDOCK_NODE_NOT_FOUND") {
		t.Fatalf("body should explain missing runtime API: %s", response.Body.String())
	}
}

func TestRuntimeDeleteTaskUsesAgentDockRuntimeAPI(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "action": "delete", "task_id": "tsk_demo",
			"deleted_task": map[string]any{"id": "tsk_demo", "title": "Demo Task"},
		})
	}))
	defer runtime.Close()

	server := newRuntimeTestServer(t, runtime.URL, "runtime-secret")
	request := setRuntimeNode(httptest.NewRequest(http.MethodDelete, "/v1/runtime/nodes/dockmini/tasks/tsk_demo", nil))
	request.SetPathValue("fileName", "tsk_demo")
	response := httptest.NewRecorder()

	server.runtimeDeleteTask(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if gotMethod != http.MethodDelete || gotPath != "/internal/runtime/tasks/tsk_demo" {
		t.Fatalf("unexpected upstream request: %s %s", gotMethod, gotPath)
	}
	if gotAuth != "Bearer runtime-secret" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if !strings.Contains(response.Body.String(), `"action":"delete"`) || !strings.Contains(response.Body.String(), `"source":"agentdock-runtime-api"`) {
		t.Fatalf("unexpected response body: %s", response.Body.String())
	}
}

func TestRuntimeDeleteTaskMapsMissingTaskToNotFound(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "code": "TASK_NOT_FOUND", "error": "task not found"})
	}))
	defer runtime.Close()

	server := newRuntimeTestServer(t, runtime.URL, "runtime-secret")
	request := setRuntimeNode(httptest.NewRequest(http.MethodDelete, "/v1/runtime/nodes/dockmini/tasks/tsk_missing", nil))
	request.SetPathValue("fileName", "tsk_missing")
	response := httptest.NewRecorder()

	server.runtimeDeleteTask(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"code":"TASK_NOT_FOUND"`) {
		t.Fatalf("body should preserve upstream task error: %s", response.Body.String())
	}
}

func TestRuntimeSkillsUsesAgentDockRuntimeAPI(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/runtime/skills" {
			t.Fatalf("unexpected runtime path: %s", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "source": "agentdock-runtime-api", "skills": []map[string]any{{"skill": "demo-skill", "name": "Demo Skill", "description": "Runtime API skill", "file_count": 2, "versions": []string{"0.1.0", "0.2.0"}, "active_version": "0.2.0", "channels": map[string]string{"stable": "0.2.0"}, "updated_at": "2026-07-06T01:02:03Z"}}, "count": 1})
	}))
	defer runtime.Close()
	server := newRuntimeTestServer(t, runtime.URL, "runtime-secret")
	request := setRuntimeNode(httptest.NewRequest(http.MethodGet, "/v1/runtime/nodes/dockmini/skills", nil))
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
	if len(decoded.Items) != 1 || decoded.Items[0].ID != "demo-skill" || decoded.Items[0].Title != "Demo Skill" || decoded.Items[0].Description != "Runtime API skill" || decoded.Items[0].FileCount != 2 || decoded.Items[0].Source != "agentdock-api" || decoded.Items[0].ActiveVersion != "0.2.0" {
		t.Fatalf("unexpected skills: %+v", decoded.Items)
	}
}

func TestRuntimeSkillDetailUsesAgentDockRuntimeAPI(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/runtime/skills/demo-skill" {
			t.Fatalf("unexpected runtime path: %s", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "source": "agentdock-runtime-api", "skill": "demo-skill", "version": "0.2.0",
			"versions":   []string{"0.1.0", "0.2.0"},
			"selection":  map[string]any{"active_version": "0.2.0", "channels": map[string]string{"stable": "0.2.0"}, "updated_at": "2026-07-06T01:02:03Z"},
			"document":   map[string]any{"name": "demo-skill", "description": "Runtime API skill", "version": "0.2.0", "body": "# Demo Skill\n\nRuntime API content."},
			"file_count": 2,
			"files": []map[string]any{
				{"path": "SKILL.md", "kind": "doc", "size_bytes": 120, "updated_at": "2026-07-06T01:02:03Z"},
				{"path": "references/guide.md", "kind": "doc", "size_bytes": 30, "updated_at": "2026-07-06T01:02:03Z"},
			},
		})
	}))
	defer runtime.Close()

	server := newRuntimeTestServer(t, runtime.URL, "runtime-secret")
	request := setRuntimeNode(httptest.NewRequest(http.MethodGet, "/v1/runtime/nodes/dockmini/skills/runtime/demo-skill", nil))
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
	if decoded.Skill.ID != "demo-skill" || decoded.Skill.Title != "demo-skill" || decoded.Skill.Description != "Runtime API skill" || decoded.Skill.ActiveVersion != "0.2.0" || decoded.Skill.Root != "agentdock-runtime-api" {
		t.Fatalf("unexpected skill detail: %+v", decoded.Skill)
	}
	if decoded.Skill.FileCount != 2 || len(decoded.Skill.Files) != 2 || decoded.Skill.Files[0].Path != "SKILL.md" || decoded.Skill.Files[1].Path != "references/guide.md" {
		t.Fatalf("unexpected Skill files: %+v", decoded.Skill.Files)
	}
	if !strings.Contains(decoded.Skill.SkillDoc, "Runtime API content") {
		t.Fatalf("Skill document should come from AgentDock API: %q", decoded.Skill.SkillDoc)
	}
}

func TestRuntimeSkillFileReturnsSafeTextPreview(t *testing.T) {
	requestCount := 0
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.URL.Path != "/internal/runtime/skills/demo-skill/files/references/guide.md" {
			t.Fatalf("unexpected runtime path: %s", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true,
			"file": map[string]any{
				"path": "references/guide.md", "kind": "doc", "size_bytes": 14,
				"updated_at": "2026-07-06T01:02:03Z", "content": "Guide content.", "truncated": false,
			},
		})
	}))
	defer runtime.Close()

	server := newRuntimeTestServer(t, runtime.URL, "runtime-secret")
	request := setRuntimeNode(httptest.NewRequest(http.MethodGet, "/v1/runtime/nodes/dockmini/skills/runtime/demo-skill/files/references/guide.md", nil))
	request.SetPathValue("source", "runtime")
	request.SetPathValue("skillID", "demo-skill")
	request.SetPathValue("filePath", "references/guide.md")
	response := httptest.NewRecorder()
	server.runtimeSkillFile(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	var decoded struct {
		File opsSkillFileContent `json:"file"`
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.File.Path != "references/guide.md" || decoded.File.Content != "Guide content." || decoded.File.Truncated {
		t.Fatalf("unexpected file preview: %+v", decoded.File)
	}

	badRequest := setRuntimeNode(httptest.NewRequest(http.MethodGet, "/v1/runtime/nodes/dockmini/skills/runtime/demo-skill/files/../secret", nil))
	badRequest.SetPathValue("source", "runtime")
	badRequest.SetPathValue("skillID", "demo-skill")
	badRequest.SetPathValue("filePath", "../secret")
	badResponse := httptest.NewRecorder()
	server.runtimeSkillFile(badResponse, badRequest)
	if badResponse.Code != http.StatusBadRequest {
		t.Fatalf("path traversal status = %d, want 400; body=%s", badResponse.Code, badResponse.Body.String())
	}
	if requestCount != 1 {
		t.Fatalf("invalid path should not reach AgentDock API, requests=%d", requestCount)
	}
}

func TestRuntimeSkillDetailAndSkillFileNeedNoLocalMount(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/internal/runtime/skills/demo-skill":
			writeJSON(w, http.StatusOK, map[string]any{
				"ok": true, "skill": "demo-skill", "version": "0.2.0",
				"selection":  map[string]any{"active_version": "0.2.0", "updated_at": "2026-07-06T01:02:03Z"},
				"document":   map[string]any{"name": "demo-skill", "description": "Runtime API skill", "version": "0.2.0", "body": "# Demo Skill\n\nAPI-only content."},
				"file_count": 1,
				"files":      []map[string]any{{"path": "SKILL.md", "kind": "doc", "size_bytes": 100, "updated_at": "2026-07-06T01:02:03Z"}},
			})
		case "/internal/runtime/skills/demo-skill/files/SKILL.md":
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":   true,
				"file": map[string]any{"path": "SKILL.md", "kind": "doc", "size_bytes": 100, "updated_at": "2026-07-06T01:02:03Z", "content": "# Demo Skill\n\nAPI-only content.", "truncated": false},
			})
		default:
			t.Fatalf("unexpected runtime path: %s", r.URL.Path)
		}
	}))
	defer runtime.Close()

	server := newRuntimeTestServer(t, runtime.URL, "runtime-secret")
	detail, err := server.runtimeSkillDetailFromRuntime(t.Context(), runtimeTestNodeID, "demo-skill")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.FileCount != 1 || len(detail.Files) != 1 || detail.Files[0].Path != "SKILL.md" || !strings.Contains(detail.SkillDoc, "API-only content") {
		t.Fatalf("unexpected API-only detail: %+v", detail)
	}

	request := setRuntimeNode(httptest.NewRequest(http.MethodGet, "/v1/runtime/nodes/dockmini/skills/agentdock-api/demo-skill/files/SKILL.md", nil))
	request.SetPathValue("source", "agentdock-api")
	request.SetPathValue("skillID", "demo-skill")
	request.SetPathValue("filePath", "SKILL.md")
	response := httptest.NewRecorder()
	server.runtimeSkillFile(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var decoded struct {
		File opsSkillFileContent `json:"file"`
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.File.Path != "SKILL.md" || !strings.Contains(decoded.File.Content, "API-only content") {
		t.Fatalf("unexpected API-only file: %+v", decoded.File)
	}
}
