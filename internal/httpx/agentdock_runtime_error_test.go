package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAgentDockRuntimeClientNormalizesUpstreamErrorCodes(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "code": "UPSTREAM_PRIVATE_CODE", "error": "upstream rejected request"})
	}))
	defer runtime.Close()

	client := &agentDockRuntimeClient{endpoint: runtime.URL, client: runtime.Client()}
	_, err := client.request(context.Background(), http.MethodGet, "/internal/runtime/tasks", nil, nil)
	var runtimeError agentDockRuntimeError
	if !errors.As(err, &runtimeError) {
		t.Fatalf("error type=%T value=%v", err, err)
	}
	if runtimeError.Code != "AGENTDOCK_RUNTIME_REQUEST_FAILED" || runtimeError.UpstreamCode != "UPSTREAM_PRIVATE_CODE" {
		t.Fatalf("runtime error=%#v", runtimeError)
	}
	payload := runtimeUnavailablePayload(err)
	detail, ok := payload["error"].(map[string]any)
	if !ok || detail["code"] != "AGENTDOCK_RUNTIME_REQUEST_FAILED" || detail["upstream_code"] != "UPSTREAM_PRIVATE_CODE" {
		t.Fatalf("payload=%#v", payload)
	}
}
