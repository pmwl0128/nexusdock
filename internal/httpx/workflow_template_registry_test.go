package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uvwt/nexusdock/internal/config"
)

func TestWorkflowTemplateLifecycleMaintainsContentHashes(t *testing.T) {
	dataDir := t.TempDir()
	handler := newTestHandler(t, config.Config{NexusDataDir: dataDir})

	saveWorkflowTemplateDraft(t, handler, "development.demo", "1.0.0")
	publishWorkflowTemplate(t, handler, "development.demo", "1.0.0")
	saveWorkflowTemplateDraft(t, handler, "development.demo", "1.1.0")
	publishWorkflowTemplate(t, handler, "development.demo", "1.1.0")

	server := &Server{cfg: config.Config{NexusDataDir: dataDir}}
	retired, err := server.loadWorkflowTemplate("published", "development.demo", "1.0.0")
	if err != nil {
		t.Fatalf("load retired template: %v", err)
	}
	if retired.Status != workflowTemplateRetired || retired.RetiredAt == nil {
		t.Fatalf("older template was not retired: status=%q retired_at=%v", retired.Status, retired.RetiredAt)
	}
	if retired.Hash != workflowTemplateHash(retired) {
		t.Fatalf("retired template hash is stale: got %q want %q", retired.Hash, workflowTemplateHash(retired))
	}

	active, err := server.loadWorkflowTemplate("published", "development.demo", "1.1.0")
	if err != nil {
		t.Fatalf("load active template: %v", err)
	}
	if active.Status != workflowTemplateActive || active.Hash != workflowTemplateHash(active) {
		t.Fatalf("active template is inconsistent: status=%q hash=%q want_hash=%q", active.Status, active.Hash, workflowTemplateHash(active))
	}
	if _, err := os.Stat(server.workflowTemplatePath("drafts", "development.demo", "1.1.0")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("published draft still exists: %v", err)
	}
}

func TestWorkflowTemplatePublishFailureKeepsCurrentVersionActive(t *testing.T) {
	server := &Server{cfg: config.Config{NexusDataDir: t.TempDir()}}
	if err := server.ensureWorkflowRegistryDirs(); err != nil {
		t.Fatal(err)
	}
	current := testWorkflowTemplate("development.demo", "1.0.0")
	current.Status = workflowTemplateActive
	current.Hash = workflowTemplateHash(current)
	if err := writeWorkflowTemplateJSON(server.workflowTemplatePath("published", current.ID, current.Version), current); err != nil {
		t.Fatal(err)
	}
	next := testWorkflowTemplate(current.ID, "1.1.0")
	next.Status = workflowTemplateActive
	next.Hash = workflowTemplateHash(next)
	if err := writeWorkflowTemplateJSON(server.workflowTemplatePath("drafts", next.ID, next.Version), next); err != nil {
		t.Fatal(err)
	}

	failedPath := server.workflowTemplatePath("published", next.ID, next.Version)
	cleanupPending, code, err := server.publishWorkflowTemplate(next, time.Now().UTC(), func(path string, value any) error {
		if path == failedPath {
			return errors.New("injected publish failure")
		}
		return writeWorkflowTemplateJSON(path, value)
	})
	if err == nil || code != "WORKFLOW_PUBLISH_FAILED" || cleanupPending {
		t.Fatalf("publish result cleanup_pending=%v code=%q err=%v", cleanupPending, code, err)
	}
	reloaded, err := server.loadWorkflowTemplate("published", current.ID, current.Version)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != workflowTemplateActive {
		t.Fatalf("current template status=%q", reloaded.Status)
	}
	if _, err := os.Stat(failedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial new template remains: %v", err)
	}
	if _, err := os.Stat(server.workflowTemplatePath("drafts", next.ID, next.Version)); err != nil {
		t.Fatalf("draft was removed after failed publish: %v", err)
	}
}

func TestWorkflowTemplateRetirementFailureRollsBackAllVersions(t *testing.T) {
	server := &Server{cfg: config.Config{NexusDataDir: t.TempDir()}}
	if err := server.ensureWorkflowRegistryDirs(); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"0.9.0", "1.0.0"} {
		current := testWorkflowTemplate("development.demo", version)
		current.Status = workflowTemplateActive
		current.Hash = workflowTemplateHash(current)
		if err := writeWorkflowTemplateJSON(server.workflowTemplatePath("published", current.ID, current.Version), current); err != nil {
			t.Fatal(err)
		}
	}
	next := testWorkflowTemplate("development.demo", "1.1.0")
	next.Status = workflowTemplateActive
	next.Hash = workflowTemplateHash(next)
	failedPath := server.workflowTemplatePath("published", next.ID, "1.0.0")

	cleanupPending, code, err := server.publishWorkflowTemplate(next, time.Now().UTC(), func(path string, value any) error {
		template, ok := value.(workflowTemplate)
		if path == failedPath && ok && template.Status == workflowTemplateRetired {
			return errors.New("injected retirement failure")
		}
		return writeWorkflowTemplateJSON(path, value)
	})
	if err == nil || code != "WORKFLOW_RETIRE_OLD_FAILED" || cleanupPending {
		t.Fatalf("publish result cleanup_pending=%v code=%q err=%v", cleanupPending, code, err)
	}
	for _, version := range []string{"0.9.0", "1.0.0"} {
		reloaded, loadErr := server.loadWorkflowTemplate("published", next.ID, version)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		if reloaded.Status != workflowTemplateActive {
			t.Fatalf("template %s status=%q after rollback", version, reloaded.Status)
		}
	}
	if _, err := os.Stat(server.workflowTemplatePath("published", next.ID, next.Version)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new template remains after retirement rollback: %v", err)
	}
}

func TestWorkflowTemplateReadRejectsExtraPathSegments(t *testing.T) {
	handler := newTestHandler(t, config.Config{NexusDataDir: t.TempDir()})
	saveWorkflowTemplateDraft(t, handler, "development.demo", "1.0.0")

	response := doJSON(t, handler, http.MethodGet, "/v1/workflow-templates/development.demo/1.0.0/extra", "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("extra path segment status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestGetWorkflowTemplateDoesNotHideCorruptPublishedVersion(t *testing.T) {
	server := &Server{cfg: config.Config{NexusDataDir: t.TempDir()}}
	if err := server.ensureWorkflowRegistryDirs(); err != nil {
		t.Fatalf("ensure registry dirs: %v", err)
	}

	validDraft := testWorkflowTemplate("development.demo", "1.0.0")
	if err := writeWorkflowTemplateJSON(server.workflowTemplatePath("drafts", validDraft.ID, validDraft.Version), validDraft); err != nil {
		t.Fatalf("write draft: %v", err)
	}
	publishedPath := server.workflowTemplatePath("published", validDraft.ID, validDraft.Version)
	if err := os.WriteFile(publishedPath, []byte(`{"id":"development.demo","version":"1.0.0","unknown":true}`), 0o600); err != nil {
		t.Fatalf("write corrupt published template: %v", err)
	}

	_, err := server.getWorkflowTemplate(validDraft.ID, validDraft.Version)
	if err == nil || !strings.Contains(err.Error(), "published") {
		t.Fatalf("corrupt published template was hidden by draft fallback: %v", err)
	}
}

func TestWorkflowTemplateMatchStopsWithClientCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	embedding := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() { close(started) })
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(embedding.Close)
	t.Cleanup(func() { close(release) })

	dataDir := t.TempDir()
	handler := newTestHandler(t, config.Config{
		NexusDataDir:      dataDir,
		EmbeddingEnabled:  true,
		EmbeddingEndpoint: embedding.URL,
		EmbeddingModel:    "test-model",
		EmbeddingTimeout:  5 * time.Second,
	})
	saveWorkflowTemplateDraft(t, handler, "development.demo", "1.0.0")
	publishWorkflowTemplate(t, handler, "development.demo", "1.0.0")

	server := &Server{cfg: config.Config{NexusDataDir: dataDir, EmbeddingModel: "test-model"}}
	index := workflowTemplateVectorIndex{
		Model:     "test-model",
		Dimension: 1,
		UpdatedAt: time.Now().UTC(),
		Documents: map[string]workflowTemplateVector{
			"development.demo@1.0.0": {ID: "development.demo", Version: "1.0.0", Hash: "sha256:test", Text: "demo", Vector: []float64{1}, UpdatedAt: time.Now().UTC()},
		},
	}
	if err := writeWorkflowTemplateJSON(server.workflowTemplateVectorIndexPath(), index); err != nil {
		t.Fatalf("write vector index: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodPost, "/v1/workflow-templates/match", strings.NewReader(`{"goal":"demo","device":"DockMini","type":"development"}`)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(response, request)
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("embedding request did not start")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("workflow match did not finish after client cancellation")
	}
	if response.Code != http.StatusOK {
		t.Fatalf("match status=%d body=%s", response.Code, response.Body.String())
	}
}

func saveWorkflowTemplateDraft(t *testing.T, handler http.Handler, id, version string) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"template": testWorkflowTemplate(id, version)})
	if err != nil {
		t.Fatalf("marshal workflow draft: %v", err)
	}
	response := doJSON(t, handler, http.MethodPost, "/v1/workflow-templates/drafts", string(payload))
	if response.Code != http.StatusOK {
		t.Fatalf("save workflow draft status=%d body=%s", response.Code, response.Body.String())
	}
}

func publishWorkflowTemplate(t *testing.T, handler http.Handler, id, version string) {
	t.Helper()
	path := "/v1/workflow-templates/" + id + "/" + version + "/publish"
	response := doJSON(t, handler, http.MethodPost, path, `{}`)
	if response.Code != http.StatusOK {
		t.Fatalf("publish workflow status=%d body=%s", response.Code, response.Body.String())
	}
}

func testWorkflowTemplate(id, version string) workflowTemplate {
	return workflowTemplate{
		ID:          id,
		Version:     version,
		Title:       "Demo workflow",
		Description: "Workflow registry behavior test.",
		Status:      workflowTemplateDraft,
		Match: workflowMatchRule{
			Keywords: []string{"demo"},
			Devices:  []string{"DockMini"},
			Type:     "development",
		},
		CompletionConditions: []string{"Registry state remains consistent."},
		Steps: []workflowTemplateStep{
			{ID: "verify_registry", Title: "Verify registry state", Phase: "verify"},
		},
	}
}

func TestWorkflowTemplateRegistryFilesRemainPrivate(t *testing.T) {
	server := &Server{cfg: config.Config{NexusDataDir: t.TempDir()}}
	if err := server.ensureWorkflowRegistryDirs(); err != nil {
		t.Fatalf("ensure registry dirs: %v", err)
	}
	for _, area := range []string{"drafts", "published"} {
		info, err := os.Stat(filepath.Join(server.workflowRegistryRoot(), area))
		if err != nil {
			t.Fatalf("stat %s: %v", area, err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("%s permissions=%#o want 0700", area, info.Mode().Perm())
		}
	}
}
