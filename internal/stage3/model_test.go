package stage3

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGenerateRedactsSnapshotAndRejectsLifecycleAuthority(t *testing.T) {
	var requestBody string
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		requestBody = string(data)
		if r.Header.Get("Authorization") != "Bearer model-secret" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": `{"candidates":[{"type":"runbook","statement":"wait for readiness","scope":"project","project":"agentdock"}]}`}}}})
	}))
	defer model.Close()

	client, err := NewClient(Config{Endpoint: model.URL, Model: "test-model", APIKey: "model-secret", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Generate(t.Context(), Snapshot{Tasks: []TaskFact{{TaskID: "tsk_1", Title: "deploy", Goal: "token=super-secret-value", VerifiedFacts: []string{"Authorization: Bearer abcdefghijklmnop"}}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"super-secret-value", "abcdefghijklmnop"} {
		if strings.Contains(requestBody, secret) {
			t.Fatalf("model request leaked %q: %s", secret, requestBody)
		}
	}
}

func TestGenerateRejectsModelLifecycleFields(t *testing.T) {
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": `{"candidates":[{"type":"runbook","statement":"x","status":"verified"}]}`}}}})
	}))
	defer model.Close()
	client, err := NewClient(Config{Endpoint: model.URL, Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Generate(t.Context(), Snapshot{Tasks: []TaskFact{{TaskID: "tsk_1", Title: "x", Goal: "y"}}}); err == nil {
		t.Fatal("expected unknown lifecycle field to be rejected")
	}
}
