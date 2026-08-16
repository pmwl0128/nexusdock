package httpx

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uvwt/nexusdock/internal/config"
)

func TestEvolutionLifecycleAPIUsesProgrammaticTokenAndCAS(t *testing.T) {
	h := newTestHandler(t, config.Config{AuthToken: "nexus-secret"})
	payload := map[string]any{
		"operation_id": "op_0123456789abcdef", "expected_revision": 0, "policy_version": "v1", "next_state": "provisional",
		"record": map[string]any{"evolution_id": "evo_0123456789abcdef", "title": "x", "statement": "wait for readiness", "type": "runbook", "scope": "project", "project": "agentdock", "status": "provisional", "policy_version": "v1"},
	}
	body, _ := json.Marshal(payload)

	unauthorized := httptest.NewRecorder()
	h.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/internal/recall/lifecycle/transition", bytes.NewReader(body)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d body=%s", unauthorized.Code, unauthorized.Body.String())
	}

	request := httptest.NewRequest(http.MethodPost, "/internal/recall/lifecycle/transition", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer nexus-secret")
	request.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	h.ServeHTTP(created, request)
	if created.Code != http.StatusOK {
		t.Fatalf("create status = %d body=%s", created.Code, created.Body.String())
	}

	queryBody := []byte(`{"evolution_id":"evo_0123456789abcdef"}`)
	query := httptest.NewRequest(http.MethodPost, "/internal/recall/lifecycle/query", bytes.NewReader(queryBody))
	query.Header.Set("Authorization", "Bearer nexus-secret")
	query.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, query)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"revision":1`)) {
		t.Fatalf("query status = %d body=%s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
}
