package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uvwt/nexusdock/internal/config"
)

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

	server := &Server{cfg: config.Config{AgentDockEndpoint: runtime.URL}}
	request := httptest.NewRequest(http.MethodGet, "/v1/runtime/tasks?limit=300", nil)
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

	server := &Server{cfg: config.Config{AgentDockEndpoint: runtime.URL}}
	request := httptest.NewRequest(http.MethodGet, "/v1/runtime/tasks/tsk_demo", nil)
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

	server := &Server{cfg: config.Config{AgentDockEndpoint: runtime.URL, AgentDockToken: "runtime-secret"}}
	request := httptest.NewRequest(http.MethodDelete, "/v1/runtime/tasks/tsk_demo", nil)
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

	server := &Server{cfg: config.Config{AgentDockEndpoint: runtime.URL}}
	request := httptest.NewRequest(http.MethodDelete, "/v1/runtime/tasks/tsk_missing", nil)
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

	agentDockDir := t.TempDir()
	packageDir := filepath.Join(agentDockDir, "skill-store", "installed", "demo-skill", "0.2.0")
	if err := os.MkdirAll(filepath.Join(packageDir, "references"), 0o755); err != nil {
		t.Fatalf("mkdir Skill package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "SKILL.md"), []byte("---\nname: demo-skill\ndescription: demo\nversion: 0.2.0\n---\n# Local Demo Skill\n\nLocal package description.\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "references", "guide.md"), []byte("# Guide\n\nGuide content.\n"), 0o644); err != nil {
		t.Fatalf("write guide: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, ".agentdock-install.json"), []byte(`{"internal":true}`), 0o644); err != nil {
		t.Fatalf("write install metadata: %v", err)
	}
	outsideFile := filepath.Join(t.TempDir(), "outside-secret.txt")
	if err := os.WriteFile(outsideFile, []byte("must not be exposed"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	symlinkCreated := os.Symlink(outsideFile, filepath.Join(packageDir, "outside-link.txt")) == nil

	server := &Server{cfg: config.Config{AgentDockEndpoint: runtime.URL, AgentDockDir: agentDockDir}}
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
	if decoded.Skill.ID != "demo-skill" || decoded.Skill.Title != "Local Demo Skill" || decoded.Skill.ActiveVersion != "0.2.0" || decoded.Skill.Root != filepath.ToSlash(packageDir) {
		t.Fatalf("unexpected skill detail: %+v", decoded.Skill)
	}
	if len(decoded.Skill.Files) != 2 || decoded.Skill.Files[0].Path != "SKILL.md" || decoded.Skill.Files[1].Path != "references/guide.md" {
		t.Fatalf("unexpected Skill files: %+v", decoded.Skill.Files)
	}

	metadataRequest := httptest.NewRequest(http.MethodGet, "/v1/runtime/skills/runtime/demo-skill/files/.agentdock-install.json", nil)
	metadataRequest.SetPathValue("skillID", "demo-skill")
	metadataRequest.SetPathValue("filePath", ".agentdock-install.json")
	metadataResponse := httptest.NewRecorder()
	server.runtimeSkillFile(metadataResponse, metadataRequest)
	if metadataResponse.Code != http.StatusNotFound {
		t.Fatalf("private metadata status = %d, want 404; body=%s", metadataResponse.Code, metadataResponse.Body.String())
	}

	if symlinkCreated {
		linkRequest := httptest.NewRequest(http.MethodGet, "/v1/runtime/skills/runtime/demo-skill/files/outside-link.txt", nil)
		linkRequest.SetPathValue("skillID", "demo-skill")
		linkRequest.SetPathValue("filePath", "outside-link.txt")
		linkResponse := httptest.NewRecorder()
		server.runtimeSkillFile(linkResponse, linkRequest)
		if linkResponse.Code != http.StatusBadRequest {
			t.Fatalf("outside symlink status = %d, want 400; body=%s", linkResponse.Code, linkResponse.Body.String())
		}
	}
}

func TestRuntimeSkillFileReturnsSafeTextPreview(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "skill": "demo-skill", "version": "0.2.0", "selection": map[string]any{"active_version": "0.2.0"}})
	}))
	defer runtime.Close()

	agentDockDir := t.TempDir()
	packageDir := filepath.Join(agentDockDir, "skill-store", "installed", "demo-skill", "0.2.0")
	if err := os.MkdirAll(filepath.Join(packageDir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "SKILL.md"), []byte("---\nname: demo-skill\ndescription: demo\nversion: 0.2.0\n---\n# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "references", "guide.md"), []byte("Guide content."), 0o644); err != nil {
		t.Fatal(err)
	}

	server := &Server{cfg: config.Config{AgentDockEndpoint: runtime.URL, AgentDockDir: agentDockDir}}
	request := httptest.NewRequest(http.MethodGet, "/v1/runtime/skills/runtime/demo-skill/files/references/guide.md", nil)
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

	badRequest := httptest.NewRequest(http.MethodGet, "/v1/runtime/skills/runtime/demo-skill/files/../secret", nil)
	badRequest.SetPathValue("source", "runtime")
	badRequest.SetPathValue("skillID", "demo-skill")
	badRequest.SetPathValue("filePath", "../secret")
	badResponse := httptest.NewRecorder()
	server.runtimeSkillFile(badResponse, badRequest)
	if badResponse.Code != http.StatusBadRequest {
		t.Fatalf("path traversal status = %d, want 400; body=%s", badResponse.Code, badResponse.Body.String())
	}
}

func TestRuntimeSkillFileUsesRuntimeDocumentFallback(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "skill": "demo-skill", "version": "0.2.0",
			"selection": map[string]any{"active_version": "0.2.0", "updated_at": "2026-07-06T01:02:03Z"},
			"document":  map[string]any{"name": "demo-skill", "description": "Runtime API skill", "version": "0.2.0", "body": "# Demo Skill\n\nFallback content."},
		})
	}))
	defer runtime.Close()

	server := &Server{cfg: config.Config{AgentDockEndpoint: runtime.URL}}
	detail, err := server.runtimeSkillDetailFromRuntime(t.Context(), "demo-skill")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.FileCount != 1 || len(detail.Files) != 1 || detail.Files[0].Path != "SKILL.md" {
		t.Fatalf("Runtime document should expose one SKILL.md file: %+v", detail)
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/runtime/skills/agentdock-api/demo-skill/files/SKILL.md", nil)
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
	if decoded.File.Path != "SKILL.md" || !strings.Contains(decoded.File.Content, "Fallback content") {
		t.Fatalf("unexpected fallback file: %+v", decoded.File)
	}
}
