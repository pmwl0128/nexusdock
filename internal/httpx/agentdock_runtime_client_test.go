package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAgentDockRuntimeClientDoesNotFollowRedirects(t *testing.T) {
	var redirectedRequests atomic.Int32
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedRequests.Add(1)
		if authorization := r.Header.Get("Authorization"); authorization != "" {
			t.Fatalf("redirect target received authorization header: %q", authorization)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}))
	defer redirectTarget.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", redirectTarget.URL+"/internal/runtime/tasks")
		writeJSON(w, http.StatusTemporaryRedirect, map[string]any{"ok": false, "code": "REDIRECTED"})
	}))
	defer origin.Close()

	client := &agentDockRuntimeClient{
		endpoint: origin.URL,
		token:    "runtime-secret",
		client: &http.Client{
			Timeout:       time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	_, err := client.request(context.Background(), http.MethodGet, "/internal/runtime/tasks", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "Temporary Redirect") {
		t.Fatalf("redirect error = %v", err)
	}
	if redirectedRequests.Load() != 0 {
		t.Fatalf("redirect target requests = %d, want 0", redirectedRequests.Load())
	}
}

func TestAgentDockRuntimeClientRejectsOversizedResponse(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"payload":"`))
		_, _ = w.Write([]byte(strings.Repeat("x", maxAgentDockRuntimeResponseBytes)))
		_, _ = w.Write([]byte(`"}`))
	}))
	defer runtime.Close()

	client := &agentDockRuntimeClient{
		endpoint: runtime.URL,
		client:   &http.Client{Timeout: 5 * time.Second},
	}
	_, err := client.request(context.Background(), http.MethodGet, "/internal/runtime/tasks", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "超过 8 MiB 限制") {
		t.Fatalf("oversized response error = %v", err)
	}
}
