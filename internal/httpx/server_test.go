package httpx

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uvwt/nexusdock/internal/agentdock"
	"github.com/uvwt/nexusdock/internal/config"
	"github.com/uvwt/nexusdock/internal/core"
	"github.com/uvwt/nexusdock/internal/privatenotes"
	"github.com/uvwt/nexusdock/internal/recall"
	"github.com/uvwt/nexusdock/internal/syncer"
)

func newTestHandler(t *testing.T, cfg config.Config) http.Handler {
	t.Helper()
	db, err := core.OpenSQLite(t.Context(), ":memory:", 1)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := core.NewMigrationRunner(db, nil).Run(t.Context()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	nodes, err := agentdock.NewStoreWithKey(db, make([]byte, 32))
	if err != nil {
		t.Fatalf("New AgentDock node store: %v", err)
	}
	store, err := recall.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	mgr := syncer.NewManager(syncer.Config{RepoDir: store.Root()}, slog.Default())
	privateNotes, err := privatenotes.New(filepath.Join(store.Root(), "private-notes"))
	if err != nil {
		t.Fatalf("New private notes store: %v", err)
	}
	return NewServer(cfg, store, mgr, slog.Default(), WithSystemDatabase(db), WithAgentDockNodes(nodes), WithPrivateNotes(privateNotes)).Handler()
}

func doJSON(t *testing.T, h http.Handler, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

func TestHealthDoesNotRequireAuth(t *testing.T) {
	h := newTestHandler(t, config.Config{AuthToken: "token"})
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/health", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("health status = %d body=%s", res.Code, res.Body.String())
	}
}

func TestV1BearerTokenOnly(t *testing.T) {
	h := newTestHandler(t, config.Config{AuthToken: "token"})

	res := doJSON(t, h, http.MethodGet, "/v1/recall", "")
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("missing credentials status = %d", res.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/recall", nil)
	req.Header.Set("Authorization", "Bearer token")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("bearer authorized status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestV1LocalhostAPIAccessWhenTokenEmpty(t *testing.T) {
	h := newTestHandler(t, config.Config{})

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/recall", nil)
	req.RemoteAddr = "127.0.0.1:51234"
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("loopback API access status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestUIAssetMissDoesNotFallbackToIndex(t *testing.T) {
	h := newTestHandler(t, config.Config{})

	missingAsset := httptest.NewRecorder()
	h.ServeHTTP(missingAsset, httptest.NewRequest(http.MethodGet, "/ui/assets/missing.js", nil))
	if missingAsset.Code != http.StatusNotFound {
		t.Fatalf("missing asset status=%d body=%s", missingAsset.Code, missingAsset.Body.String())
	}

	spaRoute := httptest.NewRecorder()
	h.ServeHTTP(spaRoute, httptest.NewRequest(http.MethodGet, "/ui/devices/unknown", nil))
	if spaRoute.Code != http.StatusOK {
		t.Fatalf("spa route status=%d body=%s", spaRoute.Code, spaRoute.Body.String())
	}
	if !strings.Contains(spaRoute.Body.String(), `id="root"`) {
		t.Fatalf("spa route did not serve index.html: %s", spaRoute.Body.String())
	}
}

func TestWriteMoveDeleteConfirmationAndErrorShape(t *testing.T) {
	h := newTestHandler(t, config.Config{})
	res := doJSON(t, h, http.MethodPost, "/v1/recall", `{"path":"profile.md","content":"# Profile"}`)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("write without confirmation status=%d body=%s", res.Code, res.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("error response is not json: %v", err)
	}
	if payload["ok"] != false || payload["error"] == nil {
		t.Fatalf("unexpected error shape: %#v", payload)
	}

	res = doJSON(t, h, http.MethodPost, "/v1/recall", `{"path":"profile.md","content":"# Profile","confirmed":true}`)
	if res.Code != http.StatusOK {
		t.Fatalf("confirmed write status=%d body=%s", res.Code, res.Body.String())
	}
	res = doJSON(t, h, http.MethodPost, "/v1/recall/move", `{"from_path":"profile.md","to_path":"recall/docs/projects/demo/project.md"}`)
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "confirmed") {
		t.Fatalf("move without confirmation status=%d body=%s", res.Code, res.Body.String())
	}
	res = doJSON(t, h, http.MethodDelete, "/v1/recall/profile.md", "")
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "confirmed") {
		t.Fatalf("delete without confirmation status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestCardEndpointsPlanWriteAndSearch(t *testing.T) {
	h := newTestHandler(t, config.Config{})
	captureBody := `{"title":"Deploy check","content":"Deployment must verify the final service endpoint instead of only source files.","type":"project_trap","scope":"project","project":"chatdock","status":"inbox","confidence":"high"}`
	res := doJSON(t, h, http.MethodPost, "/v1/recall/cards/capture", captureBody)
	if res.Code != http.StatusOK {
		t.Fatalf("capture status=%d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "capture_plan") || !strings.Contains(res.Body.String(), "auto_write") {
		t.Fatalf("capture response missing plan: %s", res.Body.String())
	}

	writeBody := `{"title":"Deploy check","content":"Deployment must verify the final service endpoint instead of only source files.","type":"project_trap","scope":"project","project":"chatdock","status":"inbox","confidence":"high","confirmed":true}`
	res = doJSON(t, h, http.MethodPost, "/v1/recall/cards", writeBody)
	if res.Code != http.StatusOK {
		t.Fatalf("write status=%d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "recall/managed/cards/chatdock/inbox/project_trap/") {
		t.Fatalf("write response missing card path: %s", res.Body.String())
	}

	res = doJSON(t, h, http.MethodPost, "/v1/recall/cards/search", `{"query":"service endpoint","max_results":5}`)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "Deploy check") {
		t.Fatalf("search status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestEmbeddingStatusIsGracefullyDisabled(t *testing.T) {
	h := newTestHandler(t, config.Config{})
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/embeddings/status", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("embedding status code=%d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"enabled":false`) {
		t.Fatalf("embedding status should be disabled: %s", res.Body.String())
	}
}

func TestRuntimeRoutesRequireExplicitNodeID(t *testing.T) {
	h := newTestHandler(t, config.Config{})
	for _, path := range []string{
		"/v1/runtime/tasks",
		"/v1/runtime/skills",
		"/v1/runtime/mcp",
		"/v1/runtime/overview",
		"/v1/runtime/workflow-templates",
	} {
		response := httptest.NewRecorder()
		h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("legacy singleton route %s status = %d, want 404", path, response.Code)
		}
	}

	for _, path := range []string{
		"/v1/runtime/nodes/dockmini/tasks",
		"/v1/runtime/nodes/dockmini/skills",
		"/v1/runtime/nodes/dockmini/mcp",
		"/v1/runtime/nodes/dockmini/overview",
	} {
		response := httptest.NewRecorder()
		h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "AGENTDOCK_NODE_NOT_FOUND") {
			t.Fatalf("node route %s was not registered: status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}
