package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/uvwt/memorydock/internal/audit"
	"github.com/uvwt/memorydock/internal/auth"
	"github.com/uvwt/memorydock/internal/core"
	"github.com/uvwt/memorydock/internal/runs"
)

func TestHTTPAuthRunAuditFlow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := core.OpenSQLite(ctx, filepath.Join(t.TempDir(), "nexus.db"), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migrations := core.NewMigrationRunner(db, nil)
	if err := migrations.Run(ctx); err != nil {
		t.Fatal(err)
	}
	authService := auth.NewService(db)
	if err := authService.EnsureBootstrapSystemToken(ctx, "root-secret"); err != nil {
		t.Fatal(err)
	}
	bus := core.NewEventBus()
	defer bus.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := newApp(db, migrations, authService, audit.NewService(db), runs.NewService(db, bus), logger).handler()

	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("X-Request-ID") == "" {
		t.Fatalf("health status=%d request-id=%q body=%s", response.Code, response.Header().Get("X-Request-ID"), response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/ready", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("ready status=%d body=%s", response.Code, response.Body.String())
	}

	issued := doJSON(t, handler, http.MethodPost, "/v1/auth/tokens", "root-secret", map[string]any{
		"subject_type": "agent",
		"subject_id":   "agent-1",
		"token_kind":   "agent_token",
		"scopes":       []string{"runs:read", "runs:write"},
		"ttl_seconds":  3600,
	}, http.StatusCreated)
	agentToken, _ := issued["token"].(string)
	tokenID, _ := issued["id"].(string)
	if agentToken == "" || tokenID == "" {
		t.Fatalf("invalid issued token response: %#v", issued)
	}

	created := doJSON(t, handler, http.MethodPost, "/v1/runs", agentToken, map[string]any{
		"kind":  "skill.run",
		"input": map[string]any{"value": 1},
	}, http.StatusCreated)
	runID, _ := created["id"].(string)
	if runID == "" {
		t.Fatalf("run response missing id: %#v", created)
	}
	doJSON(t, handler, http.MethodPost, "/v1/runs/"+runID+"/steps", agentToken, map[string]any{
		"sequence": 1,
		"name":     "execute",
		"status":   "succeeded",
		"input":    map[string]any{},
		"output":   map[string]any{"ok": true},
	}, http.StatusCreated)
	doJSON(t, handler, http.MethodPost, "/v1/runs/"+runID+"/evidence", agentToken, map[string]any{
		"kind":    "log",
		"payload": map[string]any{"message": "verified"},
	}, http.StatusCreated)
	doJSON(t, handler, http.MethodPost, "/v1/runs/"+runID+"/verifications", agentToken, map[string]any{
		"name":    "health",
		"status":  "passed",
		"summary": "ok",
	}, http.StatusCreated)
	doJSON(t, handler, http.MethodPost, "/v1/runs/"+runID+"/complete", agentToken, map[string]any{
		"status":  "succeeded",
		"output":  map[string]any{"done": true},
		"version": 1,
	}, http.StatusOK)

	auditResponse := doJSON(t, handler, http.MethodGet, "/v1/audit/events?limit=20", "root-secret", nil, http.StatusOK)
	items, _ := auditResponse["items"].([]any)
	if len(items) < 5 {
		t.Fatalf("audit item count=%d, want at least 5", len(items))
	}

	doJSON(t, handler, http.MethodPost, "/v1/auth/tokens/"+tokenID+"/revoke", "root-secret", map[string]any{}, http.StatusOK)
	request = httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	request.Header.Set("Authorization", "Bearer "+agentToken)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token status=%d body=%s", response.Code, response.Body.String())
	}
}

func doJSON(t *testing.T, handler http.Handler, method, path, token string, body any, wantStatus int) map[string]any {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	request := httptest.NewRequest(method, path, reader)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != wantStatus {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, path, response.Code, wantStatus, response.Body.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode %s %s response: %v body=%s", method, path, err, response.Body.String())
	}
	return decoded
}
