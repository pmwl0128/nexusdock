package httpx

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	contracts "github.com/uvwt/agentdock-nexus/generated/nexuscontracts"
	"github.com/uvwt/agentdock-nexus/internal/commands"
	"github.com/uvwt/agentdock-nexus/internal/config"
	"github.com/uvwt/agentdock-nexus/internal/devices"
	"github.com/uvwt/agentdock-nexus/internal/memory"
	"github.com/uvwt/agentdock-nexus/internal/syncer"
)

func newControlPlaneTestHandler(t *testing.T) (http.Handler, *commands.Service) {
	t.Helper()
	store, err := memory.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	mgr := syncer.NewManager(syncer.Config{RepoDir: store.Root()}, slog.Default())
	deviceService, err := devices.NewService(devices.NewMemoryRepository())
	if err != nil {
		t.Fatalf("New device service: %v", err)
	}
	commandService, err := commands.NewService(commands.NewMemoryRepository(), deviceService, commands.WithLeaseDuration(30*time.Second))
	if err != nil {
		t.Fatalf("New command service: %v", err)
	}
	handler := NewServer(
		config.Config{StoreDir: store.Root(), AuthToken: "admin-token"},
		store,
		mgr,
		slog.Default(),
		WithControlPlane(deviceService, commandService),
	).Handler()
	return handler, commandService
}

func controlPlaneRequest(t *testing.T, handler http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func decodeResponse[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode response status=%d body=%s: %v", response.Code, response.Body.String(), err)
	}
	return value
}

func TestDeviceCommandHTTPFlow(t *testing.T) {
	handler, commandService := newControlPlaneTestHandler(t)
	now := time.Now().UTC().Truncate(time.Second)

	response := controlPlaneRequest(t, handler, http.MethodPost, "/v1/devices/enrollment-tokens", "admin-token", contracts.EnrollmentTokenCreateRequest{
		CreatedBy:           "test-admin",
		TtlSeconds:          3600,
		AllowedCommandTypes: []string{"health.check"},
		MaxRisk:             "low",
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("create enrollment token status=%d body=%s", response.Code, response.Body.String())
	}
	enrollmentToken := decodeResponse[contracts.EnrollmentTokenCreateResponse](t, response)

	response = controlPlaneRequest(t, handler, http.MethodPost, "/v1/devices/enroll", "", contracts.DeviceEnrollmentRequest{
		EnrollmentToken:  enrollmentToken.Token,
		Name:             "DockMini",
		Platform:         "darwin",
		Arch:             "arm64",
		AgentdockVersion: "test",
		PublicKey:        "test-public-key",
		Labels:           json.RawMessage(`{"role":"test"}`),
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("enroll status=%d body=%s", response.Code, response.Body.String())
	}
	enrolled := decodeResponse[contracts.DeviceEnrollmentResponse](t, response)

	response = controlPlaneRequest(t, handler, http.MethodPost, "/v1/devices/"+enrolled.DeviceId+"/approve", "admin-token", nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("approve status=%d body=%s", response.Code, response.Body.String())
	}

	response = controlPlaneRequest(t, handler, http.MethodPost, "/v1/devices/"+enrolled.DeviceId+"/heartbeat", enrolled.DeviceToken, contracts.DeviceHeartbeat{
		DeviceId:         "different-device",
		SentAt:           now.Format(time.RFC3339),
		UptimeSeconds:    10,
		AgentdockVersion: "test",
		Metrics:          json.RawMessage(`{"cpu_percent":1,"memory_percent":2,"disk_percent":3}`),
		Capabilities:     []contracts.DeviceCapability{},
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("mismatched heartbeat device status=%d body=%s", response.Code, response.Body.String())
	}

	response = controlPlaneRequest(t, handler, http.MethodPost, "/v1/devices/"+enrolled.DeviceId+"/heartbeat", enrolled.DeviceToken, contracts.DeviceHeartbeat{
		DeviceId:         enrolled.DeviceId,
		SentAt:           now.Format(time.RFC3339),
		UptimeSeconds:    10,
		AgentdockVersion: "test",
		Metrics:          json.RawMessage(`{"cpu_percent":1,"memory_percent":2,"disk_percent":3}`),
		Capabilities:     []contracts.DeviceCapability{{Name: "memory", Version: "v1", Enabled: true}},
	})
	if response.Code != http.StatusNoContent {
		t.Fatalf("heartbeat status=%d body=%s", response.Code, response.Body.String())
	}

	response = controlPlaneRequest(t, handler, http.MethodGet, "/api/v1/skills", "admin-token", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list skills status=%d body=%s", response.Code, response.Body.String())
	}
	var skills struct {
		Items []skillListItem `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &skills); err != nil {
		t.Fatalf("skills response is not json: %v body=%s", err, response.Body.String())
	}
	if len(skills.Items) != 1 || skills.Items[0].Name != "memory" || skills.Items[0].Installations != 1 {
		t.Fatalf("unexpected skills response: %#v", skills.Items)
	}

	response = controlPlaneRequest(t, handler, http.MethodPost, "/v1/devices/"+enrolled.DeviceId+"/commands", "admin-token", contracts.DeviceCommandCreateRequest{
		Type:           "health.check",
		Payload:        json.RawMessage(`{"scope":"local"}`),
		Risk:           "low",
		IdempotencyKey: "health-check-0001",
		Priority:       10,
		MaxAttempts:    3,
		NotBefore:      now.Add(-time.Second).Format(time.RFC3339),
		ExpiresAt:      now.Add(time.Minute).Format(time.RFC3339),
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("create command status=%d body=%s", response.Code, response.Body.String())
	}
	created := decodeResponse[contracts.DeviceCommand](t, response)

	response = controlPlaneRequest(t, handler, http.MethodGet, "/api/v1/runs", "admin-token", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list runs status=%d body=%s", response.Code, response.Body.String())
	}
	var runs struct {
		Items []runListItem `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &runs); err != nil {
		t.Fatalf("runs response is not json: %v body=%s", err, response.Body.String())
	}
	if len(runs.Items) != 1 || runs.Items[0].ID != created.Id || runs.Items[0].Status != string(commands.StatusQueued) {
		t.Fatalf("unexpected runs response: %#v", runs.Items)
	}

	response = controlPlaneRequest(t, handler, http.MethodPost, "/v1/devices/"+enrolled.DeviceId+"/commands/lease", enrolled.DeviceToken, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("lease status=%d body=%s", response.Code, response.Body.String())
	}
	lease := decodeResponse[contracts.CommandLease](t, response)
	if lease.Command.Id != created.Id || lease.LeaseId == "" {
		t.Fatalf("unexpected lease: %#v", lease)
	}

	action := contracts.CommandLeaseAction{LeaseId: lease.LeaseId}
	response = controlPlaneRequest(t, handler, http.MethodPost, "/v1/commands/"+created.Id+"/start", enrolled.DeviceToken, action)
	if response.Code != http.StatusNoContent {
		t.Fatalf("start status=%d body=%s", response.Code, response.Body.String())
	}

	response = controlPlaneRequest(t, handler, http.MethodPost, "/v1/commands/"+created.Id+"/renew", enrolled.DeviceToken, action)
	if response.Code != http.StatusOK {
		t.Fatalf("renew status=%d body=%s", response.Code, response.Body.String())
	}
	renewed := decodeResponse[contracts.CommandLease](t, response)
	if renewed.LeaseId != lease.LeaseId {
		t.Fatalf("renewed lease id=%q want=%q", renewed.LeaseId, lease.LeaseId)
	}

	percent := int64(50)
	message := "checking"
	response = controlPlaneRequest(t, handler, http.MethodPost, "/v1/commands/"+created.Id+"/progress", enrolled.DeviceToken, contracts.CommandProgress{
		CommandId:  "different-command",
		LeaseId:    lease.LeaseId,
		Status:     "running",
		Percent:    &percent,
		Message:    &message,
		ReportedAt: now.Format(time.RFC3339),
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("mismatched progress command status=%d body=%s", response.Code, response.Body.String())
	}

	response = controlPlaneRequest(t, handler, http.MethodPost, "/v1/commands/"+created.Id+"/progress", enrolled.DeviceToken, contracts.CommandProgress{
		CommandId:  created.Id,
		LeaseId:    lease.LeaseId,
		Status:     "running",
		Percent:    &percent,
		Message:    &message,
		ReportedAt: now.Format(time.RFC3339),
	})
	if response.Code != http.StatusNoContent {
		t.Fatalf("progress status=%d body=%s", response.Code, response.Body.String())
	}

	response = controlPlaneRequest(t, handler, http.MethodPost, "/v1/commands/"+created.Id+"/result", enrolled.DeviceToken, contracts.CommandResult{
		CommandId:   created.Id,
		LeaseId:     lease.LeaseId,
		Status:      "succeeded",
		StartedAt:   now.Format(time.RFC3339),
		CompletedAt: now.Add(time.Second).Format(time.RFC3339),
		Output:      json.RawMessage(`{"ok":true}`),
	})
	if response.Code != http.StatusNoContent {
		t.Fatalf("complete status=%d body=%s", response.Code, response.Body.String())
	}
	completed, err := commandService.Get(t.Context(), created.Id)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != commands.StatusSucceeded || completed.Result == nil || !completed.Result.Success {
		t.Fatalf("unexpected completed command: %#v", completed)
	}

	response = controlPlaneRequest(t, handler, http.MethodPost, "/v1/devices/"+enrolled.DeviceId+"/token/rotate", enrolled.DeviceToken, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("rotate status=%d body=%s", response.Code, response.Body.String())
	}
	rotated := decodeResponse[contracts.DeviceTokenRotationResponse](t, response)
	if rotated.DeviceToken == "" || rotated.DeviceToken == enrolled.DeviceToken {
		t.Fatalf("unexpected rotated credential")
	}

	response = controlPlaneRequest(t, handler, http.MethodPost, "/v1/devices/"+enrolled.DeviceId+"/heartbeat", enrolled.DeviceToken, contracts.DeviceHeartbeat{
		DeviceId: enrolled.DeviceId, SentAt: now.Format(time.RFC3339), AgentdockVersion: "test",
		Metrics: json.RawMessage(`{"cpu_percent":1,"memory_percent":2,"disk_percent":3}`), Capabilities: []contracts.DeviceCapability{},
	})
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("old token heartbeat status=%d body=%s", response.Code, response.Body.String())
	}

	response = controlPlaneRequest(t, handler, http.MethodPost, "/v1/devices/"+enrolled.DeviceId+"/heartbeat", rotated.DeviceToken, contracts.DeviceHeartbeat{
		DeviceId: enrolled.DeviceId, SentAt: now.Format(time.RFC3339), AgentdockVersion: "test",
		Metrics: json.RawMessage(`{"cpu_percent":1,"memory_percent":2,"disk_percent":3}`), Capabilities: []contracts.DeviceCapability{},
	})
	if response.Code != http.StatusNoContent {
		t.Fatalf("new token heartbeat status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestEnvManagePayloadIsRedactedInControlPlaneResponses(t *testing.T) {
	handler, _ := newControlPlaneTestHandler(t)
	secret := "weread-secret-value"

	response := controlPlaneRequest(t, handler, http.MethodPost, "/v1/devices/enrollment-tokens", "admin-token", contracts.EnrollmentTokenCreateRequest{
		CreatedBy:           "test-admin",
		TtlSeconds:          3600,
		AllowedCommandTypes: []string{"env.manage"},
		MaxRisk:             "medium",
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("create enrollment token status=%d body=%s", response.Code, response.Body.String())
	}
	enrollmentToken := decodeResponse[contracts.EnrollmentTokenCreateResponse](t, response)

	response = controlPlaneRequest(t, handler, http.MethodPost, "/v1/devices/enroll", "", contracts.DeviceEnrollmentRequest{
		EnrollmentToken:  enrollmentToken.Token,
		Name:             "DockMini",
		Platform:         "darwin",
		Arch:             "arm64",
		AgentdockVersion: "test",
		PublicKey:        "test-public-key",
		Labels:           json.RawMessage(`{"role":"test"}`),
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("enroll status=%d body=%s", response.Code, response.Body.String())
	}
	enrolled := decodeResponse[contracts.DeviceEnrollmentResponse](t, response)

	response = controlPlaneRequest(t, handler, http.MethodPost, "/v1/devices/"+enrolled.DeviceId+"/approve", "admin-token", nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("approve status=%d body=%s", response.Code, response.Body.String())
	}

	response = controlPlaneRequest(t, handler, http.MethodPost, "/v1/devices/"+enrolled.DeviceId+"/env/actions", "admin-token", map[string]string{
		"action": "set", "skill": "weread-skills", "name": "WEREAD_API_KEY", "kind": "secret", "value": secret,
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("create env action status=%d body=%s", response.Code, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte(secret)) {
		t.Fatalf("create response leaked secret: %s", response.Body.String())
	}
	created := decodeResponse[contracts.DeviceCommand](t, response)
	if !bytes.Contains(created.Payload, []byte("[REDACTED]")) {
		t.Fatalf("create response did not redact payload: %s", string(created.Payload))
	}

	response = controlPlaneRequest(t, handler, http.MethodGet, "/v1/devices/"+enrolled.DeviceId+"/commands", "admin-token", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list commands status=%d body=%s", response.Code, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte(secret)) {
		t.Fatalf("list response leaked secret: %s", response.Body.String())
	}

	response = controlPlaneRequest(t, handler, http.MethodGet, "/v1/commands/"+created.Id, "admin-token", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("get command status=%d body=%s", response.Code, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte(secret)) {
		t.Fatalf("get response leaked secret: %s", response.Body.String())
	}

}
