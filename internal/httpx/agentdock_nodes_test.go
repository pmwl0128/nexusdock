package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uvwt/nexusdock/internal/agentdock"
	"github.com/uvwt/nexusdock/internal/auth"
	"github.com/uvwt/nexusdock/internal/core"
)

func newNodeTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := core.OpenSQLite(context.Background(), ":memory:", 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := core.EnsureSchema(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	store, err := agentdock.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{agentDock: store, agentDockHub: agentdock.NewHub(store), auth: auth.NewService(db)}
}

func TestPairingIssuesDeviceTokenWithoutAgentDockToken(t *testing.T) {
	server := newNodeTestServer(t)
	pairing, err := server.agentDock.CreatePairingCode(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/nodes/pair", strings.NewReader(`{"code":"`+pairing.Code+`","device_id":"device_12345678","name":"DockMini"}`))
	response := httptest.NewRecorder()
	server.agentDockNodePair(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result struct {
		Node        agentdock.Node `json:"node"`
		DeviceToken string         `json:"device_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Node.ID == "" || result.DeviceToken == "" || strings.Contains(response.Body.String(), "endpoint") {
		t.Fatalf("unexpected pairing response: %s", response.Body.String())
	}
	principal, err := server.auth.Authenticate(t.Context(), result.DeviceToken)
	if err != nil || principal.Actor.Type != core.ActorDevice || principal.Actor.ID != result.Node.ID {
		t.Fatalf("device principal=%#v err=%v", principal, err)
	}
}

func TestNodeListReportsHubOnlineState(t *testing.T) {
	server := newNodeTestServer(t)
	pairing, _ := server.agentDock.CreatePairingCode(t.Context())
	if _, err := server.agentDock.Pair(t.Context(), agentdock.PairInput{Code: pairing.Code, DeviceID: "device_12345678", Name: "DockMini"}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.agentDockNodeList(response, httptest.NewRequest(http.MethodGet, "/v1/runtime/nodes", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"online":false`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDeviceTokenAccessesOnlyExplicitDeviceRoutes(t *testing.T) {
	server := newNodeTestServer(t)
	pairing, _ := server.agentDock.CreatePairingCode(t.Context())
	node, err := server.agentDock.Pair(t.Context(), agentdock.PairInput{Code: pairing.Code, DeviceID: "device_12345678", Name: "DockMini"})
	if err != nil {
		t.Fatal(err)
	}
	issued, err := server.auth.IssueToken(t.Context(), core.Actor{Type: core.ActorDevice, ID: node.ID}, "device_token", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/recall", nil)
	request.Header.Set("Authorization", "Bearer "+issued.Token)
	response := httptest.NewRecorder()
	server.withDeviceOrAPIAccess(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("device route status=%d body=%s", response.Code, response.Body.String())
	}

	// 管理 API 仍使用普通 API/管理员认证，Device Token 不会被 withAPIAccess 接受。
	adminResponse := httptest.NewRecorder()
	server.withAPIAccess(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })(adminResponse, request)
	if adminResponse.Code != http.StatusUnauthorized {
		t.Fatalf("admin route status=%d, want 401", adminResponse.Code)
	}
}
