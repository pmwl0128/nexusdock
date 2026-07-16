package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAgentDockNodeCRUDNeverReturnsToken(t *testing.T) {
	server := newRuntimeTestServer(t, "", "")
	const token = "private-runtime-token"

	create := httptest.NewRequest(http.MethodPost, "/v1/runtime/nodes", strings.NewReader(`{
		"id":"dockmini",
		"name":"DockMini",
		"endpoint":"https://dockmini.example.com",
		"token":"`+token+`",
		"timeout_seconds":12
	}`))
	created := httptest.NewRecorder()
	server.agentDockNodeCreate(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	if strings.Contains(created.Body.String(), token) || !strings.Contains(created.Body.String(), `"token_configured":true`) {
		t.Fatalf("create response leaked or omitted token state: %s", created.Body.String())
	}

	listed := httptest.NewRecorder()
	server.agentDockNodeList(listed, httptest.NewRequest(http.MethodGet, "/v1/runtime/nodes", nil))
	if listed.Code != http.StatusOK || strings.Contains(listed.Body.String(), token) || !strings.Contains(listed.Body.String(), `"id":"dockmini"`) {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body.String())
	}

	name := "Mac mini"
	update := httptest.NewRequest(http.MethodPatch, "/v1/runtime/nodes/dockmini", strings.NewReader(`{"name":"`+name+`","enabled":false}`))
	update.SetPathValue("nodeID", "dockmini")
	updated := httptest.NewRecorder()
	server.agentDockNodeUpdate(updated, update)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), name) || !strings.Contains(updated.Body.String(), `"enabled":false`) {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/v1/runtime/nodes/dockmini", nil)
	get.SetPathValue("nodeID", "dockmini")
	got := httptest.NewRecorder()
	server.agentDockNodeGet(got, get)
	if got.Code != http.StatusOK || strings.Contains(got.Body.String(), token) {
		t.Fatalf("get status=%d body=%s", got.Code, got.Body.String())
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/v1/runtime/nodes/dockmini", nil)
	deleteRequest.SetPathValue("nodeID", "dockmini")
	deleted := httptest.NewRecorder()
	server.agentDockNodeDelete(deleted, deleteRequest)
	if deleted.Code != http.StatusOK || !strings.Contains(deleted.Body.String(), `"deleted":true`) {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
}

func TestAgentDockNodeCreateRejectsInvalidOrigin(t *testing.T) {
	server := newRuntimeTestServer(t, "", "")
	request := httptest.NewRequest(http.MethodPost, "/v1/runtime/nodes", strings.NewReader(`{
		"id":"dockmini",
		"name":"DockMini",
		"endpoint":"https://dockmini.example.com/mcp",
		"token":"secret"
	}`))
	response := httptest.NewRecorder()
	server.agentDockNodeCreate(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "INVALID_AGENTDOCK_NODE") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAgentDockNodeProbeUsesStoredCredentials(t *testing.T) {
	const token = "runtime-probe-token"
	calls := 0
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/healthz":
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": "0.5.1"})
		case "/internal/runtime/tasks":
			if r.URL.Query().Get("limit") != "1" {
				t.Fatalf("task limit = %q", r.URL.Query().Get("limit"))
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tasks": []any{}, "count": 0})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer runtime.Close()

	server := newRuntimeTestServer(t, runtime.URL, token)
	request := httptest.NewRequest(http.MethodPost, "/v1/runtime/nodes/dockmini/probe", nil)
	request.SetPathValue("nodeID", runtimeTestNodeID)
	response := httptest.NewRecorder()
	server.agentDockNodeProbe(response, request)
	if response.Code != http.StatusOK || calls != 2 || !strings.Contains(response.Body.String(), `"available":true`) {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
	}
	if strings.Contains(response.Body.String(), token) {
		t.Fatalf("probe response leaked token: %s", response.Body.String())
	}
}
