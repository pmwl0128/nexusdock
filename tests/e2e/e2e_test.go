package e2e_test

import (
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uvwt/agentdock-nexus/internal/config"
	"github.com/uvwt/agentdock-nexus/internal/httpx"
	"github.com/uvwt/agentdock-nexus/internal/memory"
	"github.com/uvwt/agentdock-nexus/internal/syncer"
)

func newHandler(t *testing.T) http.Handler {
	t.Helper()
	root := t.TempDir()
	store, err := memory.NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	manager := syncer.NewManager(syncer.Config{RepoDir: root}, slog.Default())
	return httpx.NewServer(config.Config{StoreDir: root}, store, manager, slog.Default()).Handler()
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
	if !strings.Contains(ui.Body.String(), `<title>AgentDock Nexus</title>`) {
		t.Fatalf("ui index still exposes legacy title: %s", ui.Body.String())
	}
}

func TestMemoryCompatibilityAPIStillWorks(t *testing.T) {
	handler := newHandler(t)

	create := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/memories", strings.NewReader(`{"path":"inbox/e2e.md","content":"# E2E\n\nMemory compatibility remains available."}`))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(create, request)
	if create.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}

	read := httptest.NewRecorder()
	handler.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/v1/memories/inbox/e2e.md", nil))
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), "Memory compatibility remains available") {
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
	for _, label := range []string{"AgentDock", "Nexus", "总览", "待办", "设备", "记忆", "能力", "运行", "设置", "计划任务", "API 访问受限", "全局搜索", "真实 Nexus 数据", "拒绝跨源 API 请求", "INVALID_JSON"} {
		if !strings.Contains(javascript.String(), label) {
			t.Errorf("frontend bundle missing section label %q", label)
		}
	}
	for _, rule := range []string{"nexus-sidebar.is-open", "grid-template-columns:1fr", "nexus-mobile-menu", "nexus-scrim", "nexus-search-popover", "error-state"} {
		if !strings.Contains(styles.String(), rule) {
			t.Errorf("frontend stylesheet missing responsive rule %q", rule)
		}
	}
}
