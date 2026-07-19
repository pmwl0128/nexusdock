package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uvwt/nexusdock/generated/nexuscontracts"
	"github.com/uvwt/nexusdock/internal/config"
	"github.com/uvwt/nexusdock/internal/httpx"
	"github.com/uvwt/nexusdock/internal/recall"
	"github.com/uvwt/nexusdock/internal/syncer"
)

func newHandler(t *testing.T) http.Handler {
	t.Helper()
	root := t.TempDir()
	store, err := recall.NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	manager := syncer.NewManager(syncer.Config{RepoDir: root}, slog.Default())
	handler := httpx.NewServer(config.Config{RecallRepoDir: root}, store, manager, slog.Default()).Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.RemoteAddr = "127.0.0.1:51234"
		handler.ServeHTTP(w, r)
	})
}

func TestHealthAndEmbeddedNexusUI(t *testing.T) {
	handler := newHandler(t)

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK || !strings.Contains(health.Body.String(), `"ok":true`) {
		t.Fatalf("health status=%d body=%s", health.Code, health.Body.String())
	}

	ui := httptest.NewRecorder()
	handler.ServeHTTP(ui, httptest.NewRequest(http.MethodGet, "/ui/", nil))
	if ui.Code != http.StatusOK {
		t.Fatalf("ui status=%d body=%s", ui.Code, ui.Body.String())
	}
	if !strings.Contains(ui.Body.String(), `<div id="root"></div>`) {
		t.Fatalf("ui index does not contain application mount point: %s", ui.Body.String())
	}
	if !strings.Contains(ui.Body.String(), `<title>NexusDock</title>`) {
		t.Fatalf("ui index still exposes legacy title: %s", ui.Body.String())
	}
}

func TestRecallAPIWorks(t *testing.T) {
	handler := newHandler(t)

	create := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/recall", strings.NewReader(`{"path":"recall/docs/inbox/e2e.md","content":"# E2E\n\nRecall API is canonical."}`))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(create, request)
	if create.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}

	read := httptest.NewRecorder()
	handler.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/v1/recall/recall/docs/inbox/e2e.md", nil))
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), "Recall API is canonical") {
		t.Fatalf("read status=%d body=%s", read.Code, read.Body.String())
	}
}

func TestBuiltFrontendContainsNexusSectionsAndResponsiveRules(t *testing.T) {
	root := filepath.Join("..", "..", "internal", "httpx", "web_dist")
	var javascript strings.Builder
	var styles strings.Builder
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		switch filepath.Ext(path) {
		case ".js":
			javascript.Write(data)
		case ".css":
			styles.Write(data)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk frontend dist: %v", err)
	}
	for _, label := range []string{"AgentDock", "Nexus", "总览", "Recall", "运行时", "MCP 服务", "添加 MCP", "隔离环境", "设置", "个人控制台", "数据库", "拒绝跨源 API 请求", "INVALID_JSON"} {
		if !strings.Contains(javascript.String(), label) {
			t.Errorf("frontend bundle missing section label %q", label)
		}
	}
	for _, rule := range []string{"nexus-sidebar.is-open", "grid-template-columns:1fr", "nexus-mobile-menu", "nexus-scrim", "settings-grid", "mcp-layout", "mcp-env-form"} {
		if !strings.Contains(styles.String(), rule) {
			t.Errorf("frontend stylesheet missing responsive rule %q", rule)
		}
	}
}

func TestGeneratedClientMatchesRealRecallAndErrorEnvelopes(t *testing.T) {
	handler := newHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()
	client, err := nexuscontracts.NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	health, err := client.GetHealth(context.Background())
	if err != nil || !health.Ok || health.Service != "nexusdock" {
		t.Fatalf("health=%#v err=%v", health, err)
	}
	authStatus, err := client.GetAuthStatus(context.Background())
	if err != nil || !authStatus.Ok || authStatus.Initialized {
		t.Fatalf("auth status=%#v err=%v", authStatus, err)
	}
	cards, err := client.ListRecallCards(context.Background(), nil)
	if err != nil || !cards.Ok || cards.Count != 0 || cards.Prefix != "recall/managed/cards" {
		t.Fatalf("recall cards=%#v err=%v", cards, err)
	}
	embedding, err := client.GetEmbeddingStatus(context.Background())
	if err != nil || !embedding.Ok || embedding.Enabled || embedding.Configured {
		t.Fatalf("embedding status=%#v err=%v", embedding, err)
	}
	backup, err := client.GetBackupStatus(context.Background())
	if err != nil || backup.Id != "nexusdock-backup" || backup.Provider != "launchd" {
		t.Fatalf("backup status=%#v err=%v", backup, err)
	}

	payload, err := json.Marshal(map[string]string{
		"path":    "recall/docs/inbox/client.md",
		"content": "# Client contract\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		server.URL+"/v1/recall",
		bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("create status=%d body=%s", response.StatusCode, body)
	}

	result, err := client.ReadRecall(context.Background(), "recall/docs/inbox/client.md")
	if err != nil || !result.Ok || result.Recall.Path != "recall/docs/inbox/client.md" || !strings.Contains(result.Recall.Content, "Client contract") {
		t.Fatalf("recall result=%#v err=%v", result, err)
	}

	_, err = client.ReadRecall(context.Background(), "recall/docs/missing.md")
	var apiError *nexuscontracts.APIError
	if !errors.As(err, &apiError) || apiError.StatusCode != http.StatusNotFound || apiError.Code != "READ_FAILED" || !strings.HasPrefix(apiError.RequestID, "req_") {
		t.Fatalf("missing recall error=%#v raw=%v", apiError, err)
	}
}
