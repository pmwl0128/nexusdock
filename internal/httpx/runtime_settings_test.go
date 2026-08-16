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
	"time"

	"github.com/uvwt/nexusdock/internal/config"
	"github.com/uvwt/nexusdock/internal/core"
	"github.com/uvwt/nexusdock/internal/recall"
	"github.com/uvwt/nexusdock/internal/settings"
)

func newRuntimeSettingsHTTPServer(t *testing.T, cfg config.Config) *Server {
	t.Helper()
	dataDir := t.TempDir()
	db, err := core.OpenSQLite(t.Context(), filepath.Join(dataDir, "nexus.db"), 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := core.EnsureSchema(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	store, err := recall.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg.NexusDataDir = dataDir
	cfg.RecallRepoDir = store.Root()
	runtimeSettings, err := settings.NewStore(db, dataDir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return NewServer(cfg, store, nil, slog.Default(), WithSystemDatabase(db), WithRuntimeSettings(runtimeSettings))
}

func TestRuntimeAISettingsAPIProtectsSecretsAndAppliesEmbeddingConfiguration(t *testing.T) {
	const (
		authToken      = "nexus-settings-test-token"
		embeddingToken = "embedding-settings-secret"
	)
	var embeddingAuthorization string
	embedding := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		embeddingAuthorization = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"index": 0, "embedding": []float64{1, 0}}}})
	}))
	defer embedding.Close()

	server := newRuntimeSettingsHTTPServer(t, config.Config{
		AuthToken: authToken, RequireAuth: true,
		EmbeddingModel: recall.DefaultEmbeddingModel, EmbeddingTimeout: 30 * time.Second,
		ModelTimeout: 60 * time.Second, EvolutionInterval: 6 * time.Hour,
	})
	handler := server.Handler()

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/settings/ai", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated settings status = %d, want 401", unauthorized.Code)
	}

	body := map[string]any{
		"embedding": map[string]any{
			"enabled": true, "endpoint": embedding.URL, "model": "test-embedding", "timeout_seconds": 15,
			"api_key": map[string]any{"action": "replace", "value": embeddingToken},
		},
		"stage3": map[string]any{
			"enabled": false, "endpoint": "", "model": "", "timeout_seconds": 60, "interval_minutes": 360,
			"api_key": map[string]any{"action": "keep"},
		},
	}
	payload, _ := json.Marshal(body)
	update := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v1/settings/ai", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(update, req)
	if update.Code != http.StatusOK {
		t.Fatalf("settings update status=%d body=%s", update.Code, update.Body.String())
	}
	if strings.Contains(update.Body.String(), embeddingToken) {
		t.Fatal("settings update response leaked embedding API key")
	}

	read := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/settings/ai", nil)
	req.Header.Set("Authorization", "Bearer "+authToken)
	handler.ServeHTTP(read, req)
	if read.Code != http.StatusOK || strings.Contains(read.Body.String(), embeddingToken) {
		t.Fatalf("settings read response invalid or leaked secret: status=%d body=%s", read.Code, read.Body.String())
	}
	var response struct {
		Settings settings.View `json:"settings"`
	}
	if err := json.Unmarshal(read.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Settings.Embedding.Enabled || !response.Settings.Embedding.APIKeyConfigured || response.Settings.Embedding.Model != "test-embedding" {
		t.Fatalf("unexpected settings response: %#v", response.Settings)
	}

	status := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/embeddings/status", nil)
	req.Header.Set("Authorization", "Bearer "+authToken)
	handler.ServeHTTP(status, req)
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"reachable":true`) {
		t.Fatalf("embedding status=%d body=%s", status.Code, status.Body.String())
	}
	if embeddingAuthorization != "Bearer "+embeddingToken {
		t.Fatalf("embedding authorization=%q", embeddingAuthorization)
	}
}

func TestWorkflowEmbeddingUsesRuntimeAPIKey(t *testing.T) {
	const token = "workflow-embedding-secret"
	var authorization string
	embedding := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"index": 0, "embedding": []float64{1, 0}}}})
	}))
	defer embedding.Close()

	server := &Server{}
	vectors, err := server.embedWorkflowTemplateTexts(t.Context(), config.Config{
		EmbeddingEndpoint: embedding.URL,
		EmbeddingModel:    "test-embedding",
		EmbeddingAPIKey:   token,
		EmbeddingTimeout:  time.Second,
	}, []string{"workflow text"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 2 {
		t.Fatalf("unexpected vectors: %#v", vectors)
	}
	if authorization != "Bearer "+token {
		t.Fatalf("workflow embedding authorization=%q", authorization)
	}
}
