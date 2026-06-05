package nexuscontracts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnrollDevice(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/devices/enroll" {
			t.Fatalf("path = %s, want /v1/devices/enroll", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}
		var request DeviceEnrollmentRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Name != "DockMini" {
			t.Fatalf("name = %q, want DockMini", request.Name)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"device_id":"11111111-1111-1111-1111-111111111111",
			"device_token":"device-token",
			"token_expires_at":"2026-07-05T00:00:00Z",
			"heartbeat_interval_seconds":30,
			"server_time":"2026-06-05T00:00:00Z"
		}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	response, err := client.EnrollDevice(context.Background(), DeviceEnrollmentRequest{
		EnrollmentToken: "enrollment-token-long-enough",
		Name:            "DockMini",
		Platform:        "darwin",
		Arch:            "arm64",
		AgentdockVersion: "0.3.0-go",
		PublicKey:       "test-public-key",
	})
	if err != nil {
		t.Fatalf("EnrollDevice: %v", err)
	}
	if response.DeviceToken != "device-token" {
		t.Fatalf("device token = %q, want device-token", response.DeviceToken)
	}
}

func TestRunSkillSendsAuthorizationAndIdempotency(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Idempotency-Key"); got != "run-key-12345678" {
			t.Fatalf("Idempotency-Key = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{
			"id":"22222222-2222-2222-2222-222222222222",
			"type":"skill",
			"status":"pending",
			"actor":{"type":"agent","id":"33333333-3333-3333-3333-333333333333"},
			"summary":"queued",
			"steps":[],
			"evidence":[],
			"version":1
		}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.Token = "test-token"
	result, err := client.RunSkill(context.Background(), "run-key-12345678", SkillRunRequest{
		SkillId:        "44444444-4444-4444-4444-444444444444",
		Operation:      "inspect",
		Input:          json.RawMessage(`{"service":"agentdock"}`),
		DeviceId:       "11111111-1111-1111-1111-111111111111",
		IdempotencyKey: "run-key-12345678",
	})
	if err != nil {
		t.Fatalf("RunSkill: %v", err)
	}
	if result.Status != "pending" {
		t.Fatalf("status = %q, want pending", result.Status)
	}
}

func TestClientReturnsNexusError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"VERSION_CONFLICT","message":"stale version","request_id":"req-1"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.GetTask(context.Background(), "55555555-5555-5555-5555-555555555555")
	if err == nil || err.Error() != "Nexus VERSION_CONFLICT: stale version" {
		t.Fatalf("error = %v", err)
	}
}
