package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRuntimeMCPListForwardsAgentDockAuthentication(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/internal/runtime/mcp" {
			t.Fatalf("unexpected runtime request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer runtime-token" {
			t.Fatalf("authorization = %q", got)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "servers": []map[string]any{{"name": "demo", "transport": "streamable_http", "enabled": true, "status": "ready", "tool_count": 2}}, "count": 1})
	}))
	defer runtime.Close()

	server := newRuntimeTestServer(t, runtime.URL, "runtime-token")
	response := httptest.NewRecorder()
	server.runtimeMCPServers(response, setRuntimeNode(httptest.NewRequest(http.MethodGet, "/v1/runtime/nodes/dockmini/mcp", nil)))

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"demo"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRuntimeMCPManageForwardsWhitelistedPayloadWithoutLeakingSecret(t *testing.T) {
	const secret = "nexus-runtime-mcp-secret"
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/internal/runtime/mcp" {
			t.Fatalf("unexpected runtime request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content type = %q", got)
		}
		var request runtimeMCPRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Action != "env_set" || request.Name != "demo" || request.Key != "DEMO_TOKEN" || request.Value != secret {
			t.Fatalf("unexpected request: %+v", request)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": "env_set", "name": "demo", "key": "DEMO_TOKEN", "configured": true})
	}))
	defer runtime.Close()

	server := newRuntimeTestServer(t, runtime.URL, "runtime-secret")
	request := setRuntimeNode(httptest.NewRequest(http.MethodPost, "/v1/runtime/nodes/dockmini/mcp", strings.NewReader(`{"action":"env_set","name":"demo","key":"DEMO_TOKEN","value":"`+secret+`"}`)))
	response := httptest.NewRecorder()
	server.runtimeMCPManage(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), secret) {
		t.Fatalf("response leaked secret: %s", response.Body.String())
	}
}

func TestRuntimeMCPManageRejectsUnknownActionsBeforeForwarding(t *testing.T) {
	server := newRuntimeTestServer(t, "", "")
	request := setRuntimeNode(httptest.NewRequest(http.MethodPost, "/v1/runtime/nodes/dockmini/mcp", strings.NewReader(`{"action":"inspect","name":"demo"}`)))
	response := httptest.NewRecorder()

	server.runtimeMCPManage(response, request)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "INVALID_MCP_ACTION") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRuntimeMCPDetailForwardsServerName(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/runtime/mcp/demo" {
			t.Fatalf("unexpected runtime path: %s", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "server": map[string]any{"name": "demo"}, "config": map[string]any{"name": "demo", "transport": "stdio", "command": "demo"}})
	}))
	defer runtime.Close()

	server := newRuntimeTestServer(t, runtime.URL, "runtime-secret")
	request := setRuntimeNode(httptest.NewRequest(http.MethodGet, "/v1/runtime/nodes/dockmini/mcp/demo", nil))
	request.SetPathValue("name", "demo")
	response := httptest.NewRecorder()
	server.runtimeMCPServer(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"command":"demo"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRuntimeMCPEnvironmentUsesReadOnlyBrowserRoute(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/internal/runtime/mcp" {
			t.Fatalf("unexpected runtime request: %s %s", r.Method, r.URL.Path)
		}
		var request runtimeMCPRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Action != "env_list" || request.Name != "demo" {
			t.Fatalf("unexpected request: %+v", request)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    true,
			"name":  "demo",
			"items": []map[string]any{{"key": "DEMO_TOKEN", "configured": true}},
			"count": 1,
		})
	}))
	defer runtime.Close()

	server := newRuntimeTestServer(t, runtime.URL, "runtime-secret")
	request := setRuntimeNode(httptest.NewRequest(http.MethodGet, "/v1/runtime/nodes/dockmini/mcp/demo/environment", nil))
	request.SetPathValue("name", "demo")
	response := httptest.NewRecorder()
	server.runtimeMCPEnvironment(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"key":"DEMO_TOKEN"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), `"value"`) {
		t.Fatalf("environment response must not return values: %s", response.Body.String())
	}
}
