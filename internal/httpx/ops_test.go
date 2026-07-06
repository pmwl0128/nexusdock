package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uvwt/agentdock-nexus/internal/config"
)

func TestOpsTaskDetailReturnsRawTaskInformation(t *testing.T) {
	root := t.TempDir()
	tasksDir := filepath.Join(root, "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"id":"tsk_demo","title":"Demo Task","goal":"show details","status":"active","phase":"execute","updated_at":"2026-07-06T01:02:03Z","conditions":[{"text":"done"}],"steps":[{"id":"check"}],"events":[{"type":"created"}],"final_review":{"status":"pending"}}`
	if err := os.WriteFile(filepath.Join(tasksDir, "tsk_demo.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	server := &Server{cfg: config.Config{AgentDockDir: root}}
	request := httptest.NewRequest(http.MethodGet, "/v1/ops/tasks/tsk_demo.json", nil)
	request.SetPathValue("fileName", "tsk_demo.json")
	response := httptest.NewRecorder()

	server.opsTaskDetail(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var decoded struct {
		Task opsTaskDetail `json:"task"`
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Task.ID != "tsk_demo" || decoded.Task.ConditionCount != 1 || len(decoded.Task.Events) != 1 || !strings.Contains(decoded.Task.Content, "show details") {
		t.Fatalf("unexpected task detail: %+v", decoded.Task)
	}
}

func TestOpsTaskDetailRejectsTraversal(t *testing.T) {
	server := &Server{cfg: config.Config{AgentDockDir: t.TempDir()}}
	request := httptest.NewRequest(http.MethodGet, "/v1/ops/tasks/../secret.json", nil)
	request.SetPathValue("fileName", "../secret.json")
	response := httptest.NewRecorder()

	server.opsTaskDetail(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func TestOpsSkillDetailReturnsDocAndFiles(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "demo-skill")
	if err := os.MkdirAll(filepath.Join(skillDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Demo Skill\n\n读取具体信息。\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "bin", "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	server := &Server{cfg: config.Config{AgentDockDir: root}}
	request := httptest.NewRequest(http.MethodGet, "/v1/ops/skills/runtime/demo-skill", nil)
	request.SetPathValue("source", "runtime")
	request.SetPathValue("skillID", "demo-skill")
	response := httptest.NewRecorder()

	server.opsSkillDetail(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var decoded struct {
		Skill opsSkillDetail `json:"skill"`
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Skill.ID != "demo-skill" || !strings.Contains(decoded.Skill.SkillDoc, "读取具体信息") || len(decoded.Skill.Files) != 2 {
		t.Fatalf("unexpected skill detail: %+v", decoded.Skill)
	}
}
