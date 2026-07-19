package nexuscontracts

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeneratedClientEscapesPathParameters(t *testing.T) {
	var escapedPath string
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		escapedPath = r.URL.EscapedPath()
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.Token = "test-token"
	if _, err := client.GetRuntimeSkillFile(context.Background(), "node/one", "builtin", "skill id", "docs/my file.md"); err != nil {
		t.Fatal(err)
	}
	want := "/v1/runtime/nodes/node%2Fone/skills/builtin/skill%20id/files/docs/my%20file.md"
	if escapedPath != want {
		t.Fatalf("escaped path=%q want=%q", escapedPath, want)
	}
	if authorization != "Bearer test-token" {
		t.Fatalf("authorization=%q", authorization)
	}
}

func TestGeneratedClientPreservesRecallPathSegments(t *testing.T) {
	var escapedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		escapedPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"recall": map[string]any{
				"path": "recall/docs/my file.md", "content": "demo", "body": "demo",
				"frontmatter": map[string]string{}, "size_bytes": 4,
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.ReadRecall(context.Background(), "recall/docs/my file.md")
	if err != nil {
		t.Fatal(err)
	}
	if result.Recall.Path != "recall/docs/my file.md" || escapedPath != "/v1/recall/recall/docs/my%20file.md" {
		t.Fatalf("result=%#v escaped_path=%q", result, escapedPath)
	}
}

func TestGeneratedClientSendsTypedRequestAndDecodesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request AgentDockNodeCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Id != "mini" || request.Token != "secret" {
			t.Fatalf("request=%#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(AgentDockNodeResponse{Ok: true, Node: AgentDockNode{Id: request.Id, Name: request.Name, Endpoint: request.Endpoint}})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.CreateAgentDockNode(context.Background(), AgentDockNodeCreateRequest{Id: "mini", Name: "Mac mini", Endpoint: "https://mini.example", Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if !response.Ok || response.Node.Id != "mini" {
		t.Fatalf("response=%#v", response)
	}
}

func TestGeneratedClientEncodesQueryParameters(t *testing.T) {
	var rawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	status := "in progress"
	query := "deploy & verify"
	limit := int64(25)
	if _, err := client.ListRuntimeTasks(context.Background(), "mini", &status, &query, &limit); err != nil {
		t.Fatal(err)
	}
	if rawQuery != "limit=25&q=deploy+%26+verify&status=in+progress" {
		t.Fatalf("raw query=%q", rawQuery)
	}

	if _, err := client.GetGitCommit(context.Background(), "feature/hash value"); err != nil {
		t.Fatal(err)
	}
	if rawQuery != "hash=feature%2Fhash+value" {
		t.Fatalf("required query=%q", rawQuery)
	}
}

func TestGeneratedClientDecodesLegacyErrorEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"ok":false,"request_id":"req_legacy","error":{"code":"STATE_CONFLICT","message":"state changed"}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetHealth(context.Background())
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("error type=%T value=%v", err, err)
	}
	if apiError.StatusCode != http.StatusConflict || apiError.Code != "STATE_CONFLICT" || apiError.Message != "state changed" || apiError.RequestID != "req_legacy" {
		t.Fatalf("api error=%#v", apiError)
	}
}

func TestGeneratedClientDecodesRuntimeErrorEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"ok":false,"available":false,"source":"agentdock-runtime-api","request_id":"req_runtime","error":{"code":"AGENTDOCK_RUNTIME_REQUEST_FAILED","message":"runtime rejected request","upstream_code":"TASK_LOCKED"}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetHealth(context.Background())
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("error type=%T value=%v", err, err)
	}
	if apiError.StatusCode != http.StatusBadGateway || apiError.Code != "AGENTDOCK_RUNTIME_REQUEST_FAILED" || apiError.Message != "runtime rejected request" || apiError.RequestID != "req_runtime" || apiError.UpstreamCode != "TASK_LOCKED" {
		t.Fatalf("api error=%#v", apiError)
	}
}

func TestGeneratedClientRejectsOversizedAndTrailingResponses(t *testing.T) {
	t.Run("oversized", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"ok":true,"service":"` + strings.Repeat("x", maxClientResponseBodyBytes) + `"}`))
		}))
		defer server.Close()
		client, err := NewClient(server.URL, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.GetHealth(context.Background()); err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("oversized response error=%v", err)
		}
	})

	t.Run("trailing", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"ok":true,"service":"nexus"} {"extra":true}`))
		}))
		defer server.Close()
		client, err := NewClient(server.URL, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.GetHealth(context.Background()); err == nil || !strings.Contains(err.Error(), "decode response") {
			t.Fatalf("trailing response error=%v", err)
		}
	})
}
