package httpx

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uvwt/memorydock/internal/config"
	"github.com/uvwt/memorydock/internal/memory"
	"github.com/uvwt/memorydock/internal/syncer"
)

func newTestHandler(t *testing.T, cfg config.Config) http.Handler {
	t.Helper()
	store, err := memory.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if cfg.StoreDir == "" {
		cfg.StoreDir = store.Root()
	}
	mgr := syncer.NewManager(syncer.Config{RepoDir: store.Root()}, slog.Default())
	return NewServer(cfg, store, mgr, slog.Default()).Handler()
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
	h := newTestHandler(t, config.Config{AuthToken: "token", Username: "admin", Password: "secret"})
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/health", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("health status = %d body=%s", res.Code, res.Body.String())
	}
}

func TestV1BearerTokenAndBasicAuth(t *testing.T) {
	h := newTestHandler(t, config.Config{AuthToken: "token", Username: "admin", Password: "secret"})

	res := doJSON(t, h, http.MethodGet, "/v1/memories", "")
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("missing basic auth status = %d", res.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/memories", nil)
	req.SetBasicAuth("admin", "secret")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("basic auth fallback status=%d body=%s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/memories", nil)
	req.Header.Set("Authorization", "Bearer token")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("bearer authorized status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestWriteMoveDeleteConfirmationAndErrorShape(t *testing.T) {
	h := newTestHandler(t, config.Config{})
	res := doJSON(t, h, http.MethodPost, "/v1/memories", `{"path":"profile.md","content":"# Profile"}`)
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

	res = doJSON(t, h, http.MethodPost, "/v1/memories", `{"path":"profile.md","content":"# Profile","confirmed":true}`)
	if res.Code != http.StatusOK {
		t.Fatalf("confirmed write status=%d body=%s", res.Code, res.Body.String())
	}
	res = doJSON(t, h, http.MethodPost, "/v1/memories/move", `{"from_path":"profile.md","to_path":"projects/demo/project.md"}`)
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "confirmed") {
		t.Fatalf("move without confirmation status=%d body=%s", res.Code, res.Body.String())
	}
	res = doJSON(t, h, http.MethodDelete, "/v1/memories/profile.md", "")
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "confirmed") {
		t.Fatalf("delete without confirmation status=%d body=%s", res.Code, res.Body.String())
	}
}
